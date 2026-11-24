package layout

import (
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
}

func (f *flow) newPage() {
	l := f.l
	ps := l.pageStyle
	f.pageNum++
	p := &Page{Number: f.pageNum, Size: ps.Size, Margin: ps.Margin}
	l.doc.Pages = append(l.doc.Pages, p)
	f.page = p
	f.left = ps.Margin[3]
	f.top = ps.Margin[0]
	f.width = ps.Size.Width - ps.Margin[1] - ps.Margin[3]
	f.bottom = ps.Size.Height - ps.Margin[2]
	f.y = f.top
	f.pending = 0
	f.empty = true
}

func (f *flow) contentHeight() int32 { return f.bottom - f.top }

func (b *Box) padTop() int32       { return b.Style.Padding[0].Resolve(b.width) }
func (b *Box) padRight() int32     { return b.Style.Padding[1].Resolve(b.width) }
func (b *Box) padBottom() int32    { return b.Style.Padding[2].Resolve(b.width) }
func (b *Box) padLeft() int32      { return b.Style.Padding[3].Resolve(b.width) }
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
	w := b.width - b.padLeft() - b.padRight() - b.borderLeft() - b.borderRight()
	if w < 0 {
		return 0
	}
	return w
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
	for {
		opsBefore := l.ops
		yBefore := f.y
		done := l.placeBlock(f, root, f.width, true)
		if done {
			break
		}
		iterations++
		if l.ops == opsBefore && f.y == yBefore {
			// Nothing was drawn and the flow did not advance: the content cannot
			// be placed, so fail instead of emitting empty pages.
			l.diags.Errorf("layout/no-progress", diag.Position{}, "", "content cannot be placed on a page and pagination cannot advance")
			break
		}
		if iterations > maxIterations {
			l.diags.Errorf("layout/loop", diag.Position{}, "", "pagination did not converge")
			break
		}
		f.newPage()
	}
	l.paintMarginBoxes()
	l.doc.Diags = append(l.doc.Diags, l.diags...)
	l.doc.Diags.Sort()
	return l.doc
}

