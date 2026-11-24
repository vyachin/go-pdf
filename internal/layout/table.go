package layout

import (
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
)

// computeColumns implements the fixed table layout: widths come from the first
// row, the rest of the available width is split evenly between the remaining
// columns.
func (l *layouter) computeColumns(t *Box, available int32) []int32 {
	n := 0
	for _, row := range t.Rows {
		c := 0
		for _, cell := range row.Cells {
			span := cell.ColSpan
			if span < 1 {
				span = 1
			}
			c += span
		}
		if c > n {
			n = c
		}
	}
	if n == 0 {
		return nil
	}
	widths := make([]int32, n)
	if len(t.Rows) > 0 {
		col := 0
		for _, cell := range t.Rows[0].Cells {
			span := cell.ColSpan
			if span < 1 {
				span = 1
			}
			var w int32
			switch cell.Style.Width.Kind {
			case css.MP:
				w = cell.Style.Width.Value
			case css.Percent:
				w = cell.Style.Width.Resolve(available)
			}
			if w > 0 {
				per := w / int32(span)
				for i := 0; i < span && col+i < n; i++ {
					widths[col+i] = per
				}
			}
			col += span
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
	col := 0
	for _, cell := range row.Cells {
		span := cell.ColSpan
		if span < 1 {
			span = 1
		}
		var w int32
		for i := 0; i < span && col+i < len(t.columns); i++ {
			w += t.columns[col+i]
		}
		cell.width = w
		cell.x = t.x
		cellH := l.measure(cell, w)
		if cell.RowSpan > 1 {
			cellH = cellH / int32(cell.RowSpan)
		}
		if cellH > h {
			h = cellH
		}
		col += span
	}
	return h
}

// placeTable places rows until the page is full.
func (l *layouter) placeTable(f *flow, t *Box, width int32) bool {
	if !t.built {
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
	// Repeat the header on every page that continues the table.
	if t.headerPage != f.pageNum && t.cursor.row > 0 && t.headerCount > 0 {
		for i := 0; i < t.headerCount; i++ {
			f.y += l.paintRow(f, t, i)
			f.empty = false
		}
	}
	t.headerPage = f.pageNum

	for t.cursor.row < len(t.Rows) {
		row := t.Rows[t.cursor.row]
		if row.Header && t.cursor.row < t.headerCount && t.cursor.row > 0 {
			// Already painted as page header above.
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

// paintRow draws a row and its cells and returns its height.
func (l *layouter) paintRow(f *flow, t *Box, index int) int32 {
	row := t.Rows[index]
	h := t.rowHeights[index]
	x := t.x + t.borderLeft()
	y := f.y
	col := 0
	for _, cell := range row.Cells {
		span := cell.ColSpan
		if span < 1 {
			span = 1
		}
		var w int32
		for i := 0; i < span && col+i < len(t.columns); i++ {
			w += t.columns[col+i]
		}
		cellH := h
		if cell.RowSpan > 1 {
			cellH = 0
			for r := index; r < index+cell.RowSpan && r < len(t.rowHeights); r++ {
				cellH += t.rowHeights[r]
			}
		}
		l.paintCell(f.page, cell, x, y, w, cellH, index == 0, col == 0)
		x += w
		col += span
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
	l.paintStaticBlocks(page, cell.Children, contentX, contentY, inner)
}

// paintStaticBlocks lays out blocks without pagination, used inside table
// cells. It returns the total height.
func (l *layouter) paintStaticBlocks(page *Page, blocks []*Box, x, y, width int32) int32 {
	var h int32
	for _, b := range blocks {
		h += l.paintStaticBlock(page, b, x, y+h, width)
	}
	return h
}

func (l *layouter) paintStaticBlock(page *Page, b *Box, x, y, width int32) int32 {
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
