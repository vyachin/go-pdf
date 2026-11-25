package layout

import (
	"fmt"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/text"
)

// finishFragment paints the background and borders of the fragment that ends
// at the current vertical position.
func (l *layouter) finishFragment(f *flow, b *Box, completed bool) {
	// Record what this fragment consumed against a declared height, so a
	// continuation reserves only the remainder. The baseline makes the record
	// idempotent when placement retries on the same page.
	if b.Style != nil && b.Style.Height.Kind == css.MP && b.Style.Height.Value > 0 {
		b.heightConsumed = satAdd(b.consumedBase, satSub(f.y, b.contentTopY))
	}
	top := b.fragStartY
	bottom := f.y
	if completed {
		bottom = satAdd(bottom, satAdd(b.padBottom(), b.borderBottom()))
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
		page.Ops = l.insertOps(page.Ops, b.fragOpStart, ops)
		// A background or a border is content the page now carries.
		f.pageHasBody = true
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
	if b.Background != nil && bottom > top {
		if op, ok := backgroundOp(b.Background, Rect{X: b.x, Y: top, W: b.width, H: bottom - top}); ok {
			ops = append(ops, op)
		}
	}
	paintBorder := func(side int, rect Rect) {
		border := b.Style.Border[side]
		w := visibleWidth(border)
		if w <= 0 || border.Color.Transparent || rect.W <= 0 || rect.H <= 0 {
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
	paintBorder(3, Rect{X: b.x, Y: top, W: bl, H: satSub(bottom, top)})
	paintBorder(1, Rect{X: satSub(satAdd(b.x, b.width), br), Y: top, W: br, H: satSub(bottom, top)})
	if first {
		paintBorder(0, Rect{X: b.x, Y: top, W: b.width, H: bt})
	}
	if completed {
		paintBorder(2, Rect{X: b.x, Y: bottom - bb, W: b.width, H: bb})
	}
	return ops
}

// insertOps places ops at the given index of dst, keeping paint order. The
// copy is charged to the work budget; a stopped layout keeps the page as it was
// instead of paying for a decoration it will not use.
func (l *layouter) insertOps(dst []Op, at int, ops []Op) []Op {
	if err := l.ctx.Budget.Add(uint64(len(dst)) + 1); err != nil {
		_ = l.stopReason()
		return dst
	}
	if at < 0 || at > len(dst) {
		at = len(dst)
	}
	out := make([]Op, 0, len(dst)+len(ops))
	out = append(out, dst[:at]...)
	out = append(out, ops...)
	out = append(out, dst[at:]...)
	return out
}

// inlineBaseline returns the vertical position a line item contributes: the
// text baseline for a text run, the top edge for an image. Baseline-aligned
// items share the line baseline; top, middle and bottom follow the inline box
// the item occupies, so a small span can sit above, in the middle of or below
// the line without moving it.
func inlineBaseline(line *Line, it LineItem) int32 {
	if it.Kind == ImageItem {
		switch it.Style.VerticalAlign {
		case css.VATop:
			return 0
		case css.VABottom:
			return line.Height - it.H
		case css.VAMiddle:
			return (line.Height - it.H) / 2
		default:
			// An image's baseline is its bottom edge.
			return line.Ascent - it.H
		}
	}
	boxH := it.Style.LineHeightMP()
	leading := boxH - (it.Face.Ascent + it.Face.Descent)
	if leading < 0 {
		leading = 0
	}
	inBox := leading/2 + it.Face.Ascent
	switch it.Style.VerticalAlign {
	case css.VATop:
		return inBox
	case css.VABottom:
		return line.Height - boxH + inBox
	case css.VAMiddle:
		return (line.Height-boxH)/2 + inBox
	default:
		return line.Ascent
	}
}

// paintLineAt draws one line of text.
// paintShapedRun emits one shaped run at x on the given baseline, preserving
// the shaper's glyph offsets. A glyph with an offset is drawn as its own text
// object at pen+offset, so the PDF writer needs no vertical text state, while
// the surrounding glyphs still share one pen: advances, kerning and text
// extraction do not change. Every painted shaped run — body text, margin boxes
// and list markers — goes through here, so a positioning fix has one home.
func (l *layouter) paintShapedRun(page *Page, face *FontFace, color css.Color, run text.Run, x, baseline int32) {
	emit := func(glyphs []Glyph, gx, gy int32) {
		if len(glyphs) == 0 {
			return
		}
		var width int32
		for _, g := range glyphs {
			width = satAdd(width, g.Advance)
		}
		l.ops++
		page.Ops = append(page.Ops, Op{
			Kind: TextOpKind,
			Text: &TextOp{
				X:     gx,
				Y:     gy,
				Face:  face,
				Run:   TextRun{Glyphs: glyphs, Width: width},
				Color: color,
			},
		})
	}
	pen, segStart := x, x
	var seg []Glyph
	flush := func() {
		emit(seg, segStart, baseline)
		seg = nil
	}
	for _, g := range run.Glyphs {
		if g.XOffset != 0 || g.YOffset != 0 {
			flush()
			emit([]Glyph{{GID: g.GID, Advance: g.Advance, Rune: g.Rune}},
				satAdd(pen, g.XOffset), satSub(baseline, g.YOffset))
		} else {
			if len(seg) == 0 {
				segStart = pen
			}
			seg = append(seg, Glyph{GID: g.GID, Advance: g.Advance, Rune: g.Rune})
		}
		pen = satAdd(pen, g.Advance)
	}
	flush()
}

func (l *layouter) paintLineAt(page *Page, b *Box, line *Line, x, y, width int32, last bool) {
	st := b.Style
	x = satAdd(x, line.Indent)
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
					Rect:  Rect{X: x, Y: satAdd(y, inlineBaseline(line, it)), W: it.W, H: it.H},
					Image: &ImageOp{Image: it.Image},
				})
			}
			x = satAdd(x, it.W)
		default:
			w := it.W
			if it.Kind == SpaceItem && justify > 0 && remaining > 0 {
				share := justify / int32(remaining)
				remaining--
				w += share
				justify -= share
			}
			baseline := satAdd(y, inlineBaseline(line, it))
			if len(it.Run.Glyphs) > 0 {
				l.paintShapedRun(page, it.Face, it.Color, it.Run, x, baseline)
			}
			if it.Style != nil {
				l.paintDecoration(page, x, w, baseline, it.Face, it.Style.TextDecoration, it.Color)
			}
			x += w
		}
	}
}

