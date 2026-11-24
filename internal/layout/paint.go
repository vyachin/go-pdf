package layout

import (
	"fmt"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/text"
)

// finishFragment paints the background and borders of the fragment that ends
// at the current vertical position.
func (l *layouter) finishFragment(f *flow, b *Box, completed bool) {
	top := b.fragStartY
	bottom := f.y
	if completed {
		bottom += b.padBottom() + b.borderBottom()
	}
	if bottom < top {
		bottom = top
	}
	first := b.fragments == 0
	b.fragments++
	page := f.page
	// Decorations are inserted where the fragment started so that they stay
	// behind the content that was painted afterwards.
	ops := l.decorationOps(b, top, bottom, first, completed)
	if len(ops) > 0 {
		l.ops += len(ops)
		page.Ops = insertOps(page.Ops, b.fragOpStart, ops)
	}
}

// decorationOps returns the background and border fills of one box fragment.
// first and completed select the border sides that this fragment owns.
func (l *layouter) decorationOps(b *Box, top, bottom int32, first, completed bool) []Op {
	var ops []Op
	if b.Style.Background.Set && bottom > top {
		ops = append(ops, Op{
			Kind:  FillOpKind,
			Rect:  Rect{X: b.x, Y: top, W: b.width, H: bottom - top},
			Color: b.Style.Background,
		})
	}
	paintBorder := func(side int, rect Rect) {
		border := b.Style.Border[side]
		w := visibleWidth(border)
		if w <= 0 || rect.W <= 0 || rect.H <= 0 {
			return
		}
		color := border.Color
		if !color.Set {
			color = css.Color{Set: true}
		}
		ops = append(ops, Op{Kind: FillOpKind, Rect: rect, Color: color})
	}
	bl, br := b.borderLeft(), b.borderRight()
	bt, bb := b.borderTop(), b.borderBottom()
	paintBorder(3, Rect{X: b.x, Y: top, W: bl, H: bottom - top})
	paintBorder(1, Rect{X: b.x + b.width - br, Y: top, W: br, H: bottom - top})
	if first {
		paintBorder(0, Rect{X: b.x, Y: top, W: b.width, H: bt})
	}
	if completed {
		paintBorder(2, Rect{X: b.x, Y: bottom - bb, W: b.width, H: bb})
	}
	return ops
}

// insertOps places ops at the given index of dst, keeping paint order.
func insertOps(dst []Op, at int, ops []Op) []Op {
	if at < 0 || at > len(dst) {
		at = len(dst)
	}
	out := make([]Op, 0, len(dst)+len(ops))
	out = append(out, dst[:at]...)
	out = append(out, ops...)
	out = append(out, dst[at:]...)
	return out
}

// paintLineAt draws one line of text.
func (l *layouter) paintLineAt(page *Page, b *Box, line *Line, x, y, width int32, last bool) {
	st := b.Style
	x += line.Indent
	// Natural already contains the indent, so the free space is measured from
	// the full content width. Subtracting the indent again shifted every
	// right-aligned and centred line that had a text-indent.
	extra := width - line.Natural
	if extra < 0 {
		extra = 0
	}
	spaces := 0
	for _, it := range line.Items {
		if it.Kind == SpaceItem {
			spaces++
		}
	}
	justify := int32(0)
	switch st.TextAlign {
	case css.AlignRight:
		x += extra
	case css.AlignCenter:
		x += extra / 2
	case css.AlignJustify:
		if !last && !line.ForcedBreak && spaces > 0 && extra > 0 {
			justify = extra
		}
	}
	remaining := spaces
	for _, it := range line.Items {
		switch it.Kind {
		case ImageItem:
			if it.Image != nil {
				l.ops++
				page.Ops = append(page.Ops, Op{
					Kind:  ImageOpKind,
					Rect:  Rect{X: x, Y: y + (line.Height-it.H)/2, W: it.W, H: it.H},
					Image: &ImageOp{Image: it.Image},
				})
			}
			x += it.W
		default:
			w := it.W
			if it.Kind == SpaceItem && justify > 0 && remaining > 0 {
				share := justify / int32(remaining)
				remaining--
				w += share
				justify -= share
			}
			if len(it.Run.Glyphs) > 0 {
				glyphs := make([]Glyph, len(it.Run.Glyphs))
				for i, g := range it.Run.Glyphs {
					glyphs[i] = Glyph{GID: g.GID, Advance: g.Advance, Rune: g.Rune}
				}
				l.ops++
				page.Ops = append(page.Ops, Op{
					Kind: TextOpKind,
					Text: &TextOp{
						X:     x,
						Y:     y + line.Ascent,
						Face:  it.Face,
						Run:   TextRun{Glyphs: glyphs, Width: it.Run.Width},
						Color: it.Color,
					},
				})
			}
			x += w
		}
	}
}

// paintListMarker draws the bullet or number of a list item in the paginated
// flow.
func (l *layouter) paintListMarker(f *flow, b *Box) {
	l.paintListMarkerAt(f.page, b, f.left)
}

// paintListMarkerAt draws a list marker into a page, clamped to left so it does
// not leave the containing content box. It serves both the paginated and the
// static (table cell) path.
func (l *layouter) paintListMarkerAt(page *Page, b *Box, left int32) {
	st := b.Style
	face, err := l.face(st)
	if err != nil {
		return
	}
	var label string
	switch st.ListStyleType {
	case css.ListDecimal:
		label = fmt.Sprintf("%d.", b.ListIndex)
	default:
		label = "•"
	}
	run, err := text.Shape(face.Font, face.Size, label)
	if err != nil {
		return
	}
	// The marker belongs to the list's left padding, not to the item's
	// content box: place it before the text and never over it.
	x := b.x + b.padLeft() - run.Width - 2000
	if x < left {
		x = left
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
			Y:     b.listMarkerY + run.Width*0 + face.Ascent,
			Face:  face,
			Run:   TextRun{Glyphs: glyphs, Width: run.Width},
			Color: st.Color,
		},
	})
}
