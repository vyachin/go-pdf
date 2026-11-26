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
	// The declared widths can exceed the int32 range together, so the running
	// sum that decides how much space is left is kept in 64 bits. A wrapped sum
	// divided the remainder against a negative total and split a two-column
	// table into columns wider than the page.
	var used int64
	zero := 0
	for _, w := range widths {
		used += int64(w)
		if w == 0 {
			zero++
		}
	}
	remaining := int64(available) - used
	if remaining < 0 {
		remaining = 0
	}
	if zero > 0 {
		per := clampInt32(remaining / int64(zero))
		for i := range widths {
			if widths[i] == 0 {
				widths[i] = per
			}
		}
	}
	// The declared widths may not fit the content box. Scale against the true
	// 64-bit total, then clamp each produced width to the space that is left:
	// the loop invariant is that the columns never end to the right of the
	// content box. The remainder rounds into the last column, so the rounding
	// is deterministic and no fraction of a millipoint is lost.
	var total int64
	for _, w := range widths {
		total += int64(w)
	}
	scaled := total > int64(available)
	if scaled {
		var running int64
		last := len(widths) - 1
		for i, w := range widths {
			next := int64(w) * int64(available) / total
			if i == last {
				next = int64(available) - running
			} else if rest := int64(available) - running; next > rest {
				next = rest
			}
			if next < 0 {
				next = 0
			}
			widths[i] = int32(next)
			running += next
		}
		// The warning describes the geometry that was produced, and the
		// per-box latch keeps it to one report for the table.
		l.warnOverflow(t, "the table's declared columns are wider than the content box and are scaled to fit")
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
	// A forced break that was already pending when the table was entered
	// belongs to the box flow, not to a row: an empty table does not consume
	// it, and the row loop must not take it as a row break. The flag stays
	// pending for the next box that paints.
	inheritedBreak := f.breakPending
	// Every spanning cell whose fragment crossed the previous page break
	// continues here, in grid order, before the rows that carry the rest of the
	// span are painted.
	l.paintSpanContinuations(f, t)

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
		rowBreak := false
		if row.Style != nil && row.Style.BreakBefore == css.BreakPage {
			f.breakPending = true
			rowBreak = true
		}
		if f.breakPending && (rowBreak || !inheritedBreak) {
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
		// A body row that can paint something is page content; the repeated
		// headers above are not, and neither is a row that only reserves the
		// padding height of an empty cell.
		if rowHasContent(row) {
			f.pageHasBody = true
		}
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
	// The table is complete; a continuation that never got a fragment reports
	// its loss instead of disappearing silently.
	l.finishSpanRecords(t)
	return true
}

// paintSpanContinuations paints the next fragment of every spanning cell that
// crossed a page break, in the order paintRow recorded them. A record is kept
// until the rows its cell covers have all been placed.
func (l *layouter) paintSpanContinuations(f *flow, t *Box) {
	if len(t.spanRecords) == 0 {
		return
	}
	kept := t.spanRecords[:0]
	for i := range t.spanRecords {
		rec := &t.spanRecords[i]
		if t.cursor.row >= rec.endRow {
			// Every row the cell covers is placed, so its fragments are done.
			l.reportSpanLoss(rec)
			continue
		}
		// The rows that are still covered bound this fragment: it must not
		// extend over the rows below the span.
		var remain int32
		for r := t.cursor.row; r < rec.endRow && r < len(t.rowHeights); r++ {
			remain = satAdd(remain, t.rowHeights[r])
		}
		rec.remain = remain
		space := satSub(f.bottom, f.y)
		h := remain
		if h > space {
			h = space
		}
		if h > 0 {
			l.paintSpanFragment(f, rec, h)
		}
		if h == remain {
			// The remaining rows fit on this page: this is the cell's last
			// fragment, so no line may be left unpainted.
			l.reportSpanLoss(rec)
		}
		kept = append(kept, *rec)
	}
	t.spanRecords = kept
}

// paintSpanFragment paints one continuation fragment of a spanning cell: its
// background, all four border sides and the lines that were not painted before.
// The fragment never extends below the page content bottom.
func (l *layouter) paintSpanFragment(f *flow, rec *spanContinuation, h int32) {
	cell := rec.cell
	top, bottom := f.y, satAdd(f.y, h)
	// Decoration is per fragment: every fragment is a closed box at its own
	// edges, so a continued cell never looks like an unbordered column of text.
	cell.x, cell.width = rec.x, rec.w
	ops := l.decorationOps(cell, top, bottom, true, true)
	if len(ops) > 0 {
		l.ops += len(ops)
		f.page.Ops = append(f.page.Ops, ops...)
		f.pageHasBody = true
	}
	if rec.line < 0 {
		// The content is block-level and has no line cursor: it cannot be
		// fragmented here. Report the loss once for the cell and keep the rest
		// of the document rendering.
		if cellHasContent(cell) {
			l.warnOverflow(cell, "the remaining content of the spanning cell cannot be continued and is clipped")
		}
		return
	}
	block, ok := l.spanLineBlock(cell)
	if !ok {
		return
	}
	l.buildLines(block, rec.lineWidth)
	next := l.paintBlockLines(f.page, block, rec.line, rec.lineX, satAdd(top, rec.lineTop), rec.lineWidth, bottom)
	if next > rec.line {
		f.pageHasBody = true
		rec.line = next
	}
}

// finishSpanRecords reports any content a completed table could not continue
// and drops the records.
func (l *layouter) finishSpanRecords(t *Box) {
	for i := range t.spanRecords {
		l.reportSpanLoss(&t.spanRecords[i])
	}
	t.spanRecords = nil
}

// reportSpanLoss reports the content of a spanning cell that no fragment can
// paint. warnOverflow's per-box latch makes it one diagnostic per cell.
func (l *layouter) reportSpanLoss(rec *spanContinuation) {
	if rec.line < 0 {
		if cellHasContent(rec.cell) {
			l.warnOverflow(rec.cell, "the remaining content of the spanning cell cannot be continued and is clipped")
		}
		return
	}
	block, ok := l.spanLineBlock(rec.cell)
	if !ok {
		return
	}
	l.buildLines(block, rec.lineWidth)
	if rec.line < len(block.lines) {
		l.warnOverflow(rec.cell, "the remaining content of the spanning cell does not fit on a page and is clipped")
	}
}

// spanLineBlock returns the single text block whose lines a spanning cell
// continues across pages. A cell whose content is block-level (a nested table, a
// list or several blocks) has no line cursor in this change, so the caller
// reports the loss instead of dropping it silently.
func (l *layouter) spanLineBlock(cell *Box) (*Box, bool) {
	if len(cell.Children) != 1 {
		return nil, false
	}
	block := cell.Children[0]
	if block.Kind != Block || !l.isTextBlock(block) {
		return nil, false
	}
	return block, true
}

// cellHasContent reports whether a cell has content that a fragment would have
// to paint.
func cellHasContent(cell *Box) bool {
	for _, c := range cell.Children {
		if boxHasContent(c) {
			return true
		}
	}
	return false
}

// paintRepeatedHeaders paints the header rows when the table continues on a
// new page and reports whether it did.
func (l *layouter) paintRepeatedHeaders(f *flow, t *Box) bool {
	if t.headerPage == f.pageNum || t.cursor.row == 0 || t.headerCount == 0 {
		return false
	}
	for i := 0; i < t.headerCount; i++ {
		// A repeated header is painted again on every page, so it never needs a
		// spanning-cell continuation of its own.
		f.y = satAdd(f.y, l.paintRowAt(f.page, t, i, f.y, f.bottom, false))
		f.empty = false
	}
	return true
}

// paintRow draws a row and its cells and returns its height. The flow's bottom
// is the limit: nothing in the row is painted below it.
func (l *layouter) paintRow(f *flow, t *Box, index int) int32 {
	return l.paintRowAt(f.page, t, index, f.y, f.bottom, true)
}

// paintRowAt draws one row at an explicit page position, which is what a table
// nested inside a cell needs. record enables the continuation state of a
// spanning cell that is clipped: a nested table is painted statically, so it
// has no page to continue the cell on.
func (l *layouter) paintRowAt(page *Page, t *Box, index int, y, limit int32, record bool) int32 {
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
			x = satAdd(x, t.columns[i])
		}
		var w int32
		for i := 0; i < span && cell.col+i < len(t.columns); i++ {
			w = satAdd(w, t.columns[cell.col+i])
		}
		cellH := h
		rowspan := normSpan(cell.RowSpan)
		if rowspan > 1 {
			cellH = 0
			for r := index; r < index+rowspan && r < len(t.rowHeights); r++ {
				cellH = satAdd(cellH, t.rowHeights[r])
			}
		}
		// A spanning cell that crosses the page break is clipped to the page:
		// its background, border and content must not be painted outside the
		// sheet. Its continuation is recorded so every fragment of the cell is
		// painted on the pages the span covers.
		pageBottom := limit
		visible := cellH
		clipped := false
		if satAdd(y, visible) > pageBottom {
			visible = satSub(pageBottom, y)
			if visible < 0 {
				visible = 0
			}
			clipped = record && rowspan > 1
		}
		lastRow := index == len(t.Rows)-1
		lastCol := cell.col+span >= len(t.columns)
		cursor := l.paintCell(page, cell, x, y, w, visible, index == 0, cell.col == 0, lastRow, lastCol,
			[4]int32{t.borderTop(), t.borderRight(), t.borderBottom(), t.borderLeft()}, limit, clipped)
		if clipped {
			endRow := index + rowspan
			if endRow > len(t.Rows) {
				endRow = len(t.Rows)
			}
			t.spanRecords = append(t.spanRecords, spanContinuation{
				cell:      cell,
				x:         x,
				w:         w,
				remain:    satSub(cellH, visible),
				endRow:    endRow,
				line:      cursor.line,
				lineX:     cursor.x,
				lineTop:   cursor.top,
				lineWidth: cursor.width,
			})
		}
	}
	return h
}

