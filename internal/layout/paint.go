package layout

import (
	"fmt"
	"sort"

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
		if b.Kind == Table && bottom > f.bottom {
			// A table's bottom padding and border belong to the page the table
			// ends on: paint them up to the page bottom and report the clip
			// instead of drawing past the MediaBox. afterBlock still advances
			// the flow by the declared insets, so following content starts on
			// the next page; only the paint is clipped.
			bottom = f.bottom
			l.warnOverflow(b, "the table's bottom padding and border are clipped at the page bottom")
		}
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
			Rect:  Rect{X: b.x, Y: top, W: b.width, H: satSub(bottom, top)},
			Color: b.Style.Background,
		})
	}
	if b.Background != nil && bottom > top {
		if op, ok := backgroundOp(b.Background, Rect{X: b.x, Y: top, W: b.width, H: satSub(bottom, top)}); ok {
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
		paintBorder(2, Rect{X: b.x, Y: satSub(bottom, bb), W: b.width, H: bb})
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
	leading := satSub(boxH, satAdd(it.Face.Ascent, it.Face.Descent))
	if leading < 0 {
		leading = 0
	}
	inBox := satAdd(leading/2, it.Face.Ascent)
	switch it.Style.VerticalAlign {
	case css.VATop:
		return inBox
	case css.VABottom:
		return satAdd(satSub(line.Height, boxH), inBox)
	case css.VAMiddle:
		return satAdd(satSub(line.Height, boxH)/2, inBox)
	default:
		return line.Ascent
	}
}

// paintLineAt draws one line of text. unstretched suppresses justification: the
// caller sets it for the paragraph's final line and for the last line a page or
// a table fragment paints.
// paintShapedRun emits one shaped run at x on the given baseline, preserving
// the shaper's glyph offsets. A glyph with an offset is drawn as its own text
// object at pen+offset, so the PDF writer needs no vertical text state, while
// the surrounding glyphs still share one pen: advances, kerning and text
// extraction do not change. Every painted shaped run — body text, margin boxes
// and list markers — goes through here, so a positioning fix has one home.
func (l *layouter) paintShapedRun(page *Page, face *FontFace, color css.Color, run text.Run, x, baseline int32) {
	glyphs := run.Glyphs
	if hasGlyphOffset(glyphs) {
		// A shaped run that needs mark offsets cannot be painted from the
		// cached slice: the offset glyphs become their own painted segments.
		// The copy is exact-sized and charged one unit per glyph before it is
		// allocated, so a budget that refuses it paints nothing.
		if err := l.ctx.Budget.Add(uint64(len(glyphs))); err != nil {
			_ = l.stopReason()
			return
		}
		copied := make([]Glyph, len(glyphs))
		copy(copied, glyphs)
		glyphs = copied
	}
	emit := func(part []Glyph, gx, gy int32) {
		if len(part) == 0 {
			return
		}
		var width int32
		for _, g := range part {
			width = satAdd(width, g.Advance)
		}
		l.ops++
		page.Ops = append(page.Ops, Op{
			Kind: TextOpKind,
			Text: &TextOp{
				X:     gx,
				Y:     gy,
				Face:  face,
				Run:   TextRun{Glyphs: part, Width: width},
				Color: color,
			},
		})
	}
	// Consecutive glyphs without an offset share one pen and one painted
	// segment; an offset glyph is flushed as its own segment at the shaper's
	// position. Both are subslices of glyphs, so the splitting allocates
	// nothing.
	pen, segStart, segBegin := x, x, 0
	flush := func(end int) {
		emit(glyphs[segBegin:end], segStart, baseline)
		segBegin = end
	}
	for i, g := range glyphs {
		if g.XOffset != 0 || g.YOffset != 0 {
			flush(i)
			emit(glyphs[i:i+1], satAdd(pen, g.XOffset), satSub(baseline, g.YOffset))
			segBegin = i + 1
		} else if i == segBegin {
			segStart = pen
		}
		pen = satAdd(pen, g.Advance)
	}
	flush(len(glyphs))
}

// hasGlyphOffset reports whether any glyph of the run carries a mark offset.
func hasGlyphOffset(glyphs []text.Glyph) bool {
	for _, g := range glyphs {
		if g.XOffset != 0 || g.YOffset != 0 {
			return true
		}
	}
	return false
}

func (l *layouter) paintLineAt(page *Page, b *Box, line *Line, x, y, width int32, unstretched bool) {
	if lineHasRightToLeft(line) {
		l.paintVisualLine(page, b, line, x, y, width, unstretched)
		return
	}
	st := b.Style
	x = satAdd(x, line.Indent)
	// Natural already contains the indent, so the free space is measured from
	// the full content width. Subtracting the indent again shifted every
	// right-aligned and centred line that had a text-indent.
	extra := satSub(width, line.Natural)
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
		x = satAdd(x, extra)
	case css.AlignCenter:
		x = satAdd(x, extra/2)
	case css.AlignJustify:
		// unstretched covers the paragraph's own last line and the last line
		// painted on a page; a line ended by a forced break keeps its width too.
		if !unstretched && !line.ForcedBreak && spaces > 0 && extra > 0 {
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
				w = satAdd(w, share)
				justify = satSub(justify, share)
			}
			baseline := satAdd(y, inlineBaseline(line, it))
			if len(it.Run.Glyphs) > 0 {
				l.paintShapedRun(page, it.Face, it.Color, it.Run, x, baseline)
			}
			if it.Style != nil {
				l.paintDecoration(page, x, w, baseline, it.Face, it.Style.TextDecoration, it.Color)
			}
			x = satAdd(x, w)
		}
	}
}

