package layout

import (
	"math"
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
)

// flow is the pagination cursor: the current page and vertical position.
type flow struct {
	l       *layouter
	page    *Page
	pageNum int
	left    int32
	top     int32
	width   int32
	bottom  int32
	y       int32
	pending int32
	empty   bool
	// pageHasBody records that the current page has content that is not a
	// repeated table header. A forced break is taken only from such a page; on
	// a page whose only content is a repeated header the boundary is already
	// fresh. f.empty keeps its separate meaning: "nothing was entered here",
	// which the margin and break-inside decisions use.
	pageHasBody bool
	// breakPending is a forced break that waits for the next box that paints
	// content. break-before and break-after share the flag, so adjacent forced
	// breaks collapse into one transition, and an empty box neither cancels it
	// nor creates a page on its own.
	breakPending bool
}

// newPage starts a new page and reports false when the page limit is reached.
// It is the single place where a page is created, so forced breaks, tables and
// the page loop all obey the same limit.
func (f *flow) newPage() bool {
	l := f.l
	if l.maxPages > 0 && len(l.doc.Pages) >= l.maxPages {
		l.pagesExhausted = true
		l.reportPageLimit()
		return false
	}
	ps := l.pageStyle
	f.pageNum++
	p := &Page{Number: f.pageNum, Size: ps.Size, Margin: ps.Margin}
	l.doc.Pages = append(l.doc.Pages, p)
	f.page = p
	f.left = ps.Margin[3]
	f.top = ps.Margin[0]
	// Every geometry sum saturates: a margin larger than the page must not wrap
	// the content width into a negative or huge value.
	f.width = satSub(satSub(ps.Size.Width, ps.Margin[1]), ps.Margin[3])
	f.bottom = satSub(ps.Size.Height, ps.Margin[2])
	if f.width < 0 {
		f.width = 0
	}
	if f.bottom < 0 {
		f.bottom = 0
	}
	f.y = f.top
	f.pending = 0
	f.empty = true
	f.pageHasBody = false
	// Any page transition satisfies a pending forced break: the boundary it
	// asked for has been crossed.
	f.breakPending = false
	return true
}

func (f *flow) contentHeight() int32 {
	h := satSub(f.bottom, f.top)
	if h < 0 {
		// A margin larger than the page leaves no content box; a negative
		// height would make every fitting decision compare against a wrapped
		// bound.
		return 0
	}
	return h
}

// paddingBase is the width a percentage padding resolves against: the
// containing block width, not the box's own width. A box measured before
// pagination falls back to its used width.
func (b *Box) paddingBase() int32 {
	if b.containing > 0 {
		return b.containing
	}
	return b.width
}

func (b *Box) padTop() int32       { return b.Style.Padding[0].Resolve(b.paddingBase()) }
func (b *Box) padRight() int32     { return b.Style.Padding[1].Resolve(b.paddingBase()) }
func (b *Box) padBottom() int32    { return b.Style.Padding[2].Resolve(b.paddingBase()) }
func (b *Box) padLeft() int32      { return b.Style.Padding[3].Resolve(b.paddingBase()) }
func (b *Box) borderTop() int32    { return visibleWidth(b.Style.Border[0]) }
func (b *Box) borderRight() int32  { return visibleWidth(b.Style.Border[1]) }
func (b *Box) borderBottom() int32 { return visibleWidth(b.Style.Border[2]) }
func (b *Box) borderLeft() int32   { return visibleWidth(b.Style.Border[3]) }

func visibleWidth(side css.BorderSide) int32 {
	if side.Style == css.BorderNone {
		return 0
	}
	return side.Width
}

func (b *Box) contentWidth() int32 {
	w := int64(b.width) - int64(b.padLeft()) - int64(b.padRight()) -
		int64(b.borderLeft()) - int64(b.borderRight())
	if w < 0 {
		return 0
	}
	if w > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(w)
}

