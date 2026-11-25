package layout

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
)

// forcedBreakPage is a small page whose content area is easy to reason about:
// 100 x 60 pt of content above a 5 mm margin.
func forcedBreakPage() (css.PageSize, [4]int32) {
	return css.PageSize{Width: 283465, Height: 170079}, [4]int32{14173, 14173, 14173, 14173}
}

// runRawOrFail runs a document without the profile validation that run()
// performs, for the fixtures with row-level breaks, and fails on layout errors.
func (h *harness) runRawOrFail(src string, size css.PageSize, margin [4]int32) *Document {
	h.t.Helper()
	doc, diags := h.runRaw(src, size, margin)
	if diags.HasErrors() {
		h.t.Fatalf("layout errors: %v", diags.Lines())
	}
	return doc
}

// TestAdjacentForcedBreaksCollapse guards the double transition: a
// break-after on one paragraph and a break-before on the next met at the same
// boundary, so the eager break-before in placeChild and the pending
// break-after in placeBlock each started a page and a blank page sat between
// them.
func TestAdjacentForcedBreaksCollapse(t *testing.T) {
	h := newHarness(t)
	src := `<style>p { margin: 0 }</style>
	<p style="break-after: page">A</p><p style="break-before: page">B</p>`
	doc := h.run(src)
	if len(doc.Pages) != 2 {
		t.Fatalf("adjacent forced breaks produced %d pages, want 2", len(doc.Pages))
	}
	if !strings.Contains(pageText(doc, 1), "A") || strings.Contains(pageText(doc, 1), "B") {
		t.Fatalf("page 1 = %q, want only A", pageText(doc, 1))
	}
	if !strings.Contains(pageText(doc, 2), "B") {
		t.Fatalf("page 2 = %q, want B", pageText(doc, 2))
	}
}

// TestEmptyBlockDoesNotConsumeForcedBreak guards the second half of the
// pending state: the break was cleared before the next box was known to paint
// anything, so an empty <div> silently cancelled a break-after.
func TestEmptyBlockDoesNotConsumeForcedBreak(t *testing.T) {
	h := newHarness(t)
	for _, box := range []string{`<div></div>`, `<div><div></div></div>`, `<p></p>`} {
		src := `<style>p { margin: 0 }</style>
		<p style="break-after: page">A</p>` + box + `<p>B</p>`
		doc := h.run(src)
		if len(doc.Pages) != 2 {
			t.Fatalf("%s cancelled the break: %d pages, want 2", box, len(doc.Pages))
		}
		if strings.Contains(pageText(doc, 1), "B") || !strings.Contains(pageText(doc, 2), "B") {
			t.Fatalf("B is on the wrong page: %q / %q", pageText(doc, 1), pageText(doc, 2))
		}
	}
}

// TestPaintedEmptyBoxTakesForcedBreak guards the same pending state from the
// other side: a box whose only visible content is a height or a background
// must still be the content that satisfies the break.
func TestPaintedEmptyBoxTakesForcedBreak(t *testing.T) {
	h := newHarness(t)
	src := `<style>p { margin: 0 }</style>
	<p style="break-after: page">A</p>
	<div style="height: 20pt; background-color: #ff0000"></div><p>B</p>`
	size, margin := forcedBreakPage()
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("a height-only block cancelled the break: %d pages, want 2", len(doc.Pages))
	}
	if !strings.Contains(pageText(doc, 2), "B") || strings.Contains(pageText(doc, 1), "B") {
		t.Fatalf("B is on the wrong page: %q / %q", pageText(doc, 1), pageText(doc, 2))
	}
	top := margin[0]
	found := false
	for _, op := range doc.Pages[1].Ops {
		if op.Kind == FillOpKind && op.Color == (css.Color{R: 255, Set: true}) {
			found = true
			if op.Rect.Y != top || op.Rect.H != 20000 {
				t.Fatalf("the background of the moved box is %+v, want Y=%d H=20000", op.Rect, top)
			}
		}
	}
	if !found {
		t.Fatalf("the moved box was not painted on the second page")
	}
}

// TestTrailingBreakBeforeEmptyBox guards a blank trailing page: an empty box
// with break-before eagerly started a page even though nothing followed it.
func TestTrailingBreakBeforeEmptyBox(t *testing.T) {
	h := newHarness(t)
	for _, src := range []string{
		`<p>A</p><div style="break-before: page"></div>`,
		// A container whose descendants paint nothing is empty too.
		`<p>A</p><div style="break-before: page"><div></div><p></p></div>`,
	} {
		doc := h.run(src)
		if len(doc.Pages) != 1 {
			t.Fatalf("%s produced %d pages, want 1", src, len(doc.Pages))
		}
	}
}

