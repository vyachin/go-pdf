package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/backend/raster"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
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
	// title, author, subject and keywords are the validated metadata values.
	// Both entry points share them, because preparation checks them before
	// layout: serialisation is the only stage Validate does not run.
	title, author, subject, keywords string
	// assets is the conversion's asset registry. Preparation opens it, layout
	// reads through it and the entry point closes it on every exit.
	assets *assets.Registry
}

// aggregatePosition names the document for a failure that has no single source
// node — a conversion-wide budget, a writer error, a rasterizer error. The file
// is known and the coordinates are not, so the diagnostic can point the reader
// at the document without inventing a line or an element.
func aggregatePosition(opts Options) diag.Position {
	return diag.Position{File: sourceName(opts)}
}

// prepare validates the template and resolves everything that does not need
// layout, so both entry points report the same problems. It owns the asset
// registry: a preparation that fails closes it before it returns.
func prepare(ctx context.Context, src []byte, opts Options) (*prepared, diag.List, error) {
	limits := opts.Limits.resolved()
	guard := budget.New(ctx, limits.MaxOperations)
	reg := assets.New(guard)
	if opts.Source != nil {
		// The source replaces the host filesystem for the whole conversion:
		// the registry reads every asset through it and opens no host root.
		reg = assets.NewWithSource(guard, opts.Source)
	}
	p, diags, err := prepareDocument(ctx, src, opts, guard, reg)
	if p == nil {
		_ = reg.Close()
	}
	return p, diags, err
}

