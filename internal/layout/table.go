package layout

import (
	"github.com/vyachin/go-pdf/internal/css"
)

// normSpan clamps a stored span, so layout never depends on the parser having
// validated it.
func normSpan(v int) int {
	if v < 1 {
		return 1
	}
	if v > css.MaxTableSpan {
		return css.MaxTableSpan
	}
	return v
}

// computeColumns implements the fixed table layout: widths come from the first
// row, the rest of the available width is split evenly between the remaining
// columns. Cell columns come from the occupancy grid.
func (l *layouter) computeColumns(t *Box, available int32) []int32 {
	n := 0
	for _, row := range t.Rows {
		for _, cell := range row.Cells {
			span := normSpan(cell.ColSpan)
			if end := cell.col + span; end > n {
				n = end
			}
		}
	}
	if n == 0 {
		return nil
	}
	widths := make([]int32, n)
	if len(t.Rows) > 0 {
		for _, cell := range t.Rows[0].Cells {
			span := normSpan(cell.ColSpan)
			var w int32
			switch cell.Style.Width.Kind {
			case css.MP:
				w = cell.Style.Width.Value
			case css.Percent:
				w = cell.Style.Width.Resolve(available)
			}
			if w > 0 {
				per := w / int32(span)
				for i := 0; i < span && cell.col+i < n; i++ {
					widths[cell.col+i] = per
				}
			}
		}
	}
	var used int32
	zero := 0
	for _, w := range widths {
		used += w
		if w == 0 {
			zero++
		}
	}
	remaining := available - used
	if remaining < 0 {
		remaining = 0
	}
	if zero > 0 {
		per := remaining / int32(zero)
		for i := range widths {
			if widths[i] == 0 {
				widths[i] = per
			}
		}
	}
	return widths
}

// buildGrid assigns every cell the column it occupies, skipping columns taken
// by cells that continue from earlier rows. It reports false when the work
// budget or a cancellation stopped it, so the caller does not cache a partial
// grid.
func (l *layouter) buildGrid(t *Box) bool {
	occupied := make([]map[int]bool, len(t.Rows))
	for i := range occupied {
		occupied[i] = map[int]bool{}
	}
	for r, row := range t.Rows {
		col := 0
		for _, cell := range row.Cells {
			// Placing a cell is work: a table with many rowspans fills many grid
			// slots, and the shared budget must see it.
			if l.tick() != nil {
				return false
			}
			for occupied[r][col] {
				col++
			}
			span := normSpan(cell.ColSpan)
			rowspan := normSpan(cell.RowSpan)
			cell.col = col
			for dr := 0; dr < rowspan && r+dr < len(t.Rows); dr++ {
				for c := col; c < col+span; c++ {
					occupied[r+dr][c] = true
				}
			}
			col += span
		}
	}
	return true
}

// tableLayout builds the occupancy grid and the column widths of a table. The
// grid is built and charged once per table: measurement and placement must see
// the same cells, or a break-inside: avoid decision is taken from a table
// height the placement does not match.
func (l *layouter) tableLayout(t *Box, width int32) bool {
	if t.columns != nil && t.gridWidth == width {
		return true
	}
	if !t.gridBuilt {
		if !l.buildGrid(t) {
			return false
		}
		t.gridBuilt = true
	}
	t.columns = l.computeColumns(t, width)
	t.gridWidth = width
	return true
}

// measureTable returns the content height of a table.
func (l *layouter) measureTable(t *Box, width int32) int32 {
	if !l.tableLayout(t, width) {
		return 0
	}
	var h int32
	for _, rh := range l.rowHeights(t) {
		h = satAdd(h, rh)
	}
	return h
}