// isTextBlock reports whether the box only contains inline content.
func (l *layouter) isTextBlock(b *Box) bool {
	if b.Kind == Table {
		return false
	}
	for _, c := range b.Children {
		switch c.Kind {
		case Text, Inline, Image:
			continue
		default:
			return false
		}
	}
	return true
}

// paginate turns the box tree into pages.
func (l *layouter) paginate(root *Box) *Document {
	f := &flow{l: l}
	f.newPage()
	maxIterations := l.maxIterations
	if maxIterations <= 0 {
		maxIterations = 5000
	}
	iterations := 0
	for l.tick() == nil && !l.pagesExhausted {
		opsBefore := l.ops
		yBefore := f.y
		done := l.placeBlock(f, root, f.left, f.width, true)
		if done {
			break
		}
		if l.pagesExhausted {
			break
		}
		iterations++
		if l.ctx.Budget.Err() != nil {
			// The work guard stopped the layout: a diagnostic about progress
			// would misdescribe the cause the caller already reports.
			break
		}
		if l.ops == opsBefore && f.y == yBefore && !f.breakPending {
			// Nothing was drawn and the flow did not advance: the content cannot
			// be placed, so fail instead of emitting empty pages. A pending
			// forced break is the one case where a page is expected.
			l.diags.Errorf(diag.LayoutNoProgress, diag.Position{}, "", "content cannot be placed on a page and pagination cannot advance")
			break
		}
		if iterations > maxIterations {
			l.diags.Errorf(diag.LayoutLoop, diag.Position{}, "", "pagination did not converge")
			break
		}
		f.newPage()
	}
	l.paintMarginBoxes()
	l.doc.Diags = append(l.doc.Diags, l.diags...)
	l.doc.Diags.Sort()
	return l.doc
}

// registerAnchor records the id and heading bookmark of a box at the position
// where it is placed. The paginated and the static (table cell) path share it.
func (l *layouter) registerAnchor(b *Box, page int, y int32) {
	if b.Node == nil || b.anchored {
		return
	}
	b.anchored = true
	if id := b.Node.ID(); id != "" {
		if _, ok := l.anchors[id]; !ok {
			l.anchors[id] = Anchor{Page: page, Y: y}
		}
	}
	if level := headingLevel(b.Node.Tag); level > 0 {
		l.doc.Outline = append(l.doc.Outline, OutlineEntry{
			Title: b.Node.TextContent(),
			Page:  page,
			Y:     y,
			Level: level,
		})
	}
}

// usedGeometry applies the horizontal margins and the CSS width of a block to
// the available width, setting b.x and b.width. CSS width is the content width,
// so the outer width adds padding and borders. Pagination and the static path
// inside table cells share it.
func (b *Box) usedGeometry(x, width int32) {
	b.containing = width
	w, overflow := usedWidthChecked(b.Style, width)
	b.width = w
	b.geometryOverflow = overflow
	bx := int64(x) + int64(b.Style.Margin[3].Resolve(width))
	if bx > math.MaxInt32 || bx < math.MinInt32 {
		b.geometryOverflow = true
	}
	b.x = clampInt32(bx)
}

// usedWidth returns the width a box would use inside a containing block of the
// given width: its margins are removed, and a declared width is honoured. Both
// placement and measurement go through it, so a text block inside a table cell
// wraps at the width its declarations ask for.
func usedWidth(st *css.Style, width int32) int32 {
	w, _ := usedWidthChecked(st, width)
	return w
}

