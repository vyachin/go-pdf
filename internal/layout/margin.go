package layout

import (
	"sort"
	"strconv"
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/text"
)

// PageStyle is the resolved page setup from @page rules.
type PageStyle struct {
	Size   css.PageSize
	Margin [4]int32
	Boxes  map[string][]css.ContentItem
}

// ParsePageStyle resolves @page and margin box rules across all stylesheets.
// Fallback values are used for anything the document does not specify.
func ParsePageStyle(sheets []*css.Stylesheet, fallbackSize css.PageSize, fallbackMargin [4]int32) PageStyle {
	ps := PageStyle{Size: fallbackSize, Margin: fallbackMargin, Boxes: map[string][]css.ContentItem{}}
	var spec css.PageSizeSpec
	haveSize := false
	for _, sheet := range sheets {
		if sheet == nil {
			continue
		}
		for _, r := range sheet.Rules {
			if r.Kind != css.PageRule {
				continue
			}
			for _, d := range r.Decls {
				if d.Property != "size" {
					continue
				}
				parsed, err := css.ParsePageSizeSpec(d.Value)
				if err != nil {
					// The validator reports the problem; layout keeps the last
					// size it could understand.
					continue
				}
				spec = parsed
				haveSize = true
			}
		}
	}
	if haveSize {
		ps.Size = spec.Resolve(fallbackSize)
	}
	for _, sheet := range sheets {
		if sheet == nil {
			continue
		}
		for _, r := range sheet.Rules {
			switch r.Kind {
			case css.PageRule:
				for _, d := range r.Decls {
					if d.Property != "margin" {
						continue
					}
					box, err := css.ParsePageMargin(d.Value)
					if err != nil {
						continue
					}
					for i := 0; i < 4; i++ {
						ps.Margin[i] = box[i].Resolve(ps.Size.Width)
					}
				}
			case css.PageMarginRule:
				for _, d := range r.Decls {
					if d.Property != "content" {
						continue
					}
					items, err := css.ParseContent(d.Value)
					if err != nil {
						continue
					}
					ps.Boxes[r.Name] = items
				}
			}
		}
	}
	return ps
}

// paintMarginBoxes draws page headers and footers after pagination, when the
// page count is known.
func (l *layouter) paintMarginBoxes() {
	if len(l.pageStyle.Boxes) == 0 {
		return
	}
	if l.ctx.Fonts == nil {
		return
	}
	face, err := l.ctx.Fonts.Face([]string{"serif"}, 400, false, 10000)
	if err != nil {
		l.diags.Warnf("page/margin-box", diag.Position{}, "", "cannot load a font for margin boxes: %v", err)
		return
	}
	names := make([]string, 0, len(l.pageStyle.Boxes))
	for name := range l.pageStyle.Boxes {
		names = append(names, name)
	}
	sort.Strings(names)
	total := len(l.doc.Pages)
	for _, page := range l.doc.Pages {
		contentLeft := page.Margin[3]
		contentWidth := page.Size.Width - page.Margin[1] - page.Margin[3]
		for _, name := range names {
			label := l.contentString(l.pageStyle.Boxes[name], page.Number, total)
			if label == "" {
				continue
			}
			run, err := text.Shape(face.Font, face.Size, label)
			if err != nil {
				continue
			}
			var x, y int32
			switch {
			case strings.HasSuffix(name, "left"):
				x = contentLeft
			case strings.HasSuffix(name, "center"):
				x = contentLeft + (contentWidth-run.Width)/2
			default:
				x = contentLeft + contentWidth - run.Width
			}
			if strings.HasPrefix(name, "top") {
				y = page.Margin[0]/2 + face.Ascent/2
			} else {
				y = page.Size.Height - page.Margin[2]/2 + face.Ascent/2
			}
			glyphs := make([]Glyph, len(run.Glyphs))
			for i, g := range run.Glyphs {
				glyphs[i] = Glyph{GID: g.GID, Advance: g.Advance, Rune: g.Rune}
			}
			l.ops++
			page.Ops = append(page.Ops, Op{
				Kind: TextOpKind,
				Text: &TextOp{
					X:     x,
					Y:     y,
					Face:  face,
					Run:   TextRun{Glyphs: glyphs, Width: run.Width},
					Color: css.Color{Set: true},
				},
			})
		}
	}
}

func (l *layouter) contentString(items []css.ContentItem, page, total int) string {
	var b strings.Builder
	for _, it := range items {
		switch it.Kind {
		case css.ContentText:
			b.WriteString(it.Text)
		case css.ContentPageNumber:
			b.WriteString(strconv.Itoa(page))
		case css.ContentPageCount:
			b.WriteString(strconv.Itoa(total))
		case css.ContentTarget:
			// target-counter is resolved by the second layout pass; margin
			// boxes cannot reference elements, so the value is empty here.
		}
	}
	return b.String()
}
