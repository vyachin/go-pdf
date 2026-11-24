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
	for _, row := range t.Rows {
		h += l.rowHeight(row, t)
	}
	return h
}

// rowHeight returns the height of a row.
func (l *layouter) rowHeight(row *Row, t *Box) int32 {
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
		if rowspan := normSpan(cell.RowSpan); rowspan > 1 {
			cellH = cellH / int32(rowspan)
		}
		if cellH > h {
			h = cellH
		}
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
			t.rowHeights[i] = l.rowHeight(row, t)
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
		row := t.Rows[t.cursor.row]
		if repeated && row.Header && t.cursor.row < t.headerCount {
			// Already painted as the repeated header above.
			t.cursor.row++
			continue
		}
		h := t.rowHeights[t.cursor.row]
		if f.y+h > f.bottom {
			if f.empty {
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
		l.paintCell(page, cell, x, y, w, cellH, index == 0, cell.col == 0)
	}
	return h
}

// paintCell draws one cell: background, borders and content. With collapsed
// borders each cell draws only the edges it owns, so a shared border is painted
// exactly once.
func (l *layouter) paintCell(page *Page, cell *Box, x, y, w, h int32, firstRow, firstCol bool) {
	cell.x = x
	cell.width = w
	if cell.Style.Background.Set {
		l.ops++
		page.Ops = append(page.Ops, Op{Kind: FillOpKind, Rect: Rect{X: x, Y: y, W: w, H: h}, Color: cell.Style.Background})
	}
	paintSide := func(side int, rect Rect) {
		border := cell.Style.Border[side]
		if !border.IsVisible() {
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
	if collapse {
		if firstCol {
			paintSide(3, Rect{X: x, Y: y, W: bl, H: h})
		}
		if firstRow {
			paintSide(0, Rect{X: x, Y: y, W: w, H: bt})
		}
		paintSide(1, Rect{X: x + w - br, Y: y, W: br, H: h})
		paintSide(2, Rect{X: x, Y: y + h - bb, W: w, H: bb})
	} else {
		paintSide(3, Rect{X: x, Y: y, W: bl, H: h})
		paintSide(1, Rect{X: x + w - br, Y: y, W: br, H: h})
		paintSide(0, Rect{X: x, Y: y, W: w, H: bt})
		paintSide(2, Rect{X: x, Y: y + h - bb, W: w, H: bb})
	}

	inner := w - cell.padLeft() - cell.padRight() - bl - br
	if inner < 0 {
		inner = 0
	}
	contentX := x + bl + cell.padLeft()
	contentY := y + bt + cell.padTop()
	contentH := int32(0)
	for _, c := range cell.Children {
		contentH += l.measure(c, inner)
	}
	availH := h - bt - bb - cell.padTop() - cell.padBottom()
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
		t.rowHeights[i] = l.rowHeight(row, t)
		rows += t.rowHeights[i]
	}
	t.built = true
	t.x = x
	height := t.borderTop() + t.padTop() + rows + t.padBottom() + t.borderBottom()
	if t.Style.Background.Set && height > 0 {
		l.ops++
		page.Ops = append(page.Ops, Op{Kind: FillOpKind, Rect: Rect{X: x, Y: y, W: width, H: height}, Color: t.Style.Background})
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
	if b.Kind == Table {
		return l.paintStaticTable(page, b, x, y, width)
	}
	if b.Kind == Image {
		if b.Image == nil {
			return 0
		}
		w, h := imageSize(b.Style, b.Image, width, l.ctx)
		l.ops++
		page.Ops = append(page.Ops, Op{
			Kind:  ImageOpKind,
			Rect:  Rect{X: x, Y: y, W: w, H: h},
			Image: &ImageOp{Image: b.Image},
		})
		return h
	}
	b.x = x
	b.width = width
	if b.width > width {
		b.width = width
	}
	inner := b.contentWidth()
	if l.isTextBlock(b) {
		if !b.built {
			l.buildLines(b, inner)
		}
		height := l.measure(b, width)
		if b.Style.Background.Set {
			page.Ops = append(page.Ops, Op{Kind: FillOpKind, Rect: Rect{X: x, Y: y, W: width, H: height}, Color: b.Style.Background})
		}
		lineY := y + b.borderTop() + b.padTop()
		for i := range b.lines {
			l.paintLineAt(page, b, &b.lines[i], x+b.borderLeft()+b.padLeft(), lineY, inner, i == len(b.lines)-1)
			for _, id := range b.lines[i].Anchors {
				if _, ok := l.anchors[id]; !ok {
					l.anchors[id] = Anchor{Page: page.Number, Y: lineY}
				}
			}
			lineY += b.lines[i].Height
		}
		return height
	}
	height := b.borderTop() + b.padTop()
	childY := y + height
	childH := l.paintStaticBlocks(page, b.Children, x+b.borderLeft()+b.padLeft(), childY, inner)
	height += childH + b.padBottom() + b.borderBottom()
	return height
}