// rowHeights computes the height of every row in two passes. The base pass
// measures each cell once and gives its owner row the cell height divided by
// the span, as before, plus the declared row height as a minimum. The deficit
// pass then grows the last covered row of a spanning cell until the rows it
// covers sum to the cell's outer height, so the cell's content cannot paint
// over the block that follows the table. A span that already fits is untouched.
func (l *layouter) rowHeights(t *Box) []int32 {
	heights := make([]int32, len(t.Rows))
	// measured[i][j] is the outer height of row i's cell j, measured once and
	// reused by the deficit pass.
	measured := make([][]int32, len(t.Rows))
	for i, row := range t.Rows {
		measured[i] = make([]int32, len(row.Cells))
		var h int32
		for j, cell := range row.Cells {
			span := normSpan(cell.ColSpan)
			var w int32
			for k := 0; k < span && cell.col+k < len(t.columns); k++ {
				w = satAdd(w, t.columns[cell.col+k])
			}
			cell.width = w
			cell.x = t.x
			cellH := l.measure(cell, w)
			measured[i][j] = cellH
			rowspan := cellRowspan(cell, i, len(t.Rows))
			if rowspan > 1 {
				cellH = cellH / int32(rowspan)
			}
			if cellH > h {
				h = cellH
			}
		}
		if row.Style != nil && row.Style.Height.Kind == css.MP && row.Style.Height.Value > h {
			// height on a table row is a minimum for the row box.
			h = row.Style.Height.Value
		}
		heights[i] = h
	}
	for i, row := range t.Rows {
		for j, cell := range row.Cells {
			rowspan := cellRowspan(cell, i, len(t.Rows))
			if rowspan <= 1 {
				continue
			}
			var covered int32
			for k := i; k < i+rowspan; k++ {
				covered = satAdd(covered, heights[k])
			}
			if need := measured[i][j] - covered; need > 0 {
				heights[i+rowspan-1] = satAdd(heights[i+rowspan-1], need)
			}
		}
	}
	return heights
}

// cellRowspan clamps a cell's rowspan to the rows that actually follow, so a
// cell spanning past the table does not halve the height of the only row it
// occupies.
func cellRowspan(cell *Box, index, rows int) int {
	rowspan := normSpan(cell.RowSpan)
	if remaining := rows - index; rowspan > remaining {
		rowspan = remaining
	}
	return rowspan
}

// placeTable places rows until the page is full. It never creates a page
// itself: a forced break, like a row that does not fit, returns control to the
// page loop, so every enclosing fragment is finished on the old page and the
// new page is painted from its own origin.
func (l *layouter) placeTable(f *flow, t *Box, width int32) bool {
	if !t.built {
		if !l.tableLayout(t, width) {
			return false
		}
		t.rowHeights = l.rowHeights(t)
		t.built = true
		for _, row := range t.Rows {
			if !row.Header {
				break
			}
			t.headerCount++
		}
	}
	// Repeat the header on every page that continues the table. Only then may
	// the row loop skip header rows; on the first page they are placed normally.
	repeated := l.paintRepeatedHeaders(f, t)
	t.headerPage = f.pageNum

	for t.cursor.row < len(t.Rows) {
		if l.tick() != nil {
			return false
		}
		row := t.Rows[t.cursor.row]
		if repeated && row.Header && t.cursor.row < t.headerCount {
			// Already painted as the repeated header above.
			t.cursor.row++
			continue
		}
		// A row break shares the pending decision with blocks. When the page
		// already carries body content the transition is owed; returning hands
		// it to the page loop. When it does not, the boundary is already fresh.
		if row.Style != nil && row.Style.BreakBefore == css.BreakPage {
			f.breakPending = true
		}
		if f.breakPending {
			if f.pageHasBody {
				return false
			}
			f.breakPending = false
		}
		h := t.rowHeights[t.cursor.row]
		if int64(f.y)+int64(h) > int64(f.bottom) {
			// A new page only helps when the row fits there, and a continuation
			// page first repeats the header rows. When the row is larger than
			// that space too, breaking pages can never place it: clip it once
			// instead of looping until the page limit.
			space := f.contentHeight()
			if t.cursor.row != 0 && t.headerCount > 0 {
				for i := 0; i < t.headerCount; i++ {
					space -= t.rowHeights[i]
				}
			}
			if len(f.page.Ops) == 0 || h > space {
				l.warnOverflow(t, "a table row is taller than the page and is clipped")
			} else {
				return false
			}
		}
		l.paintRow(f, t, t.cursor.row)
		f.y = satAdd(f.y, h)
		// A body row is page content; the repeated headers above are not.
		f.pageHasBody = true
		t.cursor.row++
		if row.Style != nil && row.Style.BreakAfter == css.BreakPage {
			f.breakPending = true
			if t.cursor.row < len(t.Rows) {
				// The page loop finishes this fragment; the remaining rows
				// continue after it and repeat the header.
				return false
			}
			// A trailing row break is taken only by the content that follows.
		}
	}
	return true
}