// usedWidthChecked applies the margins and the declared width of a style inside
// a containing block. Every sum is computed in 64-bit; the flag reports that an
// intermediate value did not fit in the int32 geometry, so the caller can
// diagnose the clamp.
func usedWidthChecked(st *css.Style, containing int32) (int32, bool) {
	overflow := false
	mark := func(v int64) int64 {
		if v > math.MaxInt32 || v < math.MinInt32 {
			overflow = true
		}
		return v
	}
	marginLeft := st.Margin[3].Resolve(containing)
	marginRight := st.Margin[1].Resolve(containing)
	avail := mark(int64(containing) - int64(marginLeft) - int64(marginRight))
	if avail < 0 {
		avail = 0
	}
	// Percentage padding resolves against the containing block width, not
	// against what is left after the margins or against the box's own width.
	extra := mark(int64(st.Padding[1].Resolve(containing)) + int64(st.Padding[3].Resolve(containing)) +
		int64(visibleWidth(st.Border[1])) + int64(visibleWidth(st.Border[3])))
	var w int64
	switch st.Width.Kind {
	case css.MP:
		w = mark(int64(st.Width.Value) + extra)
	case css.Percent:
		w = mark(int64(st.Width.Resolve(containing)) + extra)
	default:
		w = avail
	}
	if w < 0 {
		w = 0
	}
	if w > avail {
		w = avail
	}
	return int32(w), overflow
}

// placeBlock places as much of b as fits and reports whether it is complete.
func (l *layouter) placeBlock(f *flow, b *Box, x, width int32, isRoot bool) bool {
	if b.cursor.done {
		return true
	}
	st := b.Style
	if !b.started {
		b.started = true
		b.startPage = f.pageNum
		b.usedGeometry(x, width)
		if b.geometryOverflow {
			l.warnOverflow(b, "the box's geometry exceeds the page bounds and is clamped")
		}
	}
	cw := b.contentWidth()

	if !b.entered {
		// The single forced-break decision: a break, whether it was requested
		// by a break-before on this box or deferred by a break-after on the
		// previous one, is consumed here by the first box that paints
		// something. A box that paints nothing leaves it pending, so an empty
		// or height-only <div> cannot cancel a break and cannot create a page
		// by itself.
		if !isRoot && f.breakPending && boxHasContent(b) {
			if f.pageHasBody {
				if !f.newPage() {
					return false
				}
			} else {
				f.breakPending = false
			}
		}
		if !isRoot {
			mt := st.Margin[0].Resolve(width)
			if f.pending > mt {
				mt = f.pending
			}
			f.pending = 0
			if !f.empty {
				f.y = satAdd(f.y, mt)
			}
		}
		if st.BreakInside == css.BreakAvoid && !f.empty {
			// Only a box that fits an empty page can be kept together. A box
			// taller than the content height will be split anyway, so moving to
			// a new page would only add an empty page before it; and a refused
			// transition must stop placement instead of retrying on the page it
			// did not create.
			if h := l.measure(b, width); int64(f.y)+int64(h) > int64(f.bottom) && h <= f.contentHeight() {
				if !f.newPage() {
					return false
				}
			}
		}
		if l.isTextBlock(b) {
			l.buildLines(b, cw)
			// Only a page that already painted something can be advanced; on an
			// empty page the orphans rule would loop between pages forever.
			if len(f.page.Ops) > 0 {
				startY := satAdd(f.y, satAdd(b.borderTop(), b.padTop()))
				avail := satSub(satSub(f.bottom, b.padBottom()), b.borderBottom())
				n := countFitting(b.lines, 0, startY, avail)
				if n < len(b.lines) && n < st.Orphans {
					// Move the whole block to the next page.
					b.entered = false
					return false
				}
			}
		}
		b.entered = true
		b.fragStartY = f.y
		b.fragStartPage = f.pageNum
		b.fragOpStart = len(f.page.Ops)
		l.registerAnchor(b, f.pageNum, f.y)
		f.y = satAdd(f.y, satAdd(b.borderTop(), b.padTop()))
		b.contentTopY = f.y
		b.consumedBase = b.heightConsumed
		if b.Kind == ListItem && b.cursor.child == 0 && b.cursor.line == 0 {
			b.listMarkerY = f.y
			l.paintListMarker(f, b)
		}
	}
	if b.fragStartPage != f.pageNum {
		// A continuation on a new page: the fragment starts here, with no top
		// border and padding.
		b.fragStartPage = f.pageNum
		b.fragStartY = f.y
		b.fragOpStart = len(f.page.Ops)
	}
	f.empty = false

	switch {
	case b.Kind == Table:
		if l.placeTable(f, b, cw) {
			l.applyFixedHeight(f, b)
			l.finishFragment(f, b, true)
			l.afterBlock(f, b)
			return true
		}
		l.finishFragment(f, b, false)
		return false
	case b.Kind == Image:
		if l.placeImageBlock(f, b, cw) {
			l.applyFixedHeight(f, b)
			l.finishFragment(f, b, true)
			l.afterBlock(f, b)
			return true
		}
		l.finishFragment(f, b, false)
		return false
	case l.isTextBlock(b):
		if l.placeText(f, b, cw) {
			l.applyFixedHeight(f, b)
			l.finishFragment(f, b, true)
			l.afterBlock(f, b)
			return true
		}
		l.finishFragment(f, b, false)
		return false
	default:
		for b.cursor.child < len(b.Children) {
			c := b.Children[b.cursor.child]
			// Children start inside the parent content box.
			if c.indentBase == 0 {
				c.indentBase = b.containing
			}
			cx, overflow := satAddChecked(satAdd(b.x, b.borderLeft()), b.padLeft())
			if overflow {
				c.geometryOverflow = true
				l.warnOverflow(c, "the box's geometry exceeds the page bounds and is clamped")
			}
			if !l.placeChild(f, c, cx, cw) {
				l.finishFragment(f, b, false)
				return false
			}
			b.cursor.child++
		}
		l.applyFixedHeight(f, b)
		l.finishFragment(f, b, true)
		l.afterBlock(f, b)
		return true
	}
}

