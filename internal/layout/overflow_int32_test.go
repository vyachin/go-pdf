package layout

import (
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
)

// countElementWarnings counts warnings for an element path containing element.
func countElementWarnings(diags diag.List, code, element string) int {
	n := 0
	for _, d := range diags {
		if d.Code == code && strings.Contains(d.Element, element) {
			n++
		}
	}
	return n
}

// countMessageWarnings counts warnings whose message contains substr.
func countMessageWarnings(diags diag.List, code, substr string) int {
	n := 0
	for _, d := range diags {
		if d.Code == code && strings.Contains(d.Message, substr) {
			n++
		}
	}
	return n
}

// findText returns the page and baseline of the first text op whose run contains
// substr.
func findText(doc *Document, substr string) (page int, y int32, ok bool) {
	for i, p := range doc.Pages {
		for _, op := range p.Ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			var text strings.Builder
			for _, g := range op.Text.Run.Glyphs {
				text.WriteRune(g.Rune)
			}
			if strings.Contains(text.String(), substr) {
				return i, op.Text.Y, true
			}
		}
	}
	return 0, 0, false
}

// TestMeasureHugeHeightDoesNotWrap guards the measurement sum: a padding and a
// declared height near the int32 limit must not wrap into a negative height, and
// measuring the same box twice must agree.
func TestMeasureHugeHeightDoesNotWrap(t *testing.T) {
	const big = 2_000_000 * 1000 // 2,000,000 pt in millipoints
	st := css.DefaultStyle()
	st.Padding = [4]css.Length{css.Abs(big), {}, css.Abs(big), {}}
	st.Height = css.Abs(big)
	b := &Box{Kind: Block, Style: st}
	l := &layouter{ctx: Context{}}
	got := l.measure(b, 100000)
	if got < 0 {
		t.Fatalf("measured height = %d, want non-negative", got)
	}
	if again := l.measure(b, 100000); again != got {
		t.Fatalf("measure is not stable: %d then %d", got, again)
	}
}

// TestHugePaddingHeightReportsOneOverflow guards the end-to-end result: a box
// whose padding and height overflow the range reports exactly one overflow
// instead of silently losing its content.
func TestHugePaddingHeightReportsOneOverflow(t *testing.T) {
	h := newHarness(t)
	_, diags := h.runRaw(
		`<div style="padding:2000000pt;height:2000000pt">x</div>`,
		h.ctx.PageSize, h.ctx.Margin,
	)
	if n := countElementWarnings(diags, "layout/overflow", "div"); n != 1 {
		t.Fatalf("the div reported layout/overflow %d times, want 1:\n%s", n, diags.Lines())
	}
}

// TestHugeLineHeightCellReportsAndKeepsContent guards the huge line-height cell:
// the clipped line is reported once, and the following paragraph is not painted
// above the cell.
func TestHugeLineHeightCellReportsAndKeepsContent(t *testing.T) {
	h := newHarness(t)
	doc, diags := h.runRaw(
		`<table><tr><td style="line-height:2000000pt">A<br>B</td></tr></table><p>after</p>`,
		h.ctx.PageSize, h.ctx.Margin,
	)
	if n := countMessageWarnings(diags, "layout/overflow", "content is taller than the page"); n != 1 {
		t.Fatalf("the clipped cell reported layout/overflow %d times, want 1:\n%s", n, diags.Lines())
	}
	aPage, aY, ok := findText(doc, "A")
	if !ok {
		t.Fatal("the cell's first line is missing")
	}
	afterPage, afterY, ok := findText(doc, "after")
	if !ok {
		t.Fatal("the following paragraph is missing")
	}
	if afterPage < aPage || (afterPage == aPage && afterY <= aY) {
		t.Fatalf("the following paragraph is at page %d y=%d, the cell line at page %d y=%d",
			afterPage, afterY, aPage, aY)
	}
}

// TestHugeBottomPaddingDoesNotWrapY guards the flow's bottom inset: a huge bottom
// padding moves the next block forward instead of wrapping its Y.
func TestHugeBottomPaddingDoesNotWrapY(t *testing.T) {
	h := newHarness(t)
	doc, _ := h.runRaw(
		`<div style="padding-bottom:2000000pt">x</div><p>after</p>`,
		h.ctx.PageSize, h.ctx.Margin,
	)
	divPage, divY, ok := findText(doc, "x")
	if !ok {
		t.Fatal("the padded div's text is missing")
	}
	afterPage, afterY, ok := findText(doc, "after")
	if !ok {
		t.Fatal("the following paragraph is missing")
	}
	if afterPage < divPage || (afterPage == divPage && afterY <= divY) {
		t.Fatalf("the following paragraph is at page %d y=%d, the div text at page %d y=%d",
			afterPage, afterY, divPage, divY)
	}
}
