package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/png"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/vyachin/go-pdf/internal/backend/raster"
	"github.com/vyachin/go-pdf/internal/browser"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/layout"
	"github.com/vyachin/go-pdf/internal/pdf"
	"github.com/vyachin/go-pdf/internal/profile"
)

// Convert renders a template into a PDF.
// prepared is everything Validate and Convert need before layout: the parsed
// document, the style chain, page geometry and fonts. Validate stops here;
// Convert continues to layout and PDF writing.
type prepared struct {
	doc       *html.Document
	sheets    []*css.Stylesheet
	cascade   *css.Cascade
	pageSize  css.PageSize
	margin    [4]int32
	pageStyle layout.PageStyle
	fonts     *fontRegistry
	guard     *budget.Guard
}

// prepare validates the template and resolves everything that does not need
// layout, so both entry points report the same problems.
func prepare(ctx context.Context, src []byte, opts Options) (*prepared, diag.List, error) {
	limits := opts.Limits.resolved()
	guard := budget.New(ctx, limits.MaxOperations)
	var diags diag.List
	// A caller-named encoding is an option, so it is checked before the input
	// is read and before any asset is opened.
	if opts.Charset != "" {
		if _, err := charset.Validate(opts.Charset); err != nil {
			diags.Errorf("options/charset", diag.Position{}, "", "%v", err)
			return nil, diags, nil
		}
	}
	if len(src) > limits.MaxInputBytes {
		diags.Errorf("resource/limit", diag.Position{}, "", "input is %d bytes, the limit is %d", len(src), limits.MaxInputBytes)
		return nil, diags, nil
	}
	doc, err := html.ParseWithOptions(src, html.ParseOptions{
		File:     sourceName(opts),
		MaxDepth: limits.MaxDOMDepth,
		Budget:   guard,
		Charset:  opts.Charset,
	})
	if err != nil {
		// The parser reports a depth limit as a budget error of its own; the
		// shared guard is not the source, so it must be turned into a
		// diagnostic here.
		if errors.Is(err, budget.ErrExceeded) {
			diags.Errorf("resource/limit", diag.Position{}, "", "%v", err)
			return nil, diags, nil
		}
		if failed, ctxErr := budgetFailure(guard, &diags); failed {
			return nil, diags, ctxErr
		}
		var truncated *html.TruncatedError
		if errors.As(err, &truncated) {
			diags.Errorf("html/truncated", truncated.Position, "", "%v", err)
			return nil, diags, nil
		}
		var parseErr *html.ParseError
		if errors.As(err, &parseErr) {
			diags.Errorf(parseErr.Code, diag.Position{}, "", "%v", parseErr.Err)
			return nil, diags, nil
		}
		// A document that cannot be decoded is reported at the offending byte
		// of the original file, not as a font or glyph problem further down.
		var encErr *charset.Error
		if errors.As(err, &encErr) {
			pos := diag.Position{}
			if encErr.Line > 0 {
				pos = diag.Position{File: sourceName(opts), Line: encErr.Line, Col: encErr.Col}
			}
			diags.Errorf("html/encoding", pos, "", "%s", encErr.Msg)
			return nil, diags, nil
		}
		return nil, diags, err
	}
	// A configured asset root must be usable. Opening it here means a missing
	// BaseDir or FontDir fails with its own code before any stylesheet or font
	// is read, instead of surfacing as a link error or reading unbounded.
	for _, dir := range []string{opts.BaseDir, opts.FontDir} {
		if dir == "" {
			continue
		}
		root, rootErr := fsx.Open(dir)
		if rootErr != nil {
			diags.Errorf("options/asset-root", diag.Position{}, "", "cannot open the asset directory %q: %v", dir, rootErr)
			diags.Sort()
			return nil, diags, nil
		}
		_ = root.Close()
	}
	profileOpts := profile.Options{
		BaseDir:            opts.BaseDir,
		Strict:             opts.Strict,
		Budget:             guard,
		MaxStylesheetBytes: limits.MaxStylesheetBytes,
	}
	sheets, sheetDiags := profile.CollectStylesheets(doc, profileOpts)
	strictDiags := profile.ValidateSheets(doc, profileOpts, sheets, nil)
	if strictDiags.HasErrors() && opts.ImportBrowser && !opts.Strict {
		// The document is outside the profile. Try the browser lowering and
		// validate the rewritten document; the original errors are replaced by
		// the import report, which explains every approximation.
		if fallbackSize, err := ParsePageSize(opts.PageSize); err == nil {
			if fallbackMargin, err := ParseMargin(opts.Margin); err == nil {
				pageStyle := layout.ParsePageStyle(sheets, fallbackSize, fallbackMargin)
				imported := browser.Import(doc, sheets, browser.Options{
					PageSize: pageStyle.Size,
					Margin:   pageStyle.Margin,
				})
				diags = append(diags, imported.Diagnostics...)
				if !imported.Diagnostics.HasErrors() {
					// Revalidate the rewritten document. When the import itself
					// failed, keep the profile diagnostics of the document as
					// written below: they describe the source the author must fix.
					strictDiags = profile.ValidateSheets(doc, profileOpts, sheets, nil)
				}
			}
		}
	}
	diags = append(diags, sheetDiags...)
	diags = append(diags, strictDiags...)
	if failed, ctxErr := budgetFailure(guard, &diags); failed {
		return nil, diags, ctxErr
	}
	diags.Sort()
	if diags.HasErrors() {
		return nil, diags, nil
	}
	pageSize, err := ParsePageSize(opts.PageSize)
	if err != nil {
		diags.Errorf("options/page-size", diag.Position{}, "", "%v", err)
		return nil, diags, nil
	}
	margin, err := ParseMargin(opts.Margin)
	if err != nil {
		diags.Errorf("options/margin", diag.Position{}, "", "%v", err)
		return nil, diags, nil
	}
	pageStyle := layout.ParsePageStyle(sheets, pageSize, margin)
	registry, err := newFontRegistry(ctx, guard, sheets, opts)
	if err != nil {
		if failed, ctxErr := budgetFailure(guard, &diags); failed {
			return nil, diags, ctxErr
		}
		if errors.Is(err, budget.ErrExceeded) {
			diags.Errorf("resource/limit", diag.Position{}, "", "%v", err)
			return nil, diags, nil
		}
		diags.Errorf("font/unavailable", diag.Position{}, "", "%v", err)
		return nil, diags, nil
	}
	return &prepared{
		doc:       doc,
		sheets:    sheets,
		cascade:   css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...).SetGuard(guard),
		pageSize:  pageSize,
		margin:    margin,
		pageStyle: pageStyle,
		fonts:     registry,
		guard:     guard,
	}, diags, nil
}