// boxHasContent reports whether a box can paint anything visible or reserve
// vertical space. It is the predicate that consumes a pending forced break, so
// it covers a height, a background, a border, a list marker, an image and
// descendants that paint: a container of empty boxes paints nothing.
func boxHasContent(b *Box) bool {
	switch b.Kind {
	case Image:
		return b.Image != nil
	case Table:
		return len(b.Rows) > 0
	case Text:
		return strings.TrimSpace(b.Text) != ""
	case ListItem:
		// The marker is painted even when the item has no text.
		return true
	}
	if st := b.Style; st != nil {
		if st.Height.Kind == css.MP && st.Height.Value > 0 {
			return true
		}
		if st.Background.Set || b.Background != nil {
			return true
		}
		for _, side := range st.Border {
			if visibleWidth(side) > 0 {
				return true
			}
		}
	}
	for _, c := range b.Children {
		if boxHasContent(c) {
			return true
		}
	}
	return false
}

func (l *layouter) placeChild(f *flow, c *Box, x, width int32) bool {
	// break-before joins the pending decision in placeBlock; it must not start
	// a page here, or a break-after on the previous box would start a second
	// one at the same boundary.
	if c.Style.BreakBefore == css.BreakPage {
		f.breakPending = true
	}
	if c.Style.BreakBefore != css.BreakPage && !f.empty && c.Style.BreakInside == css.BreakAvoid {
		h := l.measure(c, width)
		if int64(f.y)+int64(h) > int64(f.bottom) && h <= f.contentHeight() {
			// newPage clears any pending break: the transition satisfies it.
			if !f.newPage() {
				return false
			}
		}
	}
	return l.placeBlock(f, c, x, width, false)
}

func (l *layouter) afterBlock(f *flow, b *Box) {
	// Bottom padding and border are part of the flow after a completed block.
	f.y = satAdd(f.y, satAdd(b.padBottom(), b.borderBottom()))
	b.cursor.done = true
	// A percentage bottom margin resolves against the containing block width,
	// like every other margin, not against this box's own width.
	f.pending = b.Style.Margin[2].Resolve(b.paddingBase())
	if b.Style.BreakAfter == css.BreakPage {
		// Create the page only when another visible box follows; otherwise a
		// trailing break would add an empty page and change counter(pages).
		f.breakPending = true
	}
}