// lineHasRightToLeft reports whether a line contains a right-to-left
// character. A line without one is painted by the original left-to-right path,
// so a Latin-only document keeps its exact bytes.
func lineHasRightToLeft(line *Line) bool {
	for _, it := range line.Items {
		for _, g := range it.Run.Glyphs {
			if text.IsRightToLeft(g.Rune) {
				return true
			}
		}
	}
	return false
}

// lineUnitKind classifies one indivisible painted piece of a line.
type lineUnitKind int

const (
	glyphUnit lineUnitKind = iota
	spaceUnit
	imageUnit
)

// lineUnit is one cluster of glyphs, one space or one inline image. A cluster
// is a base glyph with the combining marks attached to it: the shaper's mark
// offsets are relative to the cluster's own pen, so moving the cluster keeps
// the mark over its base. The units are kept in logical order; x is assigned
// from the visual order before anything is painted.
type lineUnit struct {
	kind   lineUnitKind
	item   *LineItem
	glyphs []text.Glyph
	w      int32
	x      int32
	// runeStart and runeEnd bound the unit in the logical line text.
	runeStart, runeEnd int
}

// buildLineUnits splits the line into clusters, spaces and images and returns
// the logical text they spell. Each rune of the text describes one glyph, and
// an image contributes the object replacement character so the bidi pass gives
// it the direction of its neighbours.
func buildLineUnits(items []LineItem) ([]lineUnit, string) {
	units := make([]lineUnit, 0, len(items))
	var logical []rune
	for i := range items {
		it := &items[i]
		switch it.Kind {
		case ImageItem:
			logical = append(logical, '\ufffc')
			units = append(units, lineUnit{
				kind:      imageUnit,
				item:      it,
				w:         it.W,
				runeStart: len(logical) - 1,
				runeEnd:   len(logical),
			})
			continue
		}
		kind := glyphUnit
		if it.Kind == SpaceItem {
			kind = spaceUnit
		}
		glyphs := it.Run.Glyphs
		for start := 0; start < len(glyphs); {
			end := start + 1
			for end < len(glyphs) && text.IsMark(glyphs[end].Rune) {
				end++
			}
			var sum int64
			for k := start; k < end; k++ {
				sum += int64(glyphs[k].Advance)
				logical = append(logical, glyphs[k].Rune)
			}
			w := clampInt32(sum)
			if kind == spaceUnit {
				// The shaped width is the one the line was measured with.
				w = it.W
			}
			units = append(units, lineUnit{
				kind:      kind,
				item:      it,
				glyphs:    glyphs[start:end],
				w:         w,
				runeStart: len(logical) - (end - start),
				runeEnd:   len(logical),
			})
			start = end
		}
	}
	return units, string(logical)
}