// budgetFailure reports why the shared work guard stopped. A cancelled context
// is returned to the caller unchanged, an exhausted budget becomes a
// resource/limit diagnostic, and a per-resource limit is left in diags by the
// stage that detected it. It reports false when work may continue.
func budgetFailure(guard *budget.Guard, diags *diag.List) (bool, error) {
	err := guard.Err()
	if err == nil {
		return false, nil
	}
	if errors.Is(err, budget.ErrExceeded) {
		diags.Errorf("resource/limit", diag.Position{}, "", "%v", err)
		return true, nil
	}
	return true, err
}

// layoutDocument runs the layout passes, including the target-counter passes,
// and appends every layout diagnostic to diags. A cancelled context is returned
// as an error; budget and layout failures stay in diags. Both Convert and
// Validate use it, so the two entry points report the same problems before the
// PDF is written.
func layoutDocument(ctx context.Context, p *prepared, opts Options, diags *diag.List) (*layout.Document, error) {
	limits := opts.Limits.resolved()
	doc, cascade, pageSize, margin, pageStyle, registry := p.doc, p.cascade, p.pageSize, p.margin, p.pageStyle, p.fonts
	images := &imageLoader{
		baseDir:        opts.BaseDir,
		maxPixels:      limits.MaxImagePixels,
		maxBytes:       limits.MaxImageBytes,
		maxTotalBytes:  limits.MaxTotalImageBytes,
		maxTotalPixels: limits.MaxTotalImagePixels,
		guard:          p.guard,
	}
	defer func() { _ = images.Close() }()

	// target-counter needs a second pass: the page numbers written into the
	// table of contents can change the page breaks themselves.
	targets := map[string]int{}
	var pages *layout.Document
	var layoutDiags diag.List
	const maxPasses = 4
	for pass := 0; pass < maxPasses; pass++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pages, layoutDiags = layout.Run(doc, layout.Context{
			Ctx:           ctx,
			Budget:        p.guard,
			MaxTableCells: limits.MaxTableCells,
			MaxPages:      limits.MaxPages,
			Fonts:         registry,
			Images:        images,
			PageSize:      pageSize,
			Margin:        margin,
			Cascade:       cascade,
			PageStyle:     pageStyle,
			TargetPages:   targets,
		})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := stageFailure(p.guard, &layoutDiags, diags); err != nil {
			return nil, err
		}
		if layoutDiags.HasErrors() {
			break
		}
		next := referencedTargets(doc, pages)
		if maps.Equal(next, targets) {
			targets = next
			break
		}
		targets = next
		if pass == maxPasses-1 {
			diags.Warnf("pagination/target-counter", diag.Position{}, "", "target-counter did not converge after %d passes", maxPasses)
		}
	}
	for id, page := range targets {
		if page == 0 {
			diags.Warnf("pagination/unknown-target", diag.Position{}, "", "no element with id %q, the table of contents shows 0", id)
		}
	}
	*diags = append(*diags, layoutDiags...)
	if opts.Strict {
		diags.EscalateWarnings()
	}
	return pages, nil
}