// prepareDocument is prepare with the shared guard and registry already built.
func prepareDocument(ctx context.Context, src []byte, opts Options, guard *budget.Guard, reg *assets.Registry) (*prepared, diag.List, error) {
	limits := opts.Limits.resolved()
	var diags diag.List
	// Every option is parsed and checked before an asset is opened. The
	// diagnostics accumulate, so a bad option and a bad document are reported
	// together instead of the first option hiding the document problems.
	var pageSize css.PageSize
	var margin [4]int32
	charsetInvalid := false
	optionInvalid := false
	if opts.Charset != "" {
		if _, err := charset.Validate(opts.Charset); err != nil {
			diags.Errorf(diag.OptionsCharset, diag.Position{}, "", "%v", err)
			charsetInvalid, optionInvalid = true, true
		}
	}
	if p, err := ParsePageSize(opts.PageSize); err != nil {
		diags.Errorf(diag.OptionsPageSize, diag.Position{}, "", "%v", err)
		optionInvalid = true
	} else {
		pageSize = p
	}
	if m, err := ParseMargin(opts.Margin); err != nil {
		diags.Errorf(diag.OptionsMargin, diag.Position{}, "", "%v", err)
		optionInvalid = true
	} else {
		margin = m
	}
	if !opts.Date.IsZero() {
		// The instant is written as UTC, so the UTC year is what must fit: a
		// local 9999-12-31 23:30 in a zone behind UTC becomes year 10000 in the
		// PDF, which strict validators reject.
		if year := opts.Date.UTC().Year(); year < 0 || year > 9999 {
			diags.Errorf(diag.OptionsDate, diag.Position{}, "", "the UTC year %d is outside 0..9999", year)
			optionInvalid = true
		}
	}
	if charsetInvalid {
		diags.Sort()
		return nil, diags, nil
	}
	if len(src) > limits.MaxInputBytes {
		diags.Errorf(diag.ResourceLimit, aggregatePosition(opts), "", "input is %d bytes, the limit is %d", len(src), limits.MaxInputBytes)
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
			diags.Errorf(diag.ResourceLimit, aggregatePosition(opts), "", "%v", err)
			return nil, diags, nil
		}
		if failed, ctxErr := budgetFailure(guard, &diags); failed {
			return nil, diags, ctxErr
		}
		var truncated *html.TruncatedError
		if errors.As(err, &truncated) {
			diags.Errorf(diag.HTMLTruncated, truncated.Position, "", "%v", err)
			return nil, diags, nil
		}
		var parseErr *html.ParseError
		if errors.As(err, &parseErr) {
			diags.Errorf(diag.Code(parseErr.Code), diag.Position{}, "", "%v", parseErr.Err)
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
			diags.Errorf(diag.HTMLEncoding, pos, "", "%s", encErr.Msg)
			return nil, diags, nil
		}
		return nil, diags, err
	}
	profileOpts := profile.Options{
		BaseDir:                 opts.BaseDir,
		Strict:                  opts.Strict,
		Budget:                  guard,
		MaxStylesheetBytes:      limits.MaxStylesheetBytes,
		MaxTotalStylesheetBytes: limits.MaxTotalStylesheetBytes,
		Assets:                  reg,
	}
	// The document metadata is part of the option gate: it depends only on the
	// parsed document, which exists before any asset, so an invalid title
	// cannot be hidden by a missing stylesheet or a broken font. The values are
	// kept for the PDF info dictionary; metadata errors join the other option
	// errors and follow the same invalid-option path below.
	title, author, subject, keywords := validateMetadata(doc, opts, &diags)
	metadataInvalid := diags.HasErrors()
	if optionInvalid || metadataInvalid {
		// The options are already wrong, but the document is still validated so
		// its inline CSS and attributes are reported in the same pass. Linked
		// stylesheets and fonts are not opened: no asset is read for a request
		// that cannot proceed.
		// Inline rules are validated even though an option is wrong: the
		// document's own CSS needs no asset, and the author must see both
		// problems in one pass.
		inline, inlineDiags := profile.CollectInlineStylesheets(doc, profileOpts)
		diags = append(diags, inlineDiags...)
		diags = append(diags, profile.ValidateSheets(doc, profileOpts, inline, nil)...)
		diags.Sort()
		return nil, diags, nil
	}
	// A configured asset root must be usable. Opening it here means a missing
	// BaseDir or FontDir fails with its own code before any stylesheet or font
	// is read, instead of surfacing as a link error or reading unbounded. A
	// source root is a directory inside the caller's fs.FS, so it is checked
	// the same way without opening a host directory.
	for _, dir := range []string{opts.BaseDir, opts.FontDir} {
		if dir == "" {
			continue
		}
		if reg.IsSource() {
			if _, sourceErr := assets.SourceDir(opts.Source, dir); sourceErr != nil {
				diags.Errorf(diag.OptionsAssetRoot, diag.Position{}, "", "cannot open the asset directory %q: %v", dir, sourceErr)
				diags.Sort()
				return nil, diags, nil
			}
			continue
		}
		// The registry keeps the opened root for the conversion, so the check
		// does not open and close the same directory twice.
		if _, rootErr := reg.Root(dir); rootErr != nil {
			diags.Errorf(diag.OptionsAssetRoot, diag.Position{}, "", "cannot open the asset directory %q: %v", dir, rootErr)
			diags.Sort()
			return nil, diags, nil
		}
	}
	sheets, sheetDiags := profile.CollectStylesheets(doc, profileOpts)
	strictDiags := profile.ValidateSheets(doc, profileOpts, sheets, nil)
	diags = append(diags, sheetDiags...)
	diags = append(diags, strictDiags...)
	if failed, ctxErr := budgetFailure(guard, &diags); failed {
		return nil, diags, ctxErr
	}
	diags.Sort()
	if diags.HasErrors() {
		return nil, diags, nil
	}
	pageStyle := layout.ParsePageStyle(sheets, pageSize, margin)
	registry, err := newFontRegistry(ctx, guard, sheets, opts, reg)
	if err != nil {
		if failed, ctxErr := budgetFailure(guard, &diags); failed {
			return nil, diags, ctxErr
		}
		if errors.Is(err, budget.ErrExceeded) {
			diags.Errorf(diag.ResourceLimit, aggregatePosition(opts), "", "%v", err)
			return nil, diags, nil
		}
		var srcErr *fontSourceError
		if errors.As(err, &srcErr) {
			diags.Errorf(diag.FontUnavailable, srcErr.pos, "", "%v", err)
			return nil, diags, nil
		}
		diags.Errorf(diag.FontUnavailable, diag.Position{}, "", "%v", err)
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
		title:     title,
		author:    author,
		subject:   subject,
		keywords:  keywords,
		assets:    reg,
	}, diags, nil
}

// validateMetadata checks the document metadata that Convert writes into the
// PDF info dictionary. It runs in preparation, so Validate reports the same
// options/title, options/author, options/subject and options/keywords
// diagnostics as Convert; serialisation is the only stage Validate leaves out.
func validateMetadata(doc *html.Document, opts Options, diags *diag.List) (title, author, subject, keywords string) {
	title = documentTitle(doc, opts)
	author = metaContent(doc, "author")
	subject = metaContent(doc, "description")
	keywords = metaContent(doc, "keywords")
	for _, meta := range []struct {
		code  diag.Code
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
	return title, author, subject, keywords
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
		diags.Errorf(diag.ResourceLimit, diag.Position{}, "", "%v", err)
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
		source:         opts.Source,
		maxPixels:      limits.MaxImagePixels,
		maxBytes:       limits.MaxImageBytes,
		maxTotalBytes:  limits.MaxTotalImageBytes,
		maxTotalPixels: limits.MaxTotalImagePixels,
		guard:          p.guard,
		reg:            p.assets,
	}
	defer func() { _ = images.Close() }()

	// Style computation reports a declaration that validation accepted but that
	// cannot be applied against the resolved parent font size.
	p.cascade.SetDiagnostics(diags)
	// target-counter needs a second pass: the page numbers written into the
	// table of contents can change the page breaks themselves.
	targets := map[string]int{}
	positions := map[string]diag.Position{}
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
		if err := stageFailure(p.guard, opts, &layoutDiags, diags); err != nil {
			return nil, err
		}
		if layoutDiags.HasErrors() {
			break
		}
		next, nextPositions := referencedTargets(doc, pages)
		if maps.Equal(next, targets) {
			targets = next
			positions = nextPositions
			break
		}
		targets = next
		positions = nextPositions
		if pass == maxPasses-1 {
			diags.Warnf(diag.PaginationTargetCounter, diag.Position{}, "", "target-counter did not converge after %d passes", maxPasses)
		}
	}
	for id, page := range targets {
		if page == 0 {
			diags.Warnf(diag.PaginationUnknownTarget, positions[id], "", "no element with id %q, the table of contents shows 0", id)
		}
	}
	*diags = append(*diags, layoutDiags...)
	if opts.Strict {
		diags.EscalateWarnings()
	}
	return pages, nil
}