// cellCursor is the text continuation state of a clipped spanning cell: the
// index of the first line that was not painted and the geometry the following
// fragments paint the rest at. line is -1 when the cell has no line cursor.
type cellCursor struct {
	line   int
	x, top int32
	width  int32
}

// paintCell draws one cell: background, borders and content, and returns the
// line cursor of a spanning cell that was clipped. With collapsed borders each
// cell draws only the edges it owns, so a shared border is painted exactly once.
// A clipped spanning cell paints all four of its sides instead: the fragment
// ends at the page boundary, so it is a closed box at its own edges.
func (l *layouter) paintCell(page *Page, cell *Box, x, y, w, h int32, firstRow, firstCol, lastRow, lastCol bool, frame [4]int32, limit int32, clipped bool) cellCursor {
	cursor := cellCursor{line: -1}
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
		// A clipped fragment is not a true cell edge, so it keeps all four.
		if !clipped {
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
		}
		paintSide(3, Rect{X: x, Y: y, W: leftInset, H: h})
		paintSide(0, Rect{X: x, Y: y, W: w, H: topInset})
		if (clipped || lastCol) && rightInset > 0 {
			paintSide(1, Rect{X: x + w - rightInset, Y: y, W: rightInset, H: h})
		}
		if (clipped || lastRow) && bottomInset > 0 {
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
	if clipped {
		// A fragment that ends at the page boundary starts its content at the
		// fragment's own top; its remaining lines are continued from there.
		if block, ok := l.spanLineBlock(cell); ok {
			cursor := l.paintSpanCellText(page, block, contentX, contentY, inner, y, limit)
			l.checkCellOverflow(cell)
			return cursor
		}
		// The content is block-level and has no line cursor. Paint what fits
		// quietly: the continuation reports the loss once for the cell instead
		// of one diagnostic per painted line.
		l.suppressOverflow++
		l.paintStaticBlocks(page, cell.Children, contentX, contentY, inner, limit)
		l.suppressOverflow--
		l.checkCellOverflow(cell)
		return cursor
	}
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
	l.paintStaticBlocks(page, cell.Children, contentX, contentY, inner, limit)
	l.checkCellOverflow(cell)
	return cursor
}

// checkCellOverflow reports a word wider than the width its cell text was
// painted at, once for the cell. The cell content is painted by the static
// path, which never calls placeText, so the shared check runs here after the
// painter has built the lines: the comparison uses exactly the geometry that
// was drawn.
func (l *layouter) checkCellOverflow(cell *Box) {
	for _, c := range cell.Children {
		if l.checkPaintedOverflow(cell, c) {
			return
		}
	}
}

// checkPaintedOverflow walks the blocks a cell painter started and checks the
// lines they built. A block below the page was not painted, so it is skipped;
// a nested table is left to its own cells.
func (l *layouter) checkPaintedOverflow(cell, b *Box) bool {
	if b.Kind == Table || b.Kind == Image || !b.started {
		return false
	}
	if l.isTextBlock(b) {
		inner := b.contentWidth()
		for i := range b.lines {
			if l.checkLineOverflow(cell, b.lines[i], inner) {
				return true
			}
		}
		return false
	}
	for _, c := range b.Children {
		if l.checkPaintedOverflow(cell, c) {
			return true
		}
	}
	return false
}

// paintSpanCellText paints the lines of a clipped spanning cell's text block
// that fit before limit and returns the cursor of the rest.
func (l *layouter) paintSpanCellText(page *Page, block *Box, contentX, contentY, inner, cellY, limit int32) cellCursor {
	block.started = true
	block.usedGeometry(contentX, inner)
	blockY := satAdd(contentY, block.Style.Margin[0].Resolve(inner))
	l.registerAnchor(block, page.Number, blockY)
	lineX := satAdd(satAdd(block.x, block.borderLeft()), block.padLeft())
	lineY := satAdd(satAdd(blockY, block.borderTop()), block.padTop())
	lineWidth := block.contentWidth()
	l.buildLines(block, lineWidth)
	line := l.paintBlockLines(page, block, 0, lineX, lineY, lineWidth, limit)
	return cellCursor{
		line:  line,
		x:     lineX,
		top:   satSub(lineY, cellY),
		width: lineWidth,
	}
}

// paintBlockLines paints the lines of block from index from and stops before a
// line that would start at or below limit. It returns the index of the first
// line that was not painted. The last line of the fragment is not justified, so
// a continued cell obeys the same rule as a page's closing line.
func (l *layouter) paintBlockLines(page *Page, block *Box, from int, x, y, width, limit int32) int {
	for i := from; i < len(block.lines); i++ {
		if y >= limit {
			return i
		}
		line := &block.lines[i]
		fragmentLast := i == len(block.lines)-1 || satAdd(y, line.Height) >= limit
		l.paintLineAt(page, block, line, x, y, width, fragmentLast)
		for _, id := range line.Anchors {
			if _, ok := l.anchors[id]; !ok {
				l.anchors[id] = Anchor{Page: page.Number, Y: y}
			}
		}
		y = satAdd(y, line.Height)
	}
	return len(block.lines)
}

// paintStaticTable draws a table nested inside a cell: columns are computed for
// the available width and rows are painted at explicit positions.
func (l *layouter) paintStaticTable(page *Page, t *Box, x, y, width, limit int32) int32 {
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
		l.paintRowAt(page, t, i, rowY, limit, false)
		rowY += t.rowHeights[i]
	}
	return height
}

