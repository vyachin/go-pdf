package layout

import (
	"math"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
)

const (
	// The harness page is A4 with 20 mm margins, so the containing block is
	// 481,890 millipoints wide and starts 56,693 in from the page edge.
	geometryContaining = 481890
	geometryLeft       = 56693
)

func fillOps(doc *Document) []Op {
	var out []Op
	for _, p := range doc.Pages {
		for _, op := range p.Ops {
			if op.Kind == FillOpKind {
				out = append(out, op)
			}
		}
	}
	return out
}

// TestPercentageWidthAndMarginUseContainingBlock guards the base of a
// percentage: width is half the containing block and the left margin is a tenth
// of it, so the border box is 240,945 wide and starts 48,189 in.
func TestPercentageWidthAndMarginUseContainingBlock(t *testing.T) {
	h := newHarness(t)
	page := h.run(`<div style="width:50%; margin-left:10%; height:100pt; background-color:#ff0000"></div>`)
	fills := fillOps(page)
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want one background", len(fills))
	}
	f := fills[0]
	if f.Rect.X != geometryLeft+48189 {
		t.Fatalf("background X = %d, want %d", f.Rect.X, geometryLeft+48189)
	}
	if f.Rect.W != 240945 {
		t.Fatalf("background width = %d, want 240945", f.Rect.W)
	}
}

// TestPercentagePaddingUsesContainingBlock guards that padding resolves
// against the containing block width, not against the box's own width.
func TestPercentagePaddingUsesContainingBlock(t *testing.T) {
	h := newHarness(t)
	page := h.run(`<div style="width:50%; padding:0 10%; background-color:#ff0000">X</div>`)
	fills := fillOps(page)
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want one background", len(fills))
	}
	if got := fills[0].Rect.W; got != 337323 {
		t.Fatalf("border box width = %d, want 337323", got)
	}
	texts := textOps(page)
	if len(texts) == 0 || texts[0].X != geometryLeft+48189 {
		t.Fatalf("text X = %v, want %d", texts, geometryLeft+48189)
	}
}

// TestPercentageBottomMarginUsesContainingBlock guards the vertical gap: a 10 %
// bottom margin is 48,189, not 10 % of the paragraph's own width.
func TestPercentageBottomMarginUsesContainingBlock(t *testing.T) {
	h := newHarness(t)
	// The paragraph is narrower than its containing block, so a base of its own
	// width would give 10,000 instead of 48,189.
	base := textOps(h.run(`<p style="width:100pt; margin:0">a</p><p style="margin:0">b</p>`))
	pct := textOps(h.run(`<p style="width:100pt; margin:0 0 10% 0">a</p><p style="margin:0">b</p>`))
	if len(base) < 2 || len(pct) < 2 {
		t.Fatalf("missing text: base %d, pct %d", len(base), len(pct))
	}
	baseGap := base[1].Y - base[0].Y
	pctGap := pct[1].Y - pct[0].Y
	if got := pctGap - baseGap; got != 48189 {
		t.Fatalf("10%% margin added %d, want 48189", got)
	}
}

// TestPercentageTextIndentUsesContainingBlock guards that an indent of a block
// with a 100 pt content width still resolves against the containing block.
func TestPercentageTextIndentUsesContainingBlock(t *testing.T) {
	h := newHarness(t)
	page := h.run(`<div style="width:100pt; text-indent:10%">X</div>`)
	texts := textOps(page)
	if len(texts) == 0 {
		t.Fatal("no text")
	}
	if got := texts[0].X; got != geometryLeft+48189 {
		t.Fatalf("text X = %d, want %d", got, geometryLeft+48189)
	}
}

// TestZeroWidthBlockPaintsNothing guards width: 0: it must stay zero instead of
// falling back to the available width.
func TestZeroWidthBlockPaintsNothing(t *testing.T) {
	h := newHarness(t)
	page := h.run(`<div style="width:0; height:50pt; background-color:#ff0000"></div>`)
	for _, f := range fillOps(page) {
		if f.Rect.W > 0 {
			t.Fatalf("a zero-width block painted a %d-wide fill", f.Rect.W)
		}
	}
}

// TestHugePaddingIsClampedAndReported guards the two-padding overflow: the
// sums saturate and the clamp is reported instead of painting at a wrapped X.
func TestHugePaddingIsClampedAndReported(t *testing.T) {
	h := newHarness(t)
	page, diags := h.runRaw(`<div style="padding-left:2000000pt"><div style="padding-left:2000000pt">X</div></div>`, h.ctx.PageSize, h.ctx.Margin)
	found := false
	for _, d := range diags {
		if d.Code == "layout/overflow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no layout/overflow diagnostic:\n%s", diags.Lines())
	}
	for _, op := range textOps(page) {
		if op.X < 0 {
			t.Fatalf("text painted at X = %d", op.X)
		}
	}
}

// TestImageGeometryClamps guards the image scale arithmetic: a huge intrinsic
// width must stay positive through the aspect-ratio division and coverRect.
func TestImageGeometryClamps(t *testing.T) {
	img := &ImageData{Width: math.MaxInt32, Height: 1500}
	w, h := imageSize(css.DefaultStyle(), img, 100000, Context{})
	if w <= 0 || h <= 0 {
		t.Fatalf("image size = %d x %d, want positive", w, h)
	}
	rect := coverRect(img, Rect{X: 0, Y: 0, W: 100000, H: 200})
	if rect.W <= 0 || rect.H <= 0 {
		t.Fatalf("cover rect = %+v, want positive", rect)
	}
}

// TestPageMarginEmResolvesAtInitialFontSize guards @page margin: 1em: it is
// 12 pt, not a fraction of the page width.
func TestPageMarginEmResolvesAtInitialFontSize(t *testing.T) {
	h := newHarness(t)
	page := h.run(`<style>@page { margin: 1em }</style><p>x</p>`)
	if got := page.Pages[0].Margin; got != [4]int32{12000, 12000, 12000, 12000} {
		t.Fatalf("page margin = %v, want 12 pt on every side", got)
	}
}

// TestPortraitFlipsLandscapeFallback guards the orientation of the fallback:
// a landscape run size with size: portrait becomes 200 x 300 mm.
func TestPortraitFlipsLandscapeFallback(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 850394, Height: 566929}
	page := h.runWithSize(`<style>@page { size: portrait }</style><p>x</p>`, size, [4]int32{56693, 56693, 56693, 56693})
	if got := page.Pages[0].Size; got.Width != 566929 || got.Height != 850394 {
		t.Fatalf("page size = %v, want 566929 x 850394", got)
	}
}