func Convert(ctx context.Context, src []byte, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	limits := opts.Limits.resolved()
	p, diags, err := prepare(ctx, src, opts)
	if err != nil {
		return Result{Diagnostics: diags}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{Diagnostics: diags}, err
	}
	if diags.HasErrors() {
		return Result{Diagnostics: diags}, diags.Error()
	}
	pages, err := layoutDocument(ctx, p, opts, &diags)
	if err != nil {
		return Result{Diagnostics: diags}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{Diagnostics: diags}, err
	}
	if diags.HasErrors() {
		diags.Sort()
		return Result{Diagnostics: diags}, diags.Error()
	}

	doc := p.doc
	title := documentTitle(doc, opts)
	author := metaContent(doc, "author")
	subject := metaContent(doc, "description")
	keywords := metaContent(doc, "keywords")
	for _, meta := range []struct {
		code  string
		value string
	}{
		{"options/title", title},
		{"options/author", author},
		{"options/subject", subject},
		{"options/keywords", keywords},
	} {
		if !utf8.ValidString(meta.value) {
			diags.Errorf(meta.code, diag.Position{}, "", "the metadata value is not valid UTF-8")
		}
	}
	if diags.HasErrors() {
		diags.Sort()
		return Result{Diagnostics: diags}, diags.Error()
	}
	writer := pdf.New(pdf.Options{
		Title:        title,
		Author:       author,
		Subject:      subject,
		Keywords:     keywords,
		CreationDate: opts.Date,
		Outline:      toPDFOutline(pages.Outline),
		Budget:       p.guard,
	})
	// One pdf.Image per decoded image, shared across pages, so the writer
	// embeds a repeated source once.
	pdfImages := map[[32]byte]*pdf.Image{}
	for _, p := range pages.Pages {
		writer.AddPage(p.Size.Width, p.Size.Height, toPDFOps(p.Ops, pdfImages))
	}
	data, err := writer.Bytes()
	if err != nil {
		if errors.Is(err, budget.ErrExceeded) {
			diags.Errorf("resource/limit", diag.Position{}, "", "%v", err)
			diags.Sort()
			return Result{Diagnostics: diags}, diags.Error()
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{Diagnostics: diags}, ctxErr
		}
		diags.Errorf("pdf/write", diag.Position{}, "", "%v", err)
		diags.Sort()
		return Result{Diagnostics: diags}, diags.Error()
	}
	if err := ctx.Err(); err != nil {
		return Result{Diagnostics: diags}, err
	}
	var pngData []byte
	if opts.RenderPNG && len(pages.Pages) > 0 {
		img, err := raster.Render(pages.Pages[0], raster.Options{
			DPI:       96,
			MaxPixels: limits.MaxRasterPixels,
			Budget:    p.guard,
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{Diagnostics: diags}, ctxErr
			}
			if errors.Is(err, budget.ErrExceeded) {
				diags.Errorf("resource/limit", diag.Position{}, "", "%v", err)
				diags.Sort()
				return Result{Diagnostics: diags}, diags.Error()
			}
			diags.Errorf("raster/render", diag.Position{}, "", "%v", err)
			diags.Sort()
			return Result{Diagnostics: diags}, diags.Error()
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil {
			pngData = buf.Bytes()
		}
	}
	diags.Sort()
	return Result{PDF: data, PNG: pngData, Diagnostics: diags}, nil
}

// stageFailure moves a budget failure recorded by layout into the conversion
// diagnostics and returns the context error when the conversion was cancelled.
func stageFailure(guard *budget.Guard, stage, diags *diag.List) error {
	err := guard.Err()
	if err == nil {
		return nil
	}
	if !errors.Is(err, budget.ErrExceeded) {
		return err
	}
	// Layout already reported an exceeded budget with its own diagnostic;
	// avoid a duplicate if it did.
	for _, d := range *stage {
		if d.Severity == diag.Error && d.Code == "resource/limit" {
			return nil
		}
	}
	diags.Errorf("resource/limit", diag.Position{}, "", "%v", err)
	return nil
}

// ParsePageSize parses A4, A5, Letter or an explicit "<w>x<h><unit>" size.
// The unit may be written once when both sides use it: "210x297mm" and
// "210mmx297mm" are the same size.
func ParsePageSize(s string) (css.PageSize, error) {
	if strings.TrimSpace(s) == "" {
		s = "A4"
	}
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "xX"); i > 0 {
		widthText := strings.TrimSpace(s[:i])
		heightText := strings.TrimSpace(s[i+1:])
		widthUnit, heightUnit := lengthUnit(widthText), lengthUnit(heightText)
		switch {
		case widthUnit == "" && heightUnit != "":
			widthText += heightUnit
		case heightUnit == "" && widthUnit != "":
			heightText += widthUnit
		}
		width, errW := css.ParseLengthString(widthText)
		height, errH := css.ParseLengthString(heightText)
		if errW == nil && errH == nil {
			if width.Kind != css.MP || height.Kind != css.MP || width.Value <= 0 || height.Value <= 0 {
				return css.PageSize{}, fmt.Errorf("page size %q must use absolute units", s)
			}
			return css.PageSize{Width: width.Value, Height: height.Value}, nil
		}
	}
	spec, err := css.ParsePageSizeSpec(css.Tokenize([]byte(s)))
	if err != nil {
		return css.PageSize{}, err
	}
	if !spec.HasBase {
		return css.PageSize{}, fmt.Errorf("page size %q must name a size (A4, A5 or Letter)", s)
	}
	if spec.Orientation != css.OrientationAny {
		return css.PageSize{}, fmt.Errorf("page size %q: orientation is set by the document, not by the flag", s)
	}
	return spec.Base, nil
}

