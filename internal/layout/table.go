package layout

import (
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
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
// by cells that continue from earlier rows.
func (l *layouter) buildGrid(t *Box) {
	occupied := make([]map[int]bool, len(t.Rows))
	for i := range occupied {
		occupied[i] = map[int]bool{}
	}
	for r, row := range t.Rows {
		col := 0
		for _, cell := range row.Cells {
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
}

// measureTable returns the content height of a table.
func (l *layouter) measureTable(t *Box, width int32) int32 {
	if t.columns == nil {
		t.columns = l.computeColumns(t, width)
	}
	var h int32
	for i, row := range t.Rows {
		h += l.rowHeight(row, t, i)
	}
	return h
}

// rowHeight returns the height of the row at index. A rowspan is clamped to the
// rows that actually follow, so a cell spanning past the table does not halve
// the height of the only row it occupies.
func (l *layouter) rowHeight(row *Row, t *Box, index int) int32 {
	var h int32
	for _, cell := range row.Cells {
		span := normSpan(cell.ColSpan)
		var w int32
		for i := 0; i < span && cell.col+i < len(t.columns); i++ {
			w += t.columns[cell.col+i]
		}
		cell.width = w
		cell.x = t.x
		cellH := l.measure(cell, w)
		rowspan := normSpan(cell.RowSpan)
		if remaining := len(t.Rows) - index; rowspan > remaining {
			rowspan = remaining
		}
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
	return h
}

// placeTable places rows until the page is full.
func (l *layouter) placeTable(f *flow, t *Box, width int32) bool {
	if !t.built {
		l.buildGrid(t)
		t.columns = l.computeColumns(t, width)
		t.rowHeights = make([]int32, len(t.Rows))
		for i, row := range t.Rows {
			t.rowHeights[i] = l.rowHeight(row, t, i)
		}
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
		h := t.rowHeights[t.cursor.row]
		if f.y+h > f.bottom {
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
				l.diags.Warnf("layout/overflow", diag.Position{}, nodePath(t), "a table row is taller than the page and is clipped")
			} else {
				return false
			}
		}
		l.paintRow(f, t, t.cursor.row)
		f.y += h
		t.cursor.row++
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
		f.y += l.paintRow(f, t, i)
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
	startX := t.x + t.borderLeft()
	if row.Style != nil && row.Style.Background.Set {
		// The row background is painted before the cells so that opaque cell
		// backgrounds still cover it.
		var rowWidth int32
		for _, w := range t.columns {
			rowWidth += w
		}
		if rowWidth > 0 && h > 0 {
			l.ops++
			page.Ops = append(page.Ops, Op{
				Kind:  FillOpKind,
				Rect:  Rect{X: startX, Y: y, W: rowWidth, H: h},
				Color: row.Style.Background,
			})
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

	inner := w - cell.padLeft() - cell.padRight() - leftInset - rightInset
	if inner < 0 {
		inner = 0
	}
	contentX := x + leftInset + cell.padLeft()
	contentY := y + topInset + cell.padTop()
	contentH := int32(0)
	for _, c := range cell.Children {
		contentH += l.measure(c, inner)
	}
	availH := h - topInset - bottomInset - cell.padTop() - cell.padBottom()
	switch cell.Style.VerticalAlign {
	case css.VAMiddle:
		contentY += (availH - contentH) / 2
	case css.VABottom:
		contentY += availH - contentH
	}
	if contentY < y+bt+cell.padTop() {
		contentY = y + bt + cell.padTop()
	}
	l.paintStaticBlocks(page, cell.Children, contentX, contentY, inner)
}

// paintStaticTable draws a table nested inside a cell: columns are computed for
// the available width and rows are painted at explicit positions.
func (l *layouter) paintStaticTable(page *Page, t *Box, x, y, width int32) int32 {
	inner := width - t.padLeft() - t.padRight() - t.borderLeft() - t.borderRight()
	if inner < 0 {
		inner = 0
	}
	// The nested table is measured for this width; a table measured earlier on
	// a different width must not reuse its columns.
	l.buildGrid(t)
	t.columns = l.computeColumns(t, inner)
	t.rowHeights = make([]int32, len(t.Rows))
	var rows int32
	for i, row := range t.Rows {
		t.rowHeights[i] = l.rowHeight(row, t, i)
		rows += t.rowHeights[i]
	}
	t.built = true
	t.x = x
	t.width = width
	height := t.borderTop() + t.padTop() + rows + t.padBottom() + t.borderBottom()
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
				X: b.x + b.borderLeft() + b.padLeft(),
				Y: y + b.borderTop() + b.padTop(),
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
		lineY := y + b.borderTop() + b.padTop()
		for i := range b.lines {
			l.paintLineAt(page, b, &b.lines[i], b.x+b.borderLeft()+b.padLeft(), lineY, inner, i == len(b.lines)-1)
			for _, id := range b.lines[i].Anchors {
				if _, ok := l.anchors[id]; !ok {
					l.anchors[id] = Anchor{Page: page.Number, Y: lineY}
				}
			}
			contentH += b.lines[i].Height
			lineY += b.lines[i].Height
		}
	} else {
		contentH = l.paintStaticBlocks(page, b.Children, b.x+b.borderLeft()+b.padLeft(), y+b.borderTop()+b.padTop(), inner)
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
	return b.borderTop() + b.padTop() + contentH + b.padBottom() + b.borderBottom()
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
	page.Ops = insertOps(page.Ops, opStart, ops)
}
