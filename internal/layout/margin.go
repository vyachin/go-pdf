package layout

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
)

// PageStyle is the resolved page setup from @page rules.
type PageStyle struct {
	Size   css.PageSize
	Margin [4]int32
	Boxes  map[string][]css.ContentItem
	// BoxPos is the source position of the content declaration that filled
	// each margin box, used when the box generates a text diagnostic.
	BoxPos map[string]diag.Position
}

// ParsePageStyle resolves @page and margin box rules across all stylesheets.
// Fallback values are used for anything the document does not specify.
func ParsePageStyle(sheets []*css.Stylesheet, fallbackSize css.PageSize, fallbackMargin [4]int32) PageStyle {
	rules := css.RulesForPrint(sheets)
	// Page declarations cascade by importance first, then by document order.
	var sizeDecl, marginDecl *css.Declaration
	var sizeImportant, marginImportant bool
	boxDecl := map[string]*css.Declaration{}
	boxImportant := map[string]bool{}
	pick := func(current **css.Declaration, currentImportant *bool, d css.Declaration) {
		if *currentImportant && !d.Important {
			return
		}
		decl := d
		*current = &decl
		*currentImportant = d.Important
	}
	for _, r := range rules {
		switch r.Kind {
		case css.PageRule:
			for _, d := range r.Decls {
				switch d.Property {
				case "size":
					pick(&sizeDecl, &sizeImportant, d)
				case "margin":
					pick(&marginDecl, &marginImportant, d)
				}
			}
		case css.PageMarginRule:
			for _, d := range r.Decls {
				if d.Property != "content" {
					continue
				}
				if boxImportant[r.Name] && !d.Important {
					continue
				}
				decl := d
				boxDecl[r.Name] = &decl
				boxImportant[r.Name] = d.Important
			}
		}
	}

	ps := PageStyle{Size: fallbackSize, Margin: fallbackMargin, Boxes: map[string][]css.ContentItem{}, BoxPos: map[string]diag.Position{}}
	if sizeDecl != nil {
		if spec, err := css.ParsePageSizeSpec(sizeDecl.Value); err == nil {
			ps.Size = spec.Resolve(fallbackSize)
		}
	}
	if marginDecl != nil {
		if box, err := css.ParsePageMargin(marginDecl.Value); err == nil {
			for i := 0; i < 4; i++ {
				ps.Margin[i] = box[i].Resolve(ps.Size.Width)
			}
		}
	}
	names := slices.Sorted(maps.Keys(boxDecl))
	for _, name := range names {
		if items, err := css.ParseContent(boxDecl[name].Value); err == nil {
			pos := boxDecl[name].Position()
			for i := range items {
				items[i].Pos = pos
			}
			ps.Boxes[name] = items
			ps.BoxPos[name] = pos
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
		l.diags.Warnf(diag.PageMarginBox, diag.Position{}, "", "cannot load a font for margin boxes: %v", err)
		return
	}
	names := slices.Sorted(maps.Keys(l.pageStyle.Boxes))
	total := len(l.doc.Pages)
	for _, page := range l.doc.Pages {
		contentLeft := page.Margin[3]
		contentWidth := page.Size.Width - page.Margin[1] - page.Margin[3]
		for _, name := range names {
			label := l.contentString(l.pageStyle.Boxes[name], page.Number, total)
			if label == "" {
				continue
			}
			run, err := l.shape(face, label)
			if err != nil {
				// Losing a header or footer silently is not acceptable: report
				// it the same way body text reports a missing glyph, at the
				// content declaration that generated the text when known.
				l.diags.Errorf(diag.TextGlyph, l.pageStyle.BoxPos[name], name, "margin box %s: %v", name, err)
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
			l.paintShapedRun(page, face, css.Color{Set: true}, run, x, y)
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