// paintStaticBlocks lays out blocks without pagination, used inside table
// cells. It returns the total height.
func (l *layouter) paintStaticBlocks(page *Page, blocks []*Box, x, y, width, limit int32) int32 {
	var h int32
	for _, b := range blocks {
		// Cell measurement already includes block margins; painting has to
		// advance by the same amounts or the drawn content drifts upwards.
		h = satAdd(h, b.Style.Margin[0].Resolve(width))
		if satAdd(y, h) < limit {
			h = satAdd(h, l.paintStaticBlock(page, b, x, satAdd(y, h), width, limit))
		} else {
			// The block starts below the page: its height still counts, but it
			// is not painted outside the sheet.
			h = satAdd(h, l.staticMeasure(b, width))
		}
		h = satAdd(h, b.Style.Margin[2].Resolve(width))
	}
	return h
}

// staticMeasure is the height paintStaticBlock would report without painting.
func (l *layouter) staticMeasure(b *Box, width int32) int32 {
	inner := b.contentWidth()
	var contentH int32
	switch {
	case b.Kind == Table:
		if !l.tableLayout(b, inner) {
			return 0
		}
		b.rowHeights = l.rowHeights(b)
		for _, rh := range b.rowHeights {
			contentH = satAdd(contentH, rh)
		}
	case b.Kind == Image:
		if b.Image == nil {
			return 0
		}
		_, h := imageSize(b.Style, b.Image, inner, l.ctx)
		contentH = h
	case l.isTextBlock(b):
		if !b.built {
			l.buildLines(b, inner)
		}
		for i := range b.lines {
			contentH = satAdd(contentH, b.lines[i].Height)
		}
	default:
		for _, c := range b.Children {
			contentH = satAdd(contentH, l.measure(c, inner))
		}
	}
	return l.staticHeight(b, contentH)
}

func (l *layouter) paintStaticBlock(page *Page, b *Box, x, y, width, limit int32) int32 {
	if !b.started {
		b.started = true
		b.usedGeometry(x, width)
	}
	l.registerAnchor(b, page.Number, y)
	if b.Kind == Table {
		return l.paintStaticTable(page, b, b.x, y, b.width, limit)
	}
	opStart := len(page.Ops)
	if b.Kind == ListItem {
		// The marker belongs to the first line. It is painted before any
		// content, exactly like the paginated path does on entry.
		b.listMarkerY = satAdd(satAdd(y, b.borderTop()), b.padTop())
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
			if lineY >= limit {
				// The line starts below the page: it is clipped, so the cell's
				// text stays inside the sheet. The loss is reported once for
				// the box instead of silently dropping content.
				l.warnOverflow(b, "content is taller than the page and is clipped")
				contentH = satAdd(contentH, b.lines[i].Height)
				lineY = satAdd(lineY, b.lines[i].Height)
				continue
			}
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
			satAdd(satAdd(y, b.borderTop()), b.padTop()), inner, limit)
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