// paintRepeatedHeaders paints the header rows when the table continues on a
// new page and reports whether it did.
func (l *layouter) paintRepeatedHeaders(f *flow, t *Box) bool {
	if t.headerPage == f.pageNum || t.cursor.row == 0 || t.headerCount == 0 {
		return false
	}
	for i := 0; i < t.headerCount; i++ {
		f.y = satAdd(f.y, l.paintRow(f, t, i))
		f.empty = false
	}
	return true
}

// paintRow draws a row and its cells and returns its height.
func (l *layouter) paintRow(f *flow, t *Box, index int) int32 {
	return l.paintRowAt(f.page, t, index, f.y)
}

// paintRowAt draws one row at an explicit page position, which is what a table
// nested inside a cell needs.
func (l *layouter) paintRowAt(page *Page, t *Box, index int, y int32) int32 {
	row := t.Rows[index]
	h := t.rowHeights[index]
	// The rows sit inside the table's padding box; the columns were already
	// computed for the content width, so the origin includes the padding.
	startX := satAdd(satAdd(t.x, t.borderLeft()), t.padLeft())
	// The row background is painted before the cells so that opaque cell
	// backgrounds still cover it.
	var rowWidth int32
	for _, w := range t.columns {
		rowWidth = satAdd(rowWidth, w)
	}
	if row.Style != nil && row.Style.Background.Set && rowWidth > 0 && h > 0 {
		l.ops++
		page.Ops = append(page.Ops, Op{
			Kind:  FillOpKind,
			Rect:  Rect{X: startX, Y: y, W: rowWidth, H: h},
			Color: row.Style.Background,
		})
	}
	if row.Background != nil && rowWidth > 0 && h > 0 {
		if op, ok := backgroundOp(row.Background, Rect{X: startX, Y: y, W: rowWidth, H: h}); ok {
			l.ops++
			page.Ops = append(page.Ops, op)
		}
	}
	for _, cell := range row.Cells {
		span := normSpan(cell.ColSpan)
		x := startX
		for i := 0; i < cell.col && i < len(t.columns); i++ {
			x += t.columns[i]
		}
		var w int32
		for i := 0; i < span && cell.col+i < len(t.columns); i++ {
			w += t.columns[cell.col+i]
		}
		cellH := h
		if rowspan := normSpan(cell.RowSpan); rowspan > 1 {
			cellH = 0
			for r := index; r < index+rowspan && r < len(t.rowHeights); r++ {
				cellH += t.rowHeights[r]
			}
		}
		lastRow := index == len(t.Rows)-1
		lastCol := cell.col+span >= len(t.columns)
		l.paintCell(page, cell, x, y, w, cellH, index == 0, cell.col == 0, lastRow, lastCol,
			[4]int32{t.borderTop(), t.borderRight(), t.borderBottom(), t.borderLeft()})
	}
	return h
}