// placeImageBlock paints a replaced block image and advances the flow.
func (l *layouter) placeImageBlock(f *flow, b *Box, width int32) bool {
	if b.Image == nil {
		return true
	}
	w, h := imageSize(b.Style, b.Image, width, l.ctx)
	avail := satSub(satSub(f.bottom, b.padBottom()), b.borderBottom())
	if int64(f.y)+int64(h) > int64(avail) {
		if len(f.page.Ops) == 0 {
			l.warnOverflow(b, "an image is taller than the page and is clipped")
		} else {
			// The page loop owns page creation: creating one here as well
			// produced an empty page for every moved image.
			return false
		}
	}
	imageX, overflow := satAddChecked(satAdd(b.x, b.borderLeft()), b.padLeft())
	if overflow {
		l.warnOverflow(b, "the box's geometry exceeds the page bounds and is clamped")
	}
	l.ops++
	f.page.Ops = append(f.page.Ops, Op{
		Kind: ImageOpKind,
		Rect: Rect{
			X: imageX,
			Y: f.y,
			W: w,
			H: h,
		},
		Image: &ImageOp{Image: b.Image},
	})
	f.y = satAdd(f.y, h)
	f.pageHasBody = true
	return true
}

// placeText places lines of a text block and reports whether it is complete.
func (l *layouter) placeText(f *flow, b *Box, width int32) bool {
	l.buildLines(b, width)
	if !b.built {
		return false
	}
	// A single unbreakable token wider than the content box is painted past the
	// edge and clipped by the page. The reader gets one report per box instead
	// of silently losing the tail of a long word.
	for _, line := range b.lines {
		if line.Natural > width {
			l.warnOverflow(b, "a word is wider than the content box and is clipped")
			break
		}
	}
	st := b.Style
	avail := satSub(satSub(f.bottom, b.padBottom()), b.borderBottom())
	total := len(b.lines) - b.cursor.line
	if total <= 0 {
		return true
	}
	fits := countFitting(b.lines, b.cursor.line, f.y, avail)
	if fits == 0 {
		// f.empty means "this box was entered"; only a page that already has
		// painted operations can be advanced. A first line taller than the
		// page must be clipped instead of looping forever between pages.
		if len(f.page.Ops) > 0 {
			f.newPage()
			return false
		}
		l.warnOverflow(b, "content is taller than the page and is clipped")
		fits = 1
	}
	if fits < total {
		remaining := total - fits
		if remaining < st.Widows {
			// Pull lines to the next page so it keeps at least widows lines,
			// but never leave fewer than orphans on this one.
			if better := total - st.Widows; better >= st.Orphans && better < fits {
				fits = better
			}
		}
	}
	for k := 0; k < fits && b.cursor.line < len(b.lines); k++ {
		if l.tick() != nil {
			return false
		}
		line := &b.lines[b.cursor.line]
		last := b.cursor.line == len(b.lines)-1
		lineX, overflow := satAddChecked(satAdd(b.x, b.borderLeft()), b.padLeft())
		if overflow {
			l.warnOverflow(b, "the box's geometry exceeds the page bounds and is clamped")
		}
		l.paintLineAt(f.page, b, line, lineX, f.y, width, last)
		for _, id := range line.Anchors {
			if _, ok := l.anchors[id]; !ok {
				l.anchors[id] = Anchor{Page: f.pageNum, Y: f.y}
			}
		}
		f.y = satAdd(f.y, line.Height)
		f.pageHasBody = true
		b.cursor.line++
	}
	return b.cursor.line >= len(b.lines)
}

func countFitting(lines []Line, from int, y, bottom int32) int {
	n := 0
	for i := from; i < len(lines); i++ {
		// Compare in int64: a cursor pushed past the page by a huge margin
		// would otherwise wrap around and report that the line fits.
		if int64(y)+int64(lines[i].Height) > int64(bottom) {
			break
		}
		y = satAdd(y, lines[i].Height)
		n++
	}
	return n
}