// TestPendingBreakSatisfiedByAnotherTransition guards the second page source:
// the pending break-after was consumed after placeChild had already moved the
// following block to the next page for break-inside: avoid, so a blank page
// appeared between them.
func TestPendingBreakSatisfiedByAnotherTransition(t *testing.T) {
	h := newHarness(t)
	size, margin := forcedBreakPage()
	src := `<style>p { margin: 0 }</style>
	<p style="break-after: page">A</p>
	<div style="break-inside: avoid; height: 45mm; background-color: #eeeeee">
	  <p>X</p>
	</div>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("an avoid transition after a forced break produced %d pages, want 2", len(doc.Pages))
	}
	for i, page := range doc.Pages {
		if len(page.Ops) == 0 {
			t.Fatalf("page %d is empty: the pending break was taken twice", i+1)
		}
	}
	if !strings.Contains(pageText(doc, 2), "X") {
		t.Fatalf("the avoided block is not on the second page: %q", pageText(doc, 2))
	}
}

// TestTableRowBreakPairMakesOneTransition guards the row path: break-after and
// break-before on adjacent rows each created a page inside placeTable, and
// repeated headers counted as page content.
func TestTableRowBreakPairMakesOneTransition(t *testing.T) {
	h := newHarness(t)
	size, margin := forcedBreakPage()
	src := `<style>
		table { width: 90mm; border-collapse: collapse }
		td, th { padding: 1mm; border: 0.5pt solid #000 }
	</style>
	<table>
	  <thead><tr><th>H1</th></tr><tr><th>H2</th></tr></thead>
	  <tbody>
	    <tr style="break-after: page"><td>A</td></tr>
	    <tr style="break-before: page"><td>B</td></tr>
	    <tr><td>C</td></tr>
	  </tbody>
	</table>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("a row break pair produced %d pages, want 2", len(doc.Pages))
	}
	if got := pageText(doc, 1); got != "H1H2A" {
		t.Fatalf("page 1 = %q, want H1H2A", got)
	}
	if got := pageText(doc, 2); got != "H1H2BC" {
		t.Fatalf("page 2 = %q, want H1H2BC", got)
	}
}