// Convert renders a template into a PDF and returns it in Result.PDF.
func Convert(ctx context.Context, src []byte, opts Options) (Result, error) {
	var buf bytes.Buffer
	res, err := convert(ctx, src, opts, &buf, png.Encode)
	if err != nil {
		return res, err
	}
	res.PDF = buf.Bytes()
	return res, nil
}

// ConvertTo renders a template and streams the PDF to dst. It reports the same
// Result and diagnostics as Convert, with Result.PDF nil, and passes the
// caller's writer to the shared serialiser instead of buffering the document.
func ConvertTo(ctx context.Context, dst io.Writer, src []byte, opts Options) (Result, error) {
	return convert(ctx, src, opts, dst, png.Encode)
}

// convert is the one conversion pipeline. The only difference between Convert
// and ConvertTo is the sink: a buffer that becomes Result.PDF, or the caller's
// writer. The encoder is a parameter so a test can exercise the preview failure
// path without a mutable package variable; production always passes png.Encode.
func convert(ctx context.Context, src []byte, opts Options, sink io.Writer, encode func(io.Writer, image.Image) error) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	limits := opts.Limits.resolved()
	p, diags, err := prepare(ctx, src, opts)
	if err != nil {
		return Result{Diagnostics: diags}, err
	}
	// The registry opened the conversion's roots; every exit — success, a
	// layout failure or a cancelled context — releases them here.
	if p != nil {
		defer func() { _ = p.assets.Close() }()
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

	writer := pdf.New(pdf.Options{
		Title:        p.title,
		Author:       p.author,
		Subject:      p.subject,
		Keywords:     p.keywords,
		CreationDate: opts.Date,
		Outline:      toPDFOutline(pages.Outline),
		Budget:       p.guard,
	})
	// One pdf.Image per decoded image, shared across pages, so the writer
	// embeds a repeated source once. The digest that identifies the content is
	// hashed once per loaded image record, however many times it is placed.
	pdfImages := map[[32]byte]*pdf.Image{}
	hasher := &imageHasher{ctx: ctx, guard: p.guard, digests: map[*layout.ImageData][32]byte{}}
	for _, p := range pages.Pages {
		pdfOps, err := toPDFOps(p.Ops, pdfImages, hasher)
		if err != nil {
			if errors.Is(err, budget.ErrExceeded) {
				diags.Errorf(diag.ResourceLimit, aggregatePosition(opts), "", "%v", err)
				diags.Sort()
				return Result{Diagnostics: diags}, diags.Error()
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{Diagnostics: diags}, ctxErr
			}
			diags.Errorf(diag.PDFWrite, aggregatePosition(opts), "", "%v", err)
			diags.Sort()
			return Result{Diagnostics: diags}, diags.Error()
		}
		writer.AddPage(p.Size.Width, p.Size.Height, pdfOps)
	}
	// The preview is rendered before the PDF is written: a raster or encoding
	// failure is a conversion failure, and ConvertTo must not leave a complete
	// document in the caller's writer when it returns an error.
	var pngData []byte
	if opts.RenderPNG && len(pages.Pages) > 0 {
		pngData, err = renderPreview(pages.Pages[0], previewOptions{
			limits: limits,
			guard:  p.guard,
		}, encode)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{Diagnostics: diags}, ctxErr
			}
			if errors.Is(err, budget.ErrExceeded) {
				diags.Errorf(diag.ResourceLimit, aggregatePosition(opts), "", "%v", err)
				diags.Sort()
				return Result{Diagnostics: diags}, diags.Error()
			}
			diags.Errorf(diag.RasterRender, aggregatePosition(opts), "", "%v", err)
			diags.Sort()
			return Result{Diagnostics: diags}, diags.Error()
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{Diagnostics: diags}, err
	}
	if _, err := writer.WriteTo(sink); err != nil {
		if errors.Is(err, budget.ErrExceeded) {
			diags.Errorf(diag.ResourceLimit, aggregatePosition(opts), "", "%v", err)
			diags.Sort()
			return Result{Diagnostics: diags}, diags.Error()
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{Diagnostics: diags}, ctxErr
		}
		diags.Errorf(diag.PDFWrite, aggregatePosition(opts), "", "%v", err)
		diags.Sort()
		return Result{Diagnostics: diags}, diags.Error()
	}
	diags.Sort()
	return Result{PNG: pngData, Diagnostics: diags}, nil
}