// lengthUnit returns the unit suffix of a CSS number, for example "mm" in
// "297mm" or "" in "297". The number syntax matches the CSS tokenizer, so an
// exponent is not mistaken for a unit.
func lengthUnit(s string) string {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && s[j] >= '0' && s[j] <= '9' {
			i = j
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
		}
	}
	return strings.ToLower(s[i:])
}

// ParseMargin parses a page margin shorthand such as "20mm" or "15mm 20mm".
func ParseMargin(s string) ([4]int32, error) {
	if strings.TrimSpace(s) == "" {
		s = "20mm"
	}
	box, err := css.ParsePageMargin(css.Tokenize([]byte(s)))
	if err != nil {
		return [4]int32{}, err
	}
	var out [4]int32
	for i := 0; i < 4; i++ {
		if box[i].Kind != css.MP || box[i].Value < 0 {
			return [4]int32{}, fmt.Errorf("margin %q must use absolute units", s)
		}
		out[i] = box[i].Value
	}
	return out, nil
}

// referencedTargets maps every anchor target used in the document to the page
// it landed on. Unknown targets get page 0.
func referencedTargets(doc *html.Document, pages *layout.Document) map[string]int {
	out := map[string]int{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Tag == "a" {
			if href, ok := n.Attr("href"); ok && strings.HasPrefix(href, "#") {
				id := strings.TrimPrefix(href, "#")
				out[id] = pages.Anchors[id].Page
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc.Root)
	return out
}

func documentTitle(doc *html.Document, opts Options) string {
	if opts.Title != "" {
		return opts.Title
	}
	if head := doc.Head(); head != nil {
		for _, c := range head.Children {
			if c.Type == html.ElementNode && c.Tag == "title" {
				return strings.TrimSpace(c.TextContent())
			}
		}
	}
	return ""
}

func metaContent(doc *html.Document, name string) string {
	head := doc.Head()
	if head == nil {
		return ""
	}
	for _, c := range head.Children {
		if c.Type != html.ElementNode || c.Tag != "meta" {
			continue
		}
		if n, _ := c.Attr("name"); strings.EqualFold(n, name) {
			v, _ := c.Attr("content")
			return v
		}
	}
	return ""
}

func toPDFOutline(entries []layout.OutlineEntry) []pdf.OutlineEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]pdf.OutlineEntry, len(entries))
	for i, e := range entries {
		out[i] = pdf.OutlineEntry{Title: e.Title, Page: e.Page, Y: e.Y, Level: e.Level}
	}
	return out
}