// placeBlock places as much of b as fits and reports whether it is complete.
func (l *layouter) placeBlock(f *flow, b *Box, width int32, isRoot bool) bool {
	if b.cursor.done {
		return true
	}
	st := b.Style
	if !b.started {
		b.started = true
		b.startPage = f.pageNum
		// CSS width is the content width; the outer width adds padding and
		// borders (content-box model).
		extra := st.Padding[1].Resolve(width) + st.Padding[3].Resolve(width) +
			visibleWidth(st.Border[1]) + visibleWidth(st.Border[3])
		b.width = width
		switch st.Width.Kind {
		case css.MP:
			b.width = st.Width.Value + extra
		case css.Percent:
			b.width = st.Width.Resolve(width) + extra
		}
		if b.width > width || b.width <= 0 {
			b.width = width
		}
		b.x = f.left
	}
	cw := b.contentWidth()

	if !b.entered {
		if st.BreakBefore == css.BreakPage && !f.empty {
			f.newPage()
		}
		if !isRoot {
			mt := st.Margin[0].Resolve(width)
			if f.pending > mt {
				mt = f.pending
			}
			f.pending = 0
			if !f.empty {
				f.y += mt
			}
		}
		if st.BreakInside == css.BreakAvoid && !f.empty {
			if h := l.measure(b, width); f.y+h > f.bottom {
				f.newPage()
			}
		}
		if l.isTextBlock(b) {
			l.buildLines(b, cw)
			if !f.empty {
				startY := f.y + b.borderTop() + b.padTop()
				avail := f.bottom - b.padBottom() - b.borderBottom()
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
		if b.Node != nil {
			if id := b.Node.ID(); id != "" {
				if _, ok := l.anchors[id]; !ok {
					l.anchors[id] = Anchor{Page: f.pageNum, Y: f.y}
				}
			}
			if level := headingLevel(b.Node.Tag); level > 0 {
				l.doc.Outline = append(l.doc.Outline, OutlineEntry{
					Title: b.Node.TextContent(),
					Page:  f.pageNum,
					Y:     f.y,
					Level: level,
				})
			}
		}
		f.y += b.borderTop() + b.padTop()
		b.contentTopY = f.y
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
			l.finishFragment(f, b, true)
			l.afterBlock(f, b)
			return true
		}
		l.finishFragment(f, b, false)
		return false
	case l.isTextBlock(b):
		if l.placeText(f, b, cw) {
			l.finishFragment(f, b, true)
			l.afterBlock(f, b)
			return true
		}
		l.finishFragment(f, b, false)
		return false
	default:
		for b.cursor.child < len(b.Children) {
			c := b.Children[b.cursor.child]
			if !l.placeChild(f, c, cw) {
				l.finishFragment(f, b, false)
				return false
			}
			b.cursor.child++
		}
		l.finishFragment(f, b, true)
		l.afterBlock(f, b)
		return true
	}
}

func (l *layouter) placeChild(f *flow, c *Box, width int32) bool {
	if !f.empty {
		if c.Style.BreakBefore == css.BreakPage {
			f.newPage()
		} else if c.Style.BreakInside == css.BreakAvoid {
			h := l.measure(c, width)
			if f.y+h > f.bottom && h <= f.contentHeight() {
				f.newPage()
			}
		}
	}
	return l.placeBlock(f, c, width, false)
}

func (l *layouter) afterBlock(f *flow, b *Box) {
	l.applyFixedHeight(f, b)
	b.cursor.done = true
	f.pending = b.Style.Margin[2].Resolve(b.width)
	if b.Style.BreakAfter == css.BreakPage {
		f.newPage()
	}
}

// placeText places lines of a text block and reports whether it is complete.
func (l *layouter) placeText(f *flow, b *Box, width int32) bool {
	if !b.built {
		l.buildLines(b, width)
	}
	st := b.Style
	avail := f.bottom - b.padBottom() - b.borderBottom()
	total := len(b.lines) - b.cursor.line
	if total <= 0 {
		return true
	}
	fits := countFitting(b.lines, b.cursor.line, f.y, avail)
	if fits == 0 {
		if !f.empty {
			f.newPage()
			return false
		}
		l.diags.Warnf("layout/overflow", diag.Position{}, nodePath(b), "content is taller than the page and is clipped")
		fits = 1
	}
	if fits < total {
		remaining := total - fits
		if remaining < st.Widows && fits > st.Widows {
			fits = total - st.Widows
		}
	}
	for k := 0; k < fits && b.cursor.line < len(b.lines); k++ {
		line := &b.lines[b.cursor.line]
		last := b.cursor.line == len(b.lines)-1
		l.paintLineAt(f.page, b, line, b.x+b.borderLeft()+b.padLeft(), f.y, width, last)
		f.y += line.Height
		b.cursor.line++
	}
	if b.cursor.line < len(b.lines) {
		return false
	}
	return true
}

func countFitting(lines []Line, from int, y, bottom int32) int {
	n := 0
	for i := from; i < len(lines); i++ {
		if y+lines[i].Height > bottom {
			break
		}
		y += lines[i].Height
		n++
	}
	return n
}

func (l *layouter) checkPageBreak(f *flow, needed int32) bool {
	if f.y+needed <= f.bottom {
		return false
	}
	if f.empty {
		l.diags.Warnf("layout/overflow", diag.Position{}, "", "content is taller than the page and is clipped")
		return false
	}
	f.newPage()
	return true
}

// measure returns the outer height of a box when placed at the given width.
func (l *layouter) measure(b *Box, width int32) int32 {
	st := b.Style
	own := st.Padding[0].Resolve(width) + st.Padding[2].Resolve(width) +
		visibleWidth(st.Border[0]) + visibleWidth(st.Border[2])
	inner := width - st.Padding[1].Resolve(width) - st.Padding[3].Resolve(width) -
		visibleWidth(st.Border[1]) - visibleWidth(st.Border[3])
	if inner < 0 {
		inner = 0
	}
	if st.Height.Kind == css.MP && st.Height.Value > 0 {
		return own + st.Height.Value
	}
	if b.Kind == Table {
		return own + l.measureTable(b, inner)
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
	target := b.contentTopY + st.Height.Value
	if target > f.bottom-b.padBottom()-b.borderBottom() {
		l.diags.Warnf("layout/overflow", diag.Position{}, nodePath(b), "content is taller than the page and is clipped")
		return
	}
	if f.y < target {
		f.y = target
	}
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
