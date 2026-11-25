package layout

import (
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
)

// findBox returns the first box matching pred.
func findBox(root *Box, pred func(*Box) bool) *Box {
	if pred(root) {
		return root
	}
	for _, c := range root.Children {
		if found := findBox(c, pred); found != nil {
			return found
		}
	}
	return nil
}

// tableWithHeights builds the box tree of src and returns the table box with
// its columns and row heights computed for the harness page. The table is
// measured exactly as placement measures it: at its content width.
func tableWithHeights(t *testing.T, h *harness, src string) (*Box, *layouter, []int32) {
	t.Helper()
	root := h.buildTree(src)
	table := findBox(root, func(b *Box) bool { return b.Kind == Table })
	if table == nil {
		t.Fatal("the document has no table box")
	}
	table.containing = geometryContaining
	table.width = usedWidth(table.Style, geometryContaining)
	l := &layouter{ctx: h.ctx, faces: map[string]*FontFace{}}
	if !l.tableLayout(table, table.contentWidth()) {
		t.Fatal("the table grid was not built")
	}
	return table, l, l.rowHeights(table)
}

// TestRowspanDeficitGrowsCoveredRows guards the two-pass rule: a tall spanning
// cell over an empty covered row grows the last covered row so the covered rows
// sum to the cell's outer height.
func TestRowspanDeficitGrowsCoveredRows(t *testing.T) {
	h := newHarness(t)
	src := `<table><tr><td rowspan="2">X<br>X<br>X<br>X<br>X<br>X</td><td>a</td></tr><tr><td></td></tr></table>`
	table, l, heights := tableWithHeights(t, h, src)
	if len(heights) != 2 {
		t.Fatalf("got %d row heights, want 2", len(heights))
	}
	cell := table.Rows[0].Cells[0]
	cellH := l.measure(cell, cell.width)
	covered := heights[0] + heights[1]
	if covered < cellH {
		t.Fatalf("covered rows sum to %d, want at least the cell's %d", covered, cellH)
	}
	if heights[1] <= 0 {
		t.Fatalf("the empty covered row did not grow: %v", heights)
	}
}

// TestTableMeasurementMatchesPlacement guards that the measured table height
// equals the sum of the row heights placement uses when a deficit exists.
func TestTableMeasurementMatchesPlacement(t *testing.T) {
	h := newHarness(t)
	src := `<table style="background-color:#ff0000"><tr><td rowspan="2">X<br>X<br>X<br>X<br>X<br>X</td><td>a</td></tr><tr><td></td></tr></table>`
	table, l, heights := tableWithHeights(t, h, src)
	var sum int32
	for _, rh := range heights {
		sum = satAdd(sum, rh)
	}
	if measured := l.measureTable(table, table.contentWidth()); measured != sum {
		t.Fatalf("measureTable = %d, sum of row heights = %d", measured, sum)
	}
	// Placement paints the table background with the height it computed.
	doc := h.run(src)
	var placed int32
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == FillOpKind && op.Color == (css.Color{R: 255, Set: true}) {
			placed = op.Rect.H
		}
	}
	if placed != sum {
		t.Fatalf("the placed table is %d tall, measurement says %d", placed, sum)
	}
}

// TestSpanningCellDoesNotOverlapFollowingBlock guards the reproduction: the
// last line of a tall rowspan cell stays above the paragraph that follows the
// table, with no overflow diagnostic.
func TestSpanningCellDoesNotOverlapFollowingBlock(t *testing.T) {
	h := newHarness(t)
	src := `<table><tr><td rowspan="2">X<br>X<br>X<br>X<br>X<br>X</td><td>a</td></tr><tr><td></td></tr></table><p style="margin:0">AFTER</p>`
	doc, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	var lastX, afterY int32 = -1, -1
	for _, op := range textOps(doc) {
		if len(op.Run.Glyphs) == 0 {
			continue
		}
		switch op.Run.Glyphs[0].Rune {
		case 'X':
			if op.Y > lastX {
				lastX = op.Y
			}
		case 'A':
			afterY = op.Y
		}
	}
	if lastX < 0 || afterY < 0 {
		t.Fatalf("missing baselines: last X = %d, AFTER = %d", lastX, afterY)
	}
	if afterY <= lastX {
		t.Fatalf("AFTER baseline %d is not below the last cell line %d", afterY, lastX)
	}
	for _, d := range diags {
		if d.Code == "layout/overflow" {
			t.Fatalf("unexpected overflow diagnostic: %s", d.String())
		}
	}
}

// TestSpanningCellWithEmptyRowStillCovers guards the empty-row shape: a row with
// only an empty cell still grows to cover the spanning cell.
func TestSpanningCellWithEmptyRowStillCovers(t *testing.T) {
	h := newHarness(t)
	src := `<table><tr><td rowspan="2">X<br>X<br>X<br>X<br>X<br>X</td><td>a</td></tr><tr></tr></table><p style="margin:0">AFTER</p>`
	doc, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	var lastX, afterY int32 = -1, -1
	for _, op := range textOps(doc) {
		if len(op.Run.Glyphs) == 0 {
			continue
		}
		switch op.Run.Glyphs[0].Rune {
		case 'X':
			if op.Y > lastX {
				lastX = op.Y
			}
		case 'A':
			afterY = op.Y
		}
	}
	if lastX < 0 || afterY < 0 || afterY <= lastX {
		t.Fatalf("AFTER baseline %d is not below the last cell line %d", afterY, lastX)
	}
	for _, d := range diags {
		if d.Code == "layout/overflow" {
			t.Fatalf("unexpected overflow diagnostic: %s", d.String())
		}
	}
}
