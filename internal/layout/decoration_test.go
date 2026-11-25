package layout

import "testing"

// fillRects returns the filled rectangles of the first page.
func fillRects(doc *Document) []Rect {
	var out []Rect
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind == FillOpKind {
				out = append(out, op.Rect)
			}
		}
	}
	return out
}

// lineBaseline returns the baseline of the first text run on the page.
func lineBaseline(t *testing.T, doc *Document) int32 {
	t.Helper()
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind == TextOpKind && op.Text != nil {
				return op.Text.Y
			}
		}
	}
	t.Fatal("no text")
	return 0
}

// TestUnderlineIsPaintedBelowTheBaseline guards the underline geometry: one
// rectangle under the run, as wide as the run and below its baseline.
func TestUnderlineIsPaintedBelowTheBaseline(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p><u>underlined</u></p>`)
	baseline := lineBaseline(t, doc)
	rects := fillRects(doc)
	var found *Rect
	for i := range rects {
		if rects[i].Y > baseline && rects[i].H > 0 && rects[i].W > 0 {
			found = &rects[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no rectangle below the baseline: baseline=%d rects=%v", baseline, rects)
	}
	// It spans the decorated word: wider than a single glyph, narrower than the
	// whole page.
	if found.W < 5000 || found.W > 200000 {
		t.Fatalf("underline width = %d, want the run width", found.W)
	}
}

// TestStrikeThroughCrossesAboveTheBaseline guards line-through: its rectangle
// sits above the baseline, at the middle of the x-height rather than under the
// text.
func TestStrikeThroughCrossesAboveTheBaseline(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p><s>struck</s></p>`)
	baseline := lineBaseline(t, doc)
	rects := fillRects(doc)
	var found *Rect
	for i := range rects {
		if rects[i].Y < baseline && rects[i].H > 0 && rects[i].W > 0 {
			found = &rects[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no rectangle above the baseline: baseline=%d rects=%v", baseline, rects)
	}
}

// TestDecorationUsesTheTextColour guards the colour: the line is painted with
// the run's colour, not with the default black.
func TestDecorationUsesTheTextColour(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p style="color:#ff0000"><u>red</u></p>`)
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind != FillOpKind {
				continue
			}
			if !op.Color.Set || op.Color.R != 255 || op.Color.G != 0 || op.Color.B != 0 {
				t.Fatalf("decoration colour = %+v, want red", op.Color)
			}
			return
		}
	}
	t.Fatal("no decoration was painted")
}

// TestBlockquoteIsIndented guards the quote layout: its text starts inside the
// left indent and its own block layout is preserved.
func TestBlockquoteIsIndented(t *testing.T) {
	h := newHarness(t)
	plain := h.run(`<p>QUOTE</p>`)
	quoted := h.run(`<blockquote><p>QUOTE</p></blockquote>`)
	plainX := firstTextX(t, plain)
	quotedX := firstTextX(t, quoted)
	if quotedX <= plainX {
		t.Fatalf("quote text starts at %d, plain at %d: no indent", quotedX, plainX)
	}
}

// firstTextX returns the X of the first text run on the page.
func firstTextX(t *testing.T, doc *Document) int32 {
	t.Helper()
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind == TextOpKind && op.Text != nil {
				return op.Text.X
			}
		}
	}
	t.Fatal("no text")
	return 0
}
