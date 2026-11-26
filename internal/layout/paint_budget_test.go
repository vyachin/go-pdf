package layout

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/text"
)

// TestPaintReusesShapedGlyphsWhenNoOffset guards RV5-14: a run whose glyphs
// carry no offset is painted from the shaped slice, so painting allocates no
// second glyph slice and charges no copy units.
func TestPaintReusesShapedGlyphsWhenNoOffset(t *testing.T) {
	h := newHarness(t)
	face, err := h.fonts.Face([]string{"Test"}, 400, false, 12000)
	if err != nil {
		t.Fatal(err)
	}
	run, err := text.Shape(h.fonts.f, 12000, strings.Repeat("x", 50_000))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range run.Glyphs {
		if g.XOffset != 0 || g.YOffset != 0 {
			t.Fatal("the fixture gained an offset")
		}
	}
	guard := budget.New(context.Background(), 0)
	l := &layouter{ctx: Context{Budget: guard}}
	allocs := testing.AllocsPerRun(5, func() {
		page := &Page{Ops: make([]Op, 0, 2)}
		l.paintShapedRun(page, face, css.Color{}, run, 0, 0)
	})
	if allocs > 6 {
		t.Fatalf("painting an offset-free run allocated %v times, want the page and the text operation only", allocs)
	}
	if got := guard.Used(); got != 0 {
		t.Fatalf("painting an offset-free run charged %d units, want 0", got)
	}
}

// TestPaintChargesGlyphCopies guards the offset-bearing run: the exact-size
// copy is charged one unit per glyph before it exists, a budget one unit short
// paints nothing, and the painted mark keeps its offset.
func TestPaintChargesGlyphCopies(t *testing.T) {
	h := newHarness(t)
	face, err := h.fonts.Face([]string{"Test"}, 400, false, 12000)
	if err != nil {
		t.Fatal(err)
	}
	run, err := text.Shape(h.fonts.f, 12000, "a\u0316")
	if err != nil {
		t.Fatal(err)
	}
	hasOffset := false
	for _, g := range run.Glyphs {
		if g.XOffset != 0 || g.YOffset != 0 {
			hasOffset = true
		}
	}
	if !hasOffset {
		t.Fatal("the fixture run has no mark offset")
	}
	const charge = uint64(2)

	exact := budget.New(context.Background(), int(charge))
	l := &layouter{ctx: Context{Budget: exact}}
	page := &Page{}
	l.paintShapedRun(page, face, css.Color{}, run, 0, 0)
	if got := exact.Used(); got != charge {
		t.Fatalf("the copy charged %d units, want one per glyph (%d)", got, charge)
	}
	if len(page.Ops) == 0 {
		t.Fatal("the offset-bearing run painted nothing")
	}
	mark := false
	for _, op := range page.Ops {
		if op.Text == nil {
			continue
		}
		for _, g := range op.Text.Run.Glyphs {
			if g.XOffset != 0 || g.YOffset != 0 {
				mark = true
			}
		}
	}
	if !mark {
		t.Fatal("the painted segments lost the mark offset")
	}

	short := budget.New(context.Background(), int(charge)-1)
	l = &layouter{ctx: Context{Budget: short}}
	page = &Page{}
	l.paintShapedRun(page, face, css.Color{}, run, 0, 0)
	if len(page.Ops) != 0 {
		t.Fatalf("a refused copy painted %d segments", len(page.Ops))
	}
	if !errors.Is(short.Err(), budget.ErrExceeded) {
		t.Fatalf("the refused copy did not exhaust the guard: %v", short.Err())
	}
}