// TestTableBackgroundFollowsRowBreak guards the table fragment: a page created
// inside placeTable left the table background inserted after the cells, with
// the previous page's coordinates.
func TestTableBackgroundFollowsRowBreak(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>
		p { margin: 0 }
		table { width: 100pt; background-color: #ff0000; border-collapse: collapse }
		td { padding: 0; border: 0; font-size: 10pt; height: 20pt }
	</style>
	<p style="height: 100pt">TOP</p>
	<table>
	  <tr><td>A</td></tr>
	  <tr style="break-after: page"><td>B</td></tr>
	  <tr><td>C</td></tr>
	</table>`
	doc := h.runRawOrFail(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("the row break produced %d pages, want 2", len(doc.Pages))
	}
	// The table starts below the filler on page 1 and at the content top on
	// page 2: each fragment paints its background with its own page origin.
	wantY := []int32{110000, 10000}
	for i := 1; i <= 2; i++ {
		page := doc.Pages[i-1]
		bg, bgAt, textAt := -1, -1, -1
		for at, op := range page.Ops {
			if op.Kind == FillOpKind && op.Color == (css.Color{R: 255, Set: true}) {
				bg, bgAt = at, at
				if op.Rect.Y != wantY[i-1] || op.Rect.X != 10000 || op.Rect.W != 100000 {
					t.Fatalf("page %d table background rect = %+v, want X=10000 Y=%d W=100000", i, op.Rect, wantY[i-1])
				}
			}
			if op.Kind == TextOpKind && op.Text != nil {
				textAt = at
			}
		}
		if bg < 0 {
			t.Fatalf("page %d has no table background", i)
		}
		if textAt >= 0 && bgAt > textAt {
			t.Fatalf("page %d paints the table background at %d, after the text at %d", i, bgAt, textAt)
		}
	}
}

// TestBodyBackgroundSurvivesRowBreak guards the ancestor fragment: <body> must
// paint its background on both pages, each with its own coordinates and before
// the content.
func TestBodyBackgroundSurvivesRowBreak(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>
		body { background-color: #00ff00 }
		p { margin: 0 }
		table { width: 100pt; border-collapse: collapse }
		td { padding: 0; border: 0; font-size: 10pt }
	</style>
	<p style="height: 100pt">TOP</p>
	<table>
	  <tr><td>A</td></tr>
	  <tr style="break-after: page"><td>B</td></tr>
	  <tr><td>C</td></tr>
	</table>`
	doc := h.runRawOrFail(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("the row break produced %d pages, want 2", len(doc.Pages))
	}
	for i := 1; i <= 2; i++ {
		page := doc.Pages[i-1]
		bgAt, textAt := -1, -1
		for at, op := range page.Ops {
			if op.Kind == FillOpKind && op.Color == (css.Color{G: 255, Set: true}) {
				if op.Rect.X != 10000 || op.Rect.Y != 10000 || op.Rect.W != 180000 {
					t.Fatalf("page %d body background rect = %+v, want X=10000 Y=10000 W=180000", i, op.Rect)
				}
				if bgAt < 0 {
					bgAt = at
				}
			}
			if op.Kind == TextOpKind && op.Text != nil {
				textAt = at
			}
		}
		if bgAt < 0 {
			t.Fatalf("page %d does not paint the body background", i)
		}
		if textAt >= 0 && bgAt > textAt {
			t.Fatalf("page %d paints the body background after the content", i)
		}
	}
}

// TestTableAvoidUsesBuiltGrid guards the measurement: measureTable computed
// columns before buildGrid assigned cell.col, so a two-column table with
// break-inside: avoid measured shorter than it is and split across pages.
func TestTableAvoidUsesBuiltGrid(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>
		p { margin: 0 }
		table { width: 100pt; break-inside: avoid; border-collapse: collapse }
		td { padding: 0; border: 0; font-size: 10pt; line-height: 12pt }
	</style>
	<p style="height: 140pt">F</p>
	<table>
	  <tr><td>AAAA BBBB CCCC</td><td>DDDD EEEE FFFF</td></tr>
	  <tr><td>GGGG HHHH IIII</td><td>JJJJ KKKK LLLL</td></tr>
	</table>`
	doc := h.runRawOrFail(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("the avoided table produced %d pages, want 2", len(doc.Pages))
	}
	if text := pageText(doc, 1); strings.Contains(text, "GGGG") || strings.Contains(text, "AAAA") {
		t.Fatalf("the table was split across pages: page 1 = %q", text)
	}
	if text := pageText(doc, 2); !strings.Contains(text, "AAAA") || !strings.Contains(text, "GGGG") {
		t.Fatalf("the whole table is not on the second page: %q", text)
	}
}

// TestIndentAndLeadingImageSetNaturalWidth guards Line.Natural: the indent was
// applied when the first word arrived, discarding the width of a preceding
// inline image from the line width, so the right-aligned line overflowed the
// content edge by the image width.
func TestIndentAndLeadingImageSetNaturalWidth(t *testing.T) {
	h := newHarness(t)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	f, err := os.Create(filepath.Join(h.images.dir, "i.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{20000, 20000, 20000, 20000}
	src := `<p style="width: 160pt; text-align: right; text-indent: 20pt; margin: 0; font-size: 10pt">
	<img src="i.png" style="width: 20pt; height: 20pt">WORD</p>`
	doc := h.runRawOrFail(src, size, margin)
	var imgRect *Rect
	var word *TextOp
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == ImageOpKind {
			r := op.Rect
			imgRect = &r
		}
		if op.Kind == TextOpKind && op.Text != nil && pageTextRun(op.Text) == "WORD" {
			word = op.Text
		}
	}
	if imgRect == nil || word == nil {
		t.Fatalf("the image or the word was not painted: image=%v word=%v", imgRect, word)
	}
	if word.X != imgRect.X+imgRect.W {
		t.Fatalf("the word starts at %d, the image ends at %d: the indent discarded the image width", word.X, imgRect.X+imgRect.W)
	}
	contentRight := int32(20000 + 160000)
	if right := word.X + word.Run.Width; right != contentRight {
		t.Fatalf("the line ends at %d, want the content edge %d", right, contentRight)
	}
}

// TestOneOverflowDiagnosticPerBox guards the duplicated applyFixedHeight calls:
// a box that only declares a height reached the text-block path twice, and a
// table with the same height reported the warning twice as well.
func TestOneOverflowDiagnosticPerBox(t *testing.T) {
	h := newHarness(t)
	size, margin := forcedBreakPage()
	cases := []string{
		`<div style="height: 200mm"></div>`,
		`<div style="height: 200mm; background-color: #eeeeee">x</div>`,
		`<table style="height: 200mm"><tr><td>x</td></tr></table>`,
	}
	for _, src := range cases {
		doc, _ := h.runRaw(src, size, margin)
		n := 0
		for _, d := range doc.Diags {
			if d.Code == "layout/overflow" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%s reported layout/overflow %d times, want 1: %v", src, n, doc.Diags.Lines())
		}
	}
}

// TestForcedBreakDocumentsHaveNoEmptyPages scans every forced-break shape this
// change fixes: a page with no drawing operations means a break created a page
// nothing was placed on.
func TestForcedBreakDocumentsHaveNoEmptyPages(t *testing.T) {
	size, margin := forcedBreakPage()
	cases := []struct{ name, src string }{
		{"adjacent pair", `<p style="break-after:page">A</p><p style="break-before:page">B</p>`},
		{"empty block", `<p style="break-after:page">A</p><div></div><p>B</p>`},
		{"height only", `<p style="break-after:page">A</p><div style="height:20pt;background-color:#eee"></div><p>B</p>`},
		{"trailing break", `<p>A</p><div style="break-before:page"></div>`},
		{"trailing nested empty", `<p>A</p><div style="break-before:page"><div></div></div>`},
		{"first content", `<p style="break-before:page">A</p>`},
		{"avoid after break", `<p style="break-after:page">A</p><div style="break-inside:avoid;height:45mm"><p>X</p></div>`},
		{"rows", `<table><thead><tr><th>H</th></tr></thead><tbody>
			<tr style="break-after:page"><td>A</td></tr>
			<tr style="break-before:page"><td>B</td></tr>
			<tr><td>C</td></tr></tbody></table>`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			doc := h.runWithSize(tc.src, size, margin)
			for i, page := range doc.Pages {
				if len(page.Ops) == 0 {
					t.Fatalf("page %d is empty in a forced-break document", i+1)
				}
			}
		})
	}
}