// paintCell draws one cell: background, borders and content. With collapsed
// borders each cell draws only the edges it owns, so a shared border is painted
// exactly once.
func (l *layouter) paintCell(page *Page, cell *Box, x, y, w, h int32, firstRow, firstCol, lastRow, lastCol bool, frame [4]int32) {
	cell.x = x
	cell.width = w
	if cell.Style.Background.Set {
		l.ops++
		page.Ops = append(page.Ops, Op{Kind: FillOpKind, Rect: Rect{X: x, Y: y, W: w, H: h}, Color: cell.Style.Background})
	}
	if cell.Background != nil && w > 0 && h > 0 {
		if op, ok := backgroundOp(cell.Background, Rect{X: x, Y: y, W: w, H: h}); ok {
			l.ops++
			page.Ops = append(page.Ops, op)
		}
	}
	paintSide := func(side int, rect Rect) {
		border := cell.Style.Border[side]
		if !border.IsVisible() || rect.W <= 0 || rect.H <= 0 {
			return
		}
		color := border.Color
		if !color.Set {
			color = css.Color{Set: true}
		}
		l.ops++
		page.Ops = append(page.Ops, Op{Kind: FillOpKind, Rect: rect, Color: color})
	}
	bl, br := visibleWidth(cell.Style.Border[3]), visibleWidth(cell.Style.Border[1])
	bt, bb := visibleWidth(cell.Style.Border[0]), visibleWidth(cell.Style.Border[2])
	collapse := cell.Style.BorderCollapse == css.BorderCollapseCollapse
	leftInset, rightInset, topInset, bottomInset := bl, br, bt, bb
	if collapse {
		// A shared grid line is drawn once by the cell on the right or below;
		// the table's own border is the frame where it exists. A cell that
		// inherits the frame does not reserve its own edge, so the border is
		// neither doubled nor separated from the content by an empty stripe.
		if firstCol && frame[3] > 0 {
			leftInset = 0
		}
		if firstRow && frame[0] > 0 {
			topInset = 0
		}
		if !lastCol || frame[1] > 0 {
			rightInset = 0
		}
		if !lastRow || frame[2] > 0 {
			bottomInset = 0
		}
		paintSide(3, Rect{X: x, Y: y, W: leftInset, H: h})
		paintSide(0, Rect{X: x, Y: y, W: w, H: topInset})
		if lastCol && rightInset > 0 {
			paintSide(1, Rect{X: x + w - rightInset, Y: y, W: rightInset, H: h})
		}
		if lastRow && bottomInset > 0 {
			paintSide(2, Rect{X: x, Y: y + h - bottomInset, W: w, H: bottomInset})
		}
	} else {
		paintSide(3, Rect{X: x, Y: y, W: bl, H: h})
		paintSide(1, Rect{X: x + w - br, Y: y, W: br, H: h})
		paintSide(0, Rect{X: x, Y: y, W: w, H: bt})
		paintSide(2, Rect{X: x, Y: y + h - bb, W: w, H: bb})
	}

	inner := clampInt32(int64(w) - int64(cell.padLeft()) - int64(cell.padRight()) - int64(leftInset) - int64(rightInset))
	if inner < 0 {
		inner = 0
	}
	contentX := satAdd(satAdd(x, leftInset), cell.padLeft())
	contentY := satAdd(satAdd(y, topInset), cell.padTop())
	contentH := int32(0)
	for _, c := range cell.Children {
		contentH = satAdd(contentH, l.measure(c, inner))
	}
	availH := clampInt32(int64(h) - int64(topInset) - int64(bottomInset) - int64(cell.padTop()) - int64(cell.padBottom()))
	switch cell.Style.VerticalAlign {
	case css.VAMiddle:
		contentY = satAdd(contentY, satSub(availH, contentH)/2)
	case css.VABottom:
		contentY = satAdd(contentY, satSub(availH, contentH))
	}
	if contentY < y+bt+cell.padTop() {
		contentY = y + bt + cell.padTop()
	}
	l.paintStaticBlocks(page, cell.Children, contentX, contentY, inner)
}

// paintStaticTable draws a table nested inside a cell: columns are computed for
// the available width and rows are painted at explicit positions.
func (l *layouter) paintStaticTable(page *Page, t *Box, x, y, width int32) int32 {
	inner := clampInt32(int64(width) - int64(t.padLeft()) - int64(t.padRight()) - int64(t.borderLeft()) - int64(t.borderRight()))
	if inner < 0 {
		inner = 0
	}
	// paintCell measured the nested table at this width, so the cached grid and
	// columns are reused instead of charging the build twice. A different
	// width recomputes the columns from the same grid.
	if !l.tableLayout(t, inner) {
		return 0
	}
	t.rowHeights = l.rowHeights(t)
	var rows int32
	for _, rh := range t.rowHeights {
		rows = satAdd(rows, rh)
	}
	t.built = true
	t.x = x
	t.width = width
	height := satAdd(satAdd(satAdd(t.borderTop(), t.padTop()), rows), satAdd(t.padBottom(), t.borderBottom()))
	if ops := l.decorationOps(t, y, y+height, true, true); len(ops) > 0 {
		l.ops += len(ops)
		page.Ops = append(page.Ops, ops...)
	}
	rowY := y + t.borderTop() + t.padTop()
	for i := range t.Rows {
		l.paintRowAt(page, t, i, rowY)
		rowY += t.rowHeights[i]
	}
	return height
}