// previewDPI is the resolution of the debug preview. It is part of the
// documented contract of Options.RenderPNG, so it is named instead of being a
// literal in the raster call.
const previewDPI = 96

// previewOptions is what renderPreview needs: the conversion's limits bound the
// raster, and the work guard bounds the raster work and reports cancellation.
type previewOptions struct {
	limits Limits
	guard  *budget.Guard
}

// renderPreview rasterizes page into the debug preview and encodes it with
// encode. The production conversion passes png.Encode; a test passes a failing
// encoder, so the failure path is exercised without mutable global state. A
// raster failure is returned with its budget error intact, and an encoding
// failure is wrapped so the conversion can report it as a raster/render
// diagnostic.
func renderPreview(page *layout.Page, opts previewOptions, encode func(io.Writer, image.Image) error) ([]byte, error) {
	img, err := raster.Render(page, raster.Options{
		DPI:       previewDPI,
		MaxPixels: opts.limits.MaxRasterPixels,
		Budget:    opts.guard,
	})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding the preview PNG: %w", err)
	}
	return buf.Bytes(), nil
}

// stageFailure moves a budget failure recorded by layout into the conversion
// diagnostics and returns the context error when the conversion was cancelled.
func stageFailure(guard *budget.Guard, opts Options, stage, diags *diag.List) error {
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
	diags.Errorf(diag.ResourceLimit, aggregatePosition(opts), "", "%v", err)
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

// referencedTargets maps every anchor a target-counter consumed to the page it
// landed on, together with the anchor's source position. Unknown targets get
// page 0. An ordinary href="#id" that no target-counter uses is not included,
// and href="#" is ignored.
func referencedTargets(doc *html.Document, pages *layout.Document) (map[string]int, map[string]diag.Position) {
	out := map[string]int{}
	positions := map[string]diag.Position{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Tag == "a" && len(pages.Targets) > 0 {
			if href, ok := n.Attr("href"); ok {
				id := strings.TrimPrefix(href, "#")
				if id != "" && pages.Targets[id] {
					out[id] = pages.Anchors[id].Page
					positions[id] = doc.Pos(n)
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc.Root)
	return out, positions
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

// imageHasher computes one content digest per loaded image record. The digest
// identifies the encoded bytes, so two files with identical content share one
// PDF image object; the hash runs once per record, however many pages place it.
type imageHasher struct {
	ctx     context.Context
	guard   *budget.Guard
	digests map[*layout.ImageData][32]byte
	// hashed counts the bytes actually hashed, so a test can prove that a
	// thousand placements scan the image once.
	hashed int64
}

// digest returns the content digest of one loaded image, hashing it the first
// time. The bytes are hashed in bounded chunks: the work is charged before each
// chunk is read and the context is checked between chunks, so a budget failure
// or a cancellation stops before the remaining bytes are scanned.
func (h *imageHasher) digest(img *layout.ImageData) ([32]byte, error) {
	if d, ok := h.digests[img]; ok {
		return d, nil
	}
	const chunk = 256 << 10
	sum := sha256.New()
	for start := 0; start < len(img.Data); start += chunk {
		if err := h.ctx.Err(); err != nil {
			return [32]byte{}, err
		}
		end := start + chunk
		if end > len(img.Data) {
			end = len(img.Data)
		}
		if err := h.guard.Add(uint64(end-start)/1024 + 1); err != nil {
			return [32]byte{}, err
		}
		sum.Write(img.Data[start:end])
		h.hashed += int64(end - start)
	}
	var out [32]byte
	copy(out[:], sum.Sum(nil))
	h.digests[img] = out
	return out, nil
}

func toPDFOps(ops []layout.Op, images map[[32]byte]*pdf.Image, hasher *imageHasher) ([]pdf.Op, error) {
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
			// one PDF object. The digest is computed once per loaded record: a
			// large image placed on every page is hashed once, not once per
			// placement, and the work is charged before the bytes are read.
			key, err := hasher.digest(img)
			if err != nil {
				return nil, err
			}
			shared, ok := images[key]
			if !ok {
				shared = &pdf.Image{
					Data:    img.Data,
					Format:  img.Format,
					PixelW:  img.PixelW,
					PixelH:  img.PixelH,
					Decoded: img.Decoded,
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
	return out, nil
}