// paintDecoration draws the decoration of one painted run: a rectangle below the
// baseline for underline and one across the middle of the x-height for
// line-through, both in the run colour. Adjacent runs abut — a word and the
// spaces around it advance by exactly the painted width — so the decoration
// reads as one continuous line without collecting per-line extents.
func (l *layouter) paintDecoration(page *Page, x, w, baseline int32, face *FontFace, dec css.Decoration, color css.Color) {
	if dec == css.DecorateNone || w <= 0 || face == nil || face.Size <= 0 {
		return
	}
	size := face.Size
	thickness := max(size/20, int32(50))
	if dec&css.DecorateUnderline != 0 {
		y := satAdd(baseline, max(size/10, thickness+50))
		l.ops++
		page.Ops = append(page.Ops, Op{
			Kind:  FillOpKind,
			Rect:  Rect{X: x, Y: y, W: w, H: thickness},
			Color: color,
		})
	}
	if dec&css.DecorateLineThrough != 0 {
		y := satSub(satSub(baseline, size/4), thickness/2)
		l.ops++
		page.Ops = append(page.Ops, Op{
			Kind:  FillOpKind,
			Rect:  Rect{X: x, Y: y, W: w, H: thickness},
			Color: color,
		})
	}
}

// paintListMarker draws the bullet or number of a list item in the paginated
// flow.
func (l *layouter) paintListMarker(f *flow, b *Box) {
	l.paintListMarkerAt(f.page, b, f.left)
	// The marker is content of the page even when the item has no text.
	f.pageHasBody = true
}

// paintListMarkerAt draws a list marker into a page, clamped to left so it does
// not leave the containing content box. It serves both the paginated and the
// static (table cell) path.
func (l *layouter) paintListMarkerAt(page *Page, b *Box, left int32) {
	st := b.Style
	face, err := l.face(st)
	if err != nil {
		_ = l.fontStop(err)
		return
	}
	var label string
	switch st.ListStyleType {
	case css.ListDecimal:
		label = fmt.Sprintf("%d.", b.ListIndex)
	default:
		label = "•"
	}
	run, err := l.shape(face, label)
	if err != nil {
		return
	}
	// The marker belongs to the list's left padding, not to the item's
	// content box: place it before the text and never over it.
	x := satSub(satSub(satAdd(b.x, b.padLeft()), run.Width), 2000)
	if x < left {
		x = left
	}
	l.paintShapedRun(page, face, st.Color, run, x, satAdd(b.listMarkerY, face.Ascent))
}
