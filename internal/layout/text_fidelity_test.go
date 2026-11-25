package layout

import "testing"

// TestCombiningMarkPaintedOverBase guards the rendered position: the separate
// acute is its own text object, moved from the pen to the anchor over "о".
func TestCombiningMarkPaintedOverBase(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p style="margin:0">сло́во</p>`)
	var mark *TextOp
	var baseX, baseAdvance int32
	var baseY int32 = -1
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != TextOpKind || op.Text == nil || len(op.Text.Run.Glyphs) == 0 {
			continue
		}
		text := op.Text
		x := text.X
		for _, g := range text.Run.Glyphs {
			switch g.Rune {
			case 'о':
				// Remember the most recent о before the mark.
				if mark == nil {
					baseX, baseAdvance, baseY = x, g.Advance, text.Y
				}
			case '\u0301':
				if mark == nil {
					mark = text
				}
			}
			x += g.Advance
		}
	}
	if mark == nil || baseY < 0 {
		t.Fatalf("missing mark or base: mark=%v baseY=%d", mark, baseY)
	}
	if mark.X < baseX || mark.X >= baseX+baseAdvance {
		t.Fatalf("mark X = %d, want inside the base [%d,%d)", mark.X, baseX, baseX+baseAdvance)
	}
	if mark.Y != baseY {
		t.Fatalf("mark baseline %d, base baseline %d", mark.Y, baseY)
	}
}

// TestUnicodeSeparatorsRenderTwoLines guards form feed and the Unicode line
// separator in text layout.
func TestUnicodeSeparatorsRenderTwoLines(t *testing.T) {
	h := newHarness(t)
	cases := []string{
		"<div style=\"white-space: pre\">A\fB</div>",
		"<div>A\u2028B</div>",
		"<div>A\u2029B</div>",
	}
	for _, src := range cases {
		doc, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
		for _, d := range diags {
			if d.Code == "text/glyph" {
				t.Fatalf("%q reported %s", src, d.String())
			}
		}
		ys := map[int32]bool{}
		for _, op := range textOps(doc) {
			ys[op.Y] = true
		}
		if len(ys) != 2 {
			t.Fatalf("%q rendered %d baselines, want 2", src, len(ys))
		}
	}
}
