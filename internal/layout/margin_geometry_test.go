package layout

import (
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
)

// TestHugeMarginClampsTheContentBox guards RV4-32: the content width and the
// page bottom were plain subtractions, so a margin larger than the page wrapped
// them into negative or huge values and painted coordinates flipped sides.
func TestHugeMarginClampsTheContentBox(t *testing.T) {
	const huge = int32(2_000_000_000)
	l := &layouter{
		doc: &Document{},
		pageStyle: PageStyle{
			Size:   css.PageSize{Width: 595276, Height: 841890},
			Margin: [4]int32{huge, huge, huge, huge},
		},
	}
	f := &flow{l: l}
	if !f.newPage() {
		t.Fatal("the first page was refused")
	}
	if f.width < 0 || f.bottom < 0 || f.contentHeight() < 0 {
		t.Fatalf("the content box is negative: width=%d bottom=%d height=%d", f.width, f.bottom, f.contentHeight())
	}
	// The arithmetic saturates instead of wrapping to the opposite side.
	if f.bottom != 0 {
		t.Fatalf("bottom = %d, want 0", f.bottom)
	}
	if satAdd(2_000_000_000, 2_000_000_000) != 2_147_483_647 {
		t.Fatal("the saturating addition wraps")
	}
	if satSub(-2_000_000_000, 2_000_000_000) != -2_147_483_648 {
		t.Fatal("the saturating subtraction wraps")
	}
}

// TestTableColumnsFitTheContentBox guards RV4-15: a declared 1000 pt column
// pushed the next cell off the page with no diagnostic. The columns are scaled
// to the content width and the table reports one layout/overflow.
func TestTableColumnsFitTheContentBox(t *testing.T) {
	h := newHarness(t)
	doc, diags := h.runRaw(`<table><tr><td style="width:1000pt">WIDE</td><td>x</td></tr></table>`, h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	found := false
	for _, d := range diags {
		if d.Code == "layout/overflow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the oversized table was not reported: %v", diags.Lines())
	}
	// Every painted rectangle stays inside the page.
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Rect.X+op.Rect.W > page.Size.Width || op.Rect.X < 0 {
				t.Fatalf("an operation is outside the page: x=%d w=%d", op.Rect.X, op.Rect.W)
			}
		}
	}
}

// TestLongWordIsReported guards RV4-16: a thousand-character word — and any run
// with no break opportunity, such as a CJK paragraph — produced no diagnostic,
// so the text was clipped at the page edge with the tail silently lost. Each
// box reports one layout/overflow.
func TestLongWordIsReported(t *testing.T) {
	h := newHarness(t)
	cases := map[string]string{
		"long word":    "<p>" + repeatRune('ж', 1000) + "</p>",
		"no-space run": "<p>" + repeatRune('ы', 400) + "</p>",
	}
	for name, src := range cases {
		_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
		found := false
		for _, d := range diags {
			if d.Code == "layout/overflow" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no layout/overflow in %v", name, diags.Lines())
		}
	}
	// Ordinary text does not warn.
	_, diags := h.runRaw(`<p>`+textOfWords(200)+`</p>`, h.ctx.PageSize, h.ctx.Margin)
	for _, d := range diags {
		if d.Code == "layout/overflow" {
			t.Fatalf("ordinary text warned: %v", d.String())
		}
	}
}

// repeatRune returns s repeated n times.
func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}

// textOfWords returns n short words separated by spaces.
func textOfWords(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("word ")
	}
	return b.String()
}

// TestDeclaredHeightIsReservedOnce guards RV4-14: a box split across pages
// reserved its whole declared height again on the continuation, painting the
// 400 pt background twice. The declared height is a total for the box, so the
// fragments together occupy the content height once.
func TestDeclaredHeightIsReservedOnce(t *testing.T) {
	h := newHarness(t)
	var b strings.Builder
	b.WriteString(`<div style="height:400pt;background-color:#ff0000">`)
	for i := 0; i < 54; i++ {
		b.WriteString("line<br>")
	}
	b.WriteString(`</div><p>after</p>`)
	doc := h.run(b.String())
	if len(doc.Pages) < 2 {
		t.Fatalf("the box did not split: %d pages", len(doc.Pages))
	}
	var total int32
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind == FillOpKind {
				total += op.Rect.H
			}
		}
	}
	// 54 lines at the harness font: about 777 pt of content. The declared
	// 400 pt is smaller than the content, so it must not be added again.
	const want = 777600
	if total > want+2000 || total < want-2000 {
		t.Fatalf("the fragments occupy %d mp, want the content height %d (a second reservation would be %d)",
			total, want, want+400000)
	}
	if !strings.Contains(pageText(doc, len(doc.Pages)), "after") {
		t.Fatal("the paragraph after the box is not on the last page")
	}
}

// TestRowspanCellIsClippedAndContinued guards RV4-13: a rowspan cell crossing
// a page break was painted once with the height of all covered rows, so its
// background and text ran past the page and no fragment appeared on the next
// page. The cell is clipped to the page and continued on the following one.
func TestRowspanCellIsClippedAndContinued(t *testing.T) {
	h := newHarness(t)
	var body strings.Builder
	body.WriteString(`<table><tr><td rowspan="60" style="background-color:#00ff00">SPAN</td><td>r1</td></tr>`)
	for i := 2; i <= 60; i++ {
		body.WriteString("<tr><td>r")
		body.WriteString(strconv.Itoa(i))
		body.WriteString("</td></tr>")
	}
	body.WriteString("</table>")
	h.ctx.PageSize = css.PageSize{Width: 283465, Height: 170079}
	h.ctx.Margin = [4]int32{14173, 14173, 14173, 14173}
	doc := h.run(body.String())
	if len(doc.Pages) < 2 {
		t.Fatalf("the table produced %d pages, want at least 2", len(doc.Pages))
	}
	for _, page := range doc.Pages {
		bottom := satSub(page.Size.Height, page.Margin[2])
		for _, op := range page.Ops {
			if op.Rect.Y > bottom+1 || op.Rect.Y+op.Rect.H > bottom+1 {
				t.Fatalf("an operation paints outside the page: y=%d h=%d bottom=%d", op.Rect.Y, op.Rect.H, bottom)
			}
		}
	}
	// The continuation fragment is painted on the following page.
	continued := false
	for _, op := range doc.Pages[1].Ops {
		if op.Kind == FillOpKind && op.Color == (css.Color{R: 0, G: 255, B: 0, Set: true}) {
			continued = true
		}
	}
	if !continued {
		t.Fatal("the spanning cell has no continuation on the next page")
	}
}
