package engine

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"strings"

	"github.com/vyachin/go-pdf/internal/backend/raster"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/layout"
	"github.com/vyachin/go-pdf/internal/pdf"
	"github.com/vyachin/go-pdf/internal/profile"
)

// Convert renders a template into a PDF.
func Convert(ctx context.Context, src []byte, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	doc, err := html.Parse(src, sourceName(opts))
	if err != nil {
		return Result{}, err
	}
	profileOpts := profile.Options{BaseDir: opts.BaseDir, Strict: opts.Strict}
	sheets, sheetDiags := profile.CollectStylesheets(doc, profileOpts)
	diags := profile.ValidateSheets(doc, profileOpts, sheets, sheetDiags)
	if diags.HasErrors() {
		return Result{Diagnostics: diags}, diags.Error()
	}
	cascade := css.NewCascade(append([]*css.Stylesheet{css.DefaultStylesheet()}, sheets...)...)

	pageSize, err := ParsePageSize(opts.PageSize)
	if err != nil {
		diags.Errorf("options/page-size", diag.Position{}, "", "%v", err)
		return Result{Diagnostics: diags}, diags.Error()
	}
	margin, err := ParseMargin(opts.Margin)
	if err != nil {
		diags.Errorf("options/margin", diag.Position{}, "", "%v", err)
		return Result{Diagnostics: diags}, diags.Error()
	}
	pageStyle := layout.ParsePageStyle(sheets, pageSize, margin)

	registry, err := newFontRegistry(sheets, opts)
	if err != nil {
		diags.Errorf("font/unavailable", diag.Position{}, "", "%v", err)
		return Result{Diagnostics: diags}, diags.Error()
	}
	images := &imageLoader{baseDir: opts.BaseDir}

	// target-counter needs a second pass: the page numbers written into the
	// table of contents can change the page breaks themselves.
	targets := map[string]int{}
	var pages *layout.Document
	var layoutDiags diag.List
	const maxPasses = 4
	for pass := 0; pass < maxPasses; pass++ {
		pages, layoutDiags = layout.Run(doc, layout.Context{
			Fonts:       registry,
			Images:      images,
			PageSize:    pageSize,
			Margin:      margin,
			Cascade:     cascade,
			PageStyle:   pageStyle,
			TargetPages: targets,
		})
		if layoutDiags.HasErrors() {
			break
		}
		next := referencedTargets(doc, pages)
		if sameTargets(next, targets) {
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
	diags = append(diags, layoutDiags...)
	if opts.Strict {
		diags.EscalateWarnings()
	}
	if diags.HasErrors() {
		diags.Sort()
		return Result{Diagnostics: diags}, diags.Error()
	}

	writer := pdf.New(pdf.Options{
		Title:        documentTitle(doc, opts),
		Author:       metaContent(doc, "author"),
		Subject:      metaContent(doc, "description"),
		CreationDate: opts.Date,
		Outline:      toPDFOutline(pages.Outline),
	})
	// One pdf.Image per decoded image, shared across pages, so the writer
	// embeds a repeated source once.
	pdfImages := map[*layout.ImageData]*pdf.Image{}
	for _, p := range pages.Pages {
		writer.AddPage(p.Size.Width, p.Size.Height, toPDFOps(p.Ops, pdfImages))
	}
	data, err := writer.Bytes()
	if err != nil {
		diags.Errorf("pdf/write", diag.Position{}, "", "%v", err)
		diags.Sort()
		return Result{Diagnostics: diags}, diags.Error()
	}
	var pngData []byte
	if opts.RenderPNG && len(pages.Pages) > 0 {
		img := raster.Render(pages.Pages[0], raster.Options{DPI: 96})
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil {
			pngData = buf.Bytes()
		}
	}
	diags.Sort()
	return Result{PDF: data, PNG: pngData, Diagnostics: diags}, nil
}

// ParsePageSize parses A4, A5, Letter or an explicit "<w>x<h><unit>" size.
func ParsePageSize(s string) (css.PageSize, error) {
	if strings.TrimSpace(s) == "" {
		s = "A4"
	}
	if i := strings.IndexAny(s, "xX"); i > 0 {
		width, errW := css.ParseLengthString(strings.TrimSpace(s[:i]))
		height, errH := css.ParseLengthString(strings.TrimSpace(s[i+1:]))
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

func sameTargets(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
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

func toPDFOps(ops []layout.Op, images map[*layout.ImageData]*pdf.Image) []pdf.Op {
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
			shared, ok := images[img]
			if !ok {
				shared = &pdf.Image{
					Data:   img.Data,
					Format: img.Format,
					PixelW: img.PixelW,
					PixelH: img.PixelH,
				}
				images[img] = shared
			}
			out = append(out, pdf.Op{
				Kind:  pdf.ImageOpKind,
				Rect:  pdf.Rect{X: op.Rect.X, Y: op.Rect.Y, W: op.Rect.W, H: op.Rect.H},
				Image: shared,
			})
		}
	}
	return out
}