// visualUnitOrder lists the units in the order they are painted, left to
// right. A right-to-left run is traversed backwards because its first logical
// cluster is painted on the right.
func visualUnitOrder(units []lineUnit, runs []text.VisualRun) []int {
	order := make([]int, 0, len(units))
	used := make([]bool, len(units))
	add := func(u int) {
		if !used[u] {
			used[u] = true
			order = append(order, u)
		}
	}
	for _, run := range runs {
		first := sort.Search(len(units), func(i int) bool { return units[i].runeEnd > run.Start })
		matched := make([]int, 0, run.End-run.Start)
		for i := first; i < len(units) && units[i].runeStart < run.End; i++ {
			matched = append(matched, i)
		}
		if run.RTL {
			for i := len(matched) - 1; i >= 0; i-- {
				add(matched[i])
			}
			continue
		}
		for _, i := range matched {
			add(i)
		}
	}
	// A run range that does not line up with a cluster boundary must not lose
	// a unit: keep the remaining ones in their logical order.
	for i := range units {
		add(i)
	}
	return order
}

// paintVisualLine places a line that contains right-to-left text. The units
// keep their logical order in the content stream, so the text layer extracts
// the authored sequence, while their x positions come from the visual order
// the bidi pass computed. A right-to-left cluster is therefore painted to the
// left of the cluster that follows it logically.
func (l *layouter) paintVisualLine(page *Page, b *Box, line *Line, x, y, width int32, unstretched bool) {
	st := b.Style
	x = satAdd(x, line.Indent)
	extra := satSub(width, line.Natural)
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
		x = satAdd(x, extra)
	case css.AlignCenter:
		x = satAdd(x, extra/2)
	case css.AlignJustify:
		if !unstretched && !line.ForcedBreak && spaces > 0 && extra > 0 {
			justify = extra
		}
	}
	units, logical := buildLineUnits(line.Items)
	order := visualUnitOrder(units, text.VisualRuns(logical, line.BaseDir))
	cursor := int64(x)
	remaining := spaces
	for _, u := range order {
		unit := &units[u]
		w := unit.w
		if unit.kind == spaceUnit && justify > 0 && remaining > 0 {
			share := justify / int32(remaining)
			remaining--
			w = satAdd(w, share)
			justify = satSub(justify, share)
		}
		unit.x = clampInt32(cursor)
		unit.w = w
		cursor += int64(w)
	}
	l.paintLineUnits(page, line, units, y)
}

// paintLineUnits paints every unit at its assigned x. The loop follows the
// logical order of the units, not the visual one, so the content stream and
// therefore the text layer keep the authored sequence.
func (l *layouter) paintLineUnits(page *Page, line *Line, units []lineUnit, y int32) {
	for i := range units {
		unit := &units[i]
		baseline := satAdd(y, inlineBaseline(line, *unit.item))
		if unit.kind == imageUnit {
			if unit.item.Image != nil {
				l.ops++
				page.Ops = append(page.Ops, Op{
					Kind:  ImageOpKind,
					Rect:  Rect{X: unit.x, Y: baseline, W: unit.w, H: unit.item.H},
					Image: &ImageOp{Image: unit.item.Image},
				})
			}
			continue
		}
		if len(unit.glyphs) > 0 {
			l.paintShapedRun(page, unit.item.Face, unit.item.Color, text.Run{Glyphs: unit.glyphs, Width: unit.w}, unit.x, baseline)
		}
		if unit.item.Style != nil {
			l.paintDecoration(page, unit.x, unit.w, baseline, unit.item.Face, unit.item.Style.TextDecoration, unit.item.Color)
		}
	}
}

// paintShapedRunDirectional paints one shaped run in the visual order of its
// own base direction. It is the path page margin boxes use, because their
// label is a single run rather than a built line.
func (l *layouter) paintShapedRunDirectional(page *Page, face *FontFace, color css.Color, run text.Run, x, baseline int32, base text.Direction) {
	hasRTL := false
	for _, g := range run.Glyphs {
		if text.IsRightToLeft(g.Rune) {
			hasRTL = true
			break
		}
	}
	if !hasRTL {
		l.paintShapedRun(page, face, color, run, x, baseline)
		return
	}
	empty := &css.Style{}
	items := []LineItem{{Kind: TextItem, Face: face, Run: run, Color: color, Style: empty, W: run.Width}}
	line := Line{Items: items, BaseDir: base}
	units, logical := buildLineUnits(line.Items)
	order := visualUnitOrder(units, text.VisualRuns(logical, base))
	cursor := int64(x)
	for _, u := range order {
		unit := &units[u]
		unit.x = clampInt32(cursor)
		cursor += int64(unit.w)
	}
	l.paintLineUnits(page, &line, units, baseline)
}

// paintDecoration draws the decoration of one painted run: a rectangle below
// the baseline for underline and one across the middle of the x-height for
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