func toPDFOps(ops []layout.Op, images map[[32]byte]*pdf.Image) []pdf.Op {
	out := make([]pdf.Op, 0, len(ops))
	for _, op := range ops {
		switch op.Kind {
		case layout.FillOpKind:
			out = append(out, pdf.Op{
				Kind:  pdf.FillOpKind,
				Rect:  pdf.Rect{X: op.Rect.X, Y: op.Rect.Y, W: op.Rect.W, H: op.Rect.H},
				Color: pdf.RGB{R: op.Color.R, G: op.Color.G, B: op.Color.B, Set: op.Color.Set},
			})
		case layout.TextOpKind:
			if op.Text == nil || len(op.Text.Run.Glyphs) == 0 {
				continue
			}
			glyphs := make([]pdf.Glyph, len(op.Text.Run.Glyphs))
			for i, g := range op.Text.Run.Glyphs {
				glyphs[i] = pdf.Glyph{GID: g.GID, Rune: g.Rune, Advance: g.Advance}
			}
			out = append(out, pdf.Op{
				Kind:  pdf.TextOpKind,
				Color: pdf.RGB{R: op.Text.Color.R, G: op.Text.Color.G, B: op.Text.Color.B, Set: op.Text.Color.Set},
				Text: &pdf.Text{
					X:      op.Text.X,
					Y:      op.Text.Y,
					Font:   op.Text.Face.Font,
					Size:   op.Text.Face.Size,
					Glyphs: glyphs,
				},
			})
		case layout.ImageOpKind:
			if op.Image == nil || op.Image.Image == nil {
				continue
			}
			img := op.Image.Image
			// The encoded bytes identify the image: two files with the same
			// content, or the same file reached through a second path, produce
			// one PDF object. Hashing also covers data: URIs, which have no path.
			key := sha256.Sum256(img.Data)
			shared, ok := images[key]
			if !ok {
				shared = &pdf.Image{
					Data:   img.Data,
					Format: img.Format,
					PixelW: img.PixelW,
					PixelH: img.PixelH,
				}
				images[key] = shared
			}
			var clip *pdf.Rect
			if c := op.Image.Clip; c != nil {
				clip = &pdf.Rect{X: c.X, Y: c.Y, W: c.W, H: c.H}
			}
			out = append(out, pdf.Op{
				Kind:  pdf.ImageOpKind,
				Rect:  pdf.Rect{X: op.Rect.X, Y: op.Rect.Y, W: op.Rect.W, H: op.Rect.H},
				Image: shared,
				Clip:  clip,
			})
		}
	}
	return out
}