// paintStaticBlocks lays out blocks without pagination, used inside table
// cells. It returns the total height.
func (l *layouter) paintStaticBlocks(page *Page, blocks []*Box, x, y, width int32) int32 {
	var h int32
	for _, b := range blocks {
		// Cell measurement already includes block margins; painting has to
		// advance by the same amounts or the drawn content drifts upwards.
		h += b.Style.Margin[0].Resolve(width)
		h += l.paintStaticBlock(page, b, x, y+h, width)
		h += b.Style.Margin[2].Resolve(width)
	}
	return h
}

func (l *layouter) paintStaticBlock(page *Page, b *Box, x, y, width int32) int32 {
	if !b.started {
		b.started = true
		b.usedGeometry(x, width)
	}
	l.registerAnchor(b, page.Number, y)
	if b.Kind == Table {
		return l.paintStaticTable(page, b, b.x, y, b.width)
	}
	opStart := len(page.Ops)
	if b.Kind == ListItem {
		// The marker belongs to the first line. It is painted before any
		// content, exactly like the paginated path does on entry.
		b.listMarkerY = y + b.borderTop() + b.padTop()
		l.paintListMarkerAt(page, b, x)
	}
	if b.Kind == Image {
		if b.Image == nil {
			return 0
		}
		inner := b.contentWidth()
		w, h := imageSize(b.Style, b.Image, inner, l.ctx)
		l.ops++
		page.Ops = append(page.Ops, Op{
			Kind: ImageOpKind,
			Rect: Rect{
				X: satAdd(satAdd(b.x, b.borderLeft()), b.padLeft()),
				Y: satAdd(satAdd(y, b.borderTop()), b.padTop()),
				W: w,
				H: h,
			},
			Image: &ImageOp{Image: b.Image},
		})
		height := l.staticHeight(b, h)
		l.paintStaticDecorations(page, b, y, height, opStart)
		return height
	}
	inner := b.contentWidth()
	var contentH int32
	if l.isTextBlock(b) {
		if !b.built {
			l.buildLines(b, inner)
		}
		lineY := satAdd(satAdd(y, b.borderTop()), b.padTop())
		for i := range b.lines {
			l.paintLineAt(page, b, &b.lines[i], satAdd(satAdd(b.x, b.borderLeft()), b.padLeft()), lineY, inner, i == len(b.lines)-1)
			for _, id := range b.lines[i].Anchors {
				if _, ok := l.anchors[id]; !ok {
					l.anchors[id] = Anchor{Page: page.Number, Y: lineY}
				}
			}
			contentH = satAdd(contentH, b.lines[i].Height)
			lineY = satAdd(lineY, b.lines[i].Height)
		}
	} else {
		contentH = l.paintStaticBlocks(page, b.Children, satAdd(satAdd(b.x, b.borderLeft()), b.padLeft()),
			satAdd(satAdd(y, b.borderTop()), b.padTop()), inner)
	}
	height := l.staticHeight(b, contentH)
	l.paintStaticDecorations(page, b, y, height, opStart)
	return height
}

// staticHeight returns the outer height of a statically painted block: its
// content height, raised to a declared CSS height when that is larger.
func (l *layouter) staticHeight(b *Box, contentH int32) int32 {
	if b.Style.Height.Kind == css.MP && b.Style.Height.Value > contentH {
		contentH = b.Style.Height.Value
	}
	return satAdd(satAdd(satAdd(b.borderTop(), b.padTop()), contentH), satAdd(b.padBottom(), b.borderBottom()))
}

// paintStaticDecorations inserts the background and border fills behind the
// operations painted for the box, so a static block looks like the same block
// in the paginated flow.
func (l *layouter) paintStaticDecorations(page *Page, b *Box, y, height int32, opStart int) {
	ops := l.decorationOps(b, y, y+height, true, true)
	if len(ops) == 0 {
		return
	}
	l.ops += len(ops)
	page.Ops = l.insertOps(page.Ops, opStart, ops)
}