// measure returns the outer height of a box when placed at the given width.
func (l *layouter) measure(b *Box, width int32) int32 {
	st := b.Style
	used := usedWidth(st, width)
	own := clampInt32(int64(st.Padding[0].Resolve(width)) + int64(st.Padding[2].Resolve(width)) +
		int64(visibleWidth(st.Border[0])) + int64(visibleWidth(st.Border[2])))
	inner := clampInt32(int64(used) - int64(st.Padding[1].Resolve(width)) - int64(st.Padding[3].Resolve(width)) -
		int64(visibleWidth(st.Border[1])) - int64(visibleWidth(st.Border[3])))
	if inner < 0 {
		inner = 0
	}
	if st.Height.Kind == css.MP && st.Height.Value > 0 {
		return own + st.Height.Value
	}
	if b.Kind == Table {
		return own + l.measureTable(b, inner)
	}
	if b.Kind == Image {
		if b.Image == nil {
			return own
		}
		_, h := imageSize(b.Style, b.Image, inner, l.ctx)
		return own + h
	}
	if l.isTextBlock(b) {
		l.buildLines(b, inner)
		var h int32
		for _, ln := range b.lines {
			h += ln.Height
		}
		return own + h
	}
	var h int32
	for _, c := range b.Children {
		h += l.measure(c, inner)
		h += c.Style.Margin[0].Resolve(inner) + c.Style.Margin[2].Resolve(inner)
	}
	return own + h
}

// applyFixedHeight grows the box to its declared height and warns when the
// content does not fit on the page.
func (l *layouter) applyFixedHeight(f *flow, b *Box) {
	st := b.Style
	if st.Height.Kind != css.MP || st.Height.Value <= 0 {
		return
	}
	// The declared height belongs to the box as a whole, not to each fragment:
	// a continuation reserves only what the earlier fragments did not consume.
	// Reserving 400 pt on both pages of a split box pushed the next paragraph
	// down by 400 pt twice.
	used := satAdd(b.heightConsumed, satSub(f.y, b.contentTopY))
	if used >= st.Height.Value {
		return
	}
	target := satAdd(f.y, satSub(st.Height.Value, used))
	if target > satSub(satSub(f.bottom, b.padBottom()), b.borderBottom()) {
		l.warnOverflow(b, "content is taller than the page and is clipped")
		return
	}
	if f.y < target {
		f.y = target
		// The reserved height is content of this page even when the box paints
		// nothing else, so a later forced break still has a page to leave.
		f.pageHasBody = true
	}
}

// warnOverflow reports a layout/overflow warning once per box. A box can be
// clipped on several pages and can reach more than one call site; the reader
// needs one report per box, not one per attempt.
func (l *layouter) warnOverflow(b *Box, message string) {
	if b.overflowWarned {
		return
	}
	b.overflowWarned = true
	l.diags.Warnf(diag.LayoutOverflow, diag.Position{}, nodePath(b), "%s", message)
}

func headingLevel(tag string) int {
	switch tag {
	case "h1":
		return 1
	case "h2":
		return 2
	case "h3":
		return 3
	}
	return 0
}

func nodePath(b *Box) string {
	if b.Node != nil {
		return b.Node.Path()
	}
	return ""
}

// satAdd adds two geometry values, saturating at the int32 range. Margins and
// offsets come from CSS lengths up to the documented maximum, so the sum of two
// of them would otherwise wrap around and place content above the page.
func satAdd(a, b int32) int32 { return clampInt32(int64(a) + int64(b)) }

// satSub subtracts two geometry values, saturating at the int32 range.
func satSub(a, b int32) int32 { return clampInt32(int64(a) - int64(b)) }

// satAddChecked adds two geometry values and reports whether the sum did not
// fit in int32, so the caller can diagnose the clamp.
func satAddChecked(a, b int32) (int32, bool) {
	sum := int64(a) + int64(b)
	if sum > math.MaxInt32 || sum < math.MinInt32 {
		return clampInt32(sum), true
	}
	return int32(sum), false
}

// clampInt32 narrows a 64-bit geometry value, saturating instead of wrapping.
func clampInt32(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
