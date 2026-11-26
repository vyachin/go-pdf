package layout

import (
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
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

// TestHugeDeclaredTableColumnsStayInsideContentBox guards RV5-16: the declared
// widths were summed in int32, so two 2,000,000 pt columns wrapped the total and
// the scale-to-fit pass saturated it. Each produced column stayed about 449 pt
// wide, the second cell began at roughly 505 pt and its box painted past the
// 595 pt page while the table still claimed its columns were scaled to fit.
func TestHugeDeclaredTableColumnsStayInsideContentBox(t *testing.T) {
	h := newHarness(t)
	size, margin := h.ctx.PageSize, h.ctx.Margin
	contentLeft := margin[3]
	contentRight := size.Width - margin[1]
	src := `<table><tr>
	  <td style="width:2000000pt;background-color:#eeeeee">A</td>
	  <td style="width:2000000pt;background-color:#dddddd">B</td>
	</tr></table>`
	doc, diags := h.runRaw(src, size, margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	colors := []css.Color{
		{R: 0xee, G: 0xee, B: 0xee, Set: true},
		{R: 0xdd, G: 0xdd, B: 0xdd, Set: true},
	}
	cells := make([]*Rect, len(colors))
	for _, page := range doc.Pages {
		for i := range page.Ops {
			op := &page.Ops[i]
			if op.Kind != FillOpKind {
				continue
			}
			for k, color := range colors {
				if op.Color == color {
					cells[k] = &op.Rect
				}
			}
		}
	}
	for k, rect := range cells {
		if rect == nil {
			t.Fatalf("cell %d has no painted background", k+1)
		}
		if rect.W <= 0 {
			t.Fatalf("cell %d has width %d, want a positive scaled width", k+1, rect.W)
		}
		if rect.X < contentLeft {
			t.Fatalf("cell %d starts at %d, before the content box %d", k+1, rect.X, contentLeft)
		}
		if right := int64(rect.X) + int64(rect.W); right > int64(contentRight) {
			t.Fatalf("cell %d ends at %d, past the content box right edge %d", k+1, right, contentRight)
		}
	}
	// No operation is painted outside the page either.
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Rect.X < 0 || int64(op.Rect.X)+int64(op.Rect.W) > int64(page.Size.Width) {
				t.Fatalf("an operation is outside the page: x=%d w=%d", op.Rect.X, op.Rect.W)
			}
		}
	}
	// The one warning describes the geometry that was produced: the columns
	// were scaled, so the table fits the content box.
	reported := 0
	for _, d := range diags {
		if d.Code != "layout/overflow" {
			continue
		}
		reported++
		if !strings.Contains(d.Message, "scaled to fit") {
			t.Fatalf("the overflow warning does not describe the scaled columns: %s", d.String())
		}
	}
	if reported != 1 {
		t.Fatalf("the scaled table reported layout/overflow %d times, want 1: %v", reported, diags.Lines())
	}
	// A table whose declared columns fit keeps its geometry and reports nothing.
	fitSrc := `<table><tr>
	  <td style="width:100pt;background-color:#eeeeee">A</td>
	  <td style="background-color:#dddddd">B</td>
	</tr></table>`
	fitDoc, fitDiags := h.runRaw(fitSrc, size, margin)
	if n := countOverflow(fitDiags); n != 0 {
		t.Fatalf("a fitting table reported %d layout/overflow diagnostics: %v", n, fitDiags.Lines())
	}
	fitCells := make([]*Rect, len(colors))
	for _, page := range fitDoc.Pages {
		for i := range page.Ops {
			op := &page.Ops[i]
			if op.Kind != FillOpKind {
				continue
			}
			for k, color := range colors {
				if op.Color == color {
					fitCells[k] = &op.Rect
				}
			}
		}
	}
	if fitCells[0] == nil || fitCells[0].W != 100000 {
		t.Fatalf("the declared 100 pt column was resized: %+v", fitCells[0])
	}
	if fitCells[1] == nil || fitCells[1].X != contentLeft+100000 {
		t.Fatalf("the fitting columns moved: %+v", fitCells[1])
	}
	if right := int64(fitCells[1].X) + int64(fitCells[1].W); right != int64(contentRight) {
		t.Fatalf("the fitting table ends at %d, want the content edge %d", right, contentRight)
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

// TestLongWordInTableCellIsReported guards RV5-17: only placeText compared a
// line with its width, so a thousand-character word inside a table cell was
// painted past the page edge with no diagnostic, while the same word in a
// paragraph was reported. The cell reports once, and a word that fits reports
// nothing.
func TestLongWordInTableCellIsReported(t *testing.T) {
	h := newHarness(t)
	_, diags := h.runRaw(`<table><tr><td>`+repeatRune('ж', 1000)+`</td></tr></table>`,
		h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if n := countOverflow(diags); n != 1 {
		t.Fatalf("the cell reported %d layout/overflow diagnostics, want 1: %v", n, diags.Lines())
	}
	for _, d := range diags {
		if d.Code == "layout/overflow" && !strings.Contains(d.Element, "td") {
			t.Fatalf("the diagnostic does not name the cell: %s", d.String())
		}
	}
	// A cell whose words fit its content width does not warn.
	_, fitDiags := h.runRaw(`<table><tr><td>короткое слово</td></tr></table>`,
		h.ctx.PageSize, h.ctx.Margin)
	if n := countOverflow(fitDiags); n != 0 {
		t.Fatalf("a fitting cell reported %d layout/overflow diagnostics: %v", n, fitDiags.Lines())
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

// TestRowspanCellIsClippedAndContinued guards RV4-13 and the spanning-cell
// continuation: a rowspan cell crossing a page break used to be painted once
// with the height of all covered rows, and only its background was continued.
// Every fragment now carries the cell's text, background and four borders.
func TestRowspanCellIsClippedAndContinued(t *testing.T) {
	const lines = 40
	h := newHarness(t)
	var body strings.Builder
	body.WriteString(`<table><tr><td rowspan="60" style="background-color:#00ff00;border:2pt solid #ff0000">`)
	for i := 0; i < lines; i++ {
		if i > 0 {
			body.WriteString("<br>")
		}
		body.WriteString("L")
		body.WriteString(strconv.Itoa(i))
	}
	body.WriteString(`</td><td>r1</td></tr>`)
	for i := 2; i <= 60; i++ {
		body.WriteString("<tr><td>r")
		body.WriteString(strconv.Itoa(i))
		body.WriteString("</td></tr>")
	}
	body.WriteString("</table>")
	size := css.PageSize{Width: 283465, Height: 170079}
	margin := [4]int32{14173, 14173, 14173, 14173}
	doc, diags := h.runRaw(body.String(), size, margin)
	if len(doc.Pages) < 3 {
		t.Fatalf("the table produced %d pages, want at least 3", len(doc.Pages))
	}
	if n := countOverflow(diags); n != 0 {
		t.Fatalf("the continued cell reported %d layout/overflow diagnostics: %v", n, diags.Lines())
	}
	// Every line is extracted once, on the page whose fragment carries it.
	perPage := assertCellLines(t, doc, "L", lines)
	for _, page := range []int{0, 1, 2} {
		if len(perPage[page]) == 0 {
			t.Fatalf("page %d carries no continued line: per page %v", page+1, perPage)
		}
	}
	// Every page the span covers is a closed box: the background plus all four
	// border sides, clipped to the page content box.
	green := css.Color{R: 0, G: 255, B: 0, Set: true}
	red := css.Color{R: 255, Set: true}
	for _, page := range doc.Pages {
		assertSpanFragment(t, page, green, red, 2000)
	}
	assertInsideContentBox(t, doc)
}

// TestTwoSpanningCellsInOneRowBothContinue guards the single-continuation-slot
// defect: when two cells span the same rows, the later record used to replace
// the earlier one. Both cells must keep their own text, background and border
// on the following pages.
func TestTwoSpanningCellsInOneRowBothContinue(t *testing.T) {
	const lines = 30
	h := newHarness(t)
	var body strings.Builder
	body.WriteString(`<table><tr>`)
	for _, cell := range []struct{ prefix, background string }{
		{"A", "#00ff00"},
		{"B", "#0000ff"},
	} {
		body.WriteString(`<td rowspan="30" style="background-color:` + cell.background + `;border:1pt solid #ff0000">`)
		for i := 0; i < lines; i++ {
			if i > 0 {
				body.WriteString("<br>")
			}
			body.WriteString(cell.prefix)
			body.WriteString(strconv.Itoa(i))
		}
		body.WriteString(`</td>`)
	}
	body.WriteString(`<td>r1</td></tr>`)
	for i := 2; i <= 30; i++ {
		body.WriteString("<tr><td>r")
		body.WriteString(strconv.Itoa(i))
		body.WriteString("</td></tr>")
	}
	body.WriteString("</table>")
	size, margin := smallPage()
	doc, diags := h.runRaw(body.String(), size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("the table produced %d pages, want at least 2", len(doc.Pages))
	}
	if n := countOverflow(diags); n != 0 {
		t.Fatalf("both spanning cells should continue without diagnostics, got %v", diags.Lines())
	}
	// Each cell keeps its own line cursor, so both streams survive the break.
	a := assertCellLines(t, doc, "A", lines)
	b := assertCellLines(t, doc, "B", lines)
	if len(a[1]) == 0 {
		t.Fatalf("cell A paints no continued line on page 2: per page %v", a)
	}
	if len(b[1]) == 0 {
		t.Fatalf("cell B paints no continued line on page 2: per page %v", b)
	}
	// The second record must not overwrite the first: both backgrounds and both
	// closed fragments are present on page 2.
	green := css.Color{R: 0, G: 255, B: 0, Set: true}
	blue := css.Color{B: 255, Set: true}
	red := css.Color{R: 255, Set: true}
	assertSpanFragment(t, doc.Pages[1], green, red, 1000)
	assertSpanFragment(t, doc.Pages[1], blue, red, 1000)
	assertInsideContentBox(t, doc)
}

// TestSpanEndsInsideAPage guards a span whose covered rows end in the middle of
// a page: the last fragment stops at the last covered row instead of running to
// the page bottom and over the rows below the span.
func TestSpanEndsInsideAPage(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	src := `<table>
	  <tr style="height:80pt"><td rowspan="2" style="background-color:#00ff00;border:2pt solid #ff0000">S</td><td>r1</td></tr>
	  <tr style="height:80pt"><td>r2</td></tr>
	  <tr><td>AFTER</td></tr>
	</table>`
	doc, diags := h.runRaw(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("the table produced %d pages, want 2", len(doc.Pages))
	}
	if n := countOverflow(diags); n != 0 {
		t.Fatalf("unexpected overflow diagnostics: %v", diags.Lines())
	}
	green := css.Color{R: 0, G: 255, B: 0, Set: true}
	red := css.Color{R: 255, Set: true}
	fill := assertSpanFragment(t, doc.Pages[1], green, red, 2000)
	// Only the second covered row (80 pt) remains on page 2, so the fragment
	// stops there rather than at the page content bottom.
	if fill.H != 80000 {
		t.Fatalf("the last fragment is %d tall, want the 80000 of the last covered row", fill.H)
	}
	after := firstBaseline(t, doc.Pages[1], "AFTER")
	if after < 0 {
		t.Fatal("the row below the span is not painted on page 2")
	}
	if after < fill.Y+fill.H {
		t.Fatalf("the row below the span (baseline %d) is painted over the fragment ending at %d", after, fill.Y+fill.H)
	}
	assertInsideContentBox(t, doc)
}

// TestSpanningCellWithoutLineCursorReportsOnce guards the no-silent-loss rule
// for content this change does not fragment (a nested table): the loss is
// reported once for the cell, not once per page, and the rest of the document
// is still rendered.
func TestSpanningCellWithoutLineCursorReportsOnce(t *testing.T) {
	h := newHarness(t)
	var body strings.Builder
	body.WriteString(`<table><tr><td rowspan="60"><table>`)
	for i := 0; i < 30; i++ {
		body.WriteString("<tr><td>N</td></tr>")
	}
	body.WriteString(`</table></td><td>r1</td></tr>`)
	for i := 2; i <= 60; i++ {
		body.WriteString("<tr><td>r")
		body.WriteString(strconv.Itoa(i))
		body.WriteString("</td></tr>")
	}
	body.WriteString("</table><p>AFTER</p>")
	size, margin := smallPage()
	doc, diags := h.runRaw(body.String(), size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("the table produced %d pages, want at least 2", len(doc.Pages))
	}
	reported := 0
	for _, d := range diags {
		if d.Code != "layout/overflow" {
			continue
		}
		reported++
		if !strings.Contains(d.Element, "td") {
			t.Fatalf("the diagnostic does not name the cell: %s", d.String())
		}
	}
	if reported != 1 {
		t.Fatalf("got %d layout/overflow diagnostics, want exactly one: %v", reported, diags.Lines())
	}
	if !strings.Contains(textOf(doc), "AFTER") {
		t.Fatal("the paragraph after the table is not rendered")
	}
}

// TestRowspanHundredLinesContinueAcrossPages is the delta-spec scenario: a
// first-row cell with rowspan="100" and one hundred lines on A4 spans three
// pages, every line is extractable exactly once, and every page carries the
// cell's background and four border sides.
func TestRowspanHundredLinesContinueAcrossPages(t *testing.T) {
	const lines = 100
	h := newHarness(t)
	var body strings.Builder
	body.WriteString(`<table><tr><td rowspan="100" style="background-color:#00ff00;border:2pt solid #ff0000">`)
	for i := 0; i < lines; i++ {
		if i > 0 {
			body.WriteString("<br>")
		}
		body.WriteString("L")
		body.WriteString(strconv.Itoa(i))
	}
	body.WriteString(`</td><td>r1</td></tr>`)
	for i := 2; i <= 100; i++ {
		body.WriteString("<tr><td>r")
		body.WriteString(strconv.Itoa(i))
		body.WriteString("</td></tr>")
	}
	body.WriteString("</table>")
	doc, diags := h.runRaw(body.String(), h.ctx.PageSize, h.ctx.Margin)
	if len(doc.Pages) != 3 {
		t.Fatalf("the table produced %d pages, want 3", len(doc.Pages))
	}
	if n := countOverflow(diags); n != 0 {
		t.Fatalf("the continued cell reported %d layout/overflow diagnostics: %v", n, diags.Lines())
	}
	perPage := assertCellLines(t, doc, "L", lines)
	if len(perPage[0]) == 0 || len(perPage[1]) == 0 {
		t.Fatalf("a page that continues the cell carries no cell line: per page %v", perPage)
	}
	if len(perPage[2]) != 0 {
		t.Fatalf("the cell content ended before page 3: per page %v", perPage)
	}
	green := css.Color{R: 0, G: 255, B: 0, Set: true}
	red := css.Color{R: 255, Set: true}
	for _, page := range doc.Pages {
		assertSpanFragment(t, page, green, red, 2000)
	}
	assertInsideContentBox(t, doc)
}

// assertSpanFragment checks that a spanning cell's fragment is a closed box:
// its background and all four border sides are painted at the fragment's edges.
func assertSpanFragment(t *testing.T, page *Page, background, border css.Color, borderWidth int32) Rect {
	t.Helper()
	fill := spanFill(page, background)
	if fill == nil {
		t.Fatalf("page %d has no spanning-cell background", page.Number)
	}
	sides := []Rect{
		{X: fill.X, Y: fill.Y, W: borderWidth, H: fill.H},
		{X: fill.X + fill.W - borderWidth, Y: fill.Y, W: borderWidth, H: fill.H},
		{X: fill.X, Y: fill.Y, W: fill.W, H: borderWidth},
		{X: fill.X, Y: fill.Y + fill.H - borderWidth, W: fill.W, H: borderWidth},
	}
	for _, side := range sides {
		if !hasFill(page, border, side) {
			t.Fatalf("page %d: border side %+v is missing from the fragment %+v", page.Number, side, *fill)
		}
	}
	return *fill
}

// spanFill returns the first filled rectangle of the given colour on the page.
func spanFill(page *Page, color css.Color) *Rect {
	for i := range page.Ops {
		op := &page.Ops[i]
		if op.Kind == FillOpKind && op.Color == color {
			return &op.Rect
		}
	}
	return nil
}

// hasFill reports whether the page paints the exact rectangle in the colour.
func hasFill(page *Page, color css.Color, rect Rect) bool {
	for _, op := range page.Ops {
		if op.Kind == FillOpKind && op.Color == color && op.Rect == rect {
			return true
		}
	}
	return false
}

// assertInsideContentBox checks that no operation extends below the page
// content bottom.
func assertInsideContentBox(t *testing.T, doc *Document) {
	t.Helper()
	for _, page := range doc.Pages {
		bottom := satSub(page.Size.Height, page.Margin[2])
		for _, op := range page.Ops {
			if op.Rect.Y < 0 || op.Rect.Y+op.Rect.H > bottom+1 {
				t.Fatalf("page %d: an operation paints outside the content box: y=%d h=%d bottom=%d",
					page.Number, op.Rect.Y, op.Rect.H, bottom)
			}
		}
	}
}

// countOverflow returns the number of layout/overflow diagnostics.
func countOverflow(diags diag.List) int {
	n := 0
	for _, d := range diags {
		if d.Code == "layout/overflow" {
			n++
		}
	}
	return n
}

// cellLinesByPage returns, per page, the text of every painted line of a cell
// whose lines start with prefix. Each line is one text operation.
func cellLinesByPage(doc *Document, prefix string) [][]string {
	out := make([][]string, len(doc.Pages))
	for i, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			var b strings.Builder
			for _, g := range op.Text.Run.Glyphs {
				b.WriteRune(g.Rune)
			}
			if text := b.String(); strings.HasPrefix(text, prefix) {
				out[i] = append(out[i], text)
			}
		}
	}
	return out
}

// assertCellLines checks that every cell line is painted exactly once and
// returns the per-page counts.
func assertCellLines(t *testing.T, doc *Document, prefix string, want int) [][]string {
	t.Helper()
	perPage := cellLinesByPage(doc, prefix)
	seen := map[string]bool{}
	total := 0
	for _, lines := range perPage {
		for _, line := range lines {
			if seen[line] {
				t.Fatalf("cell line %q is painted twice: per page %v", line, perPage)
			}
			seen[line] = true
			total++
		}
	}
	if total != want {
		t.Fatalf("%q cell lines: got %d, want %d: per page %v", prefix, total, want, perPage)
	}
	return perPage
}

// firstBaseline returns the baseline of the first painted line whose text is
// the given word, or -1.
func firstBaseline(t *testing.T, page *Page, word string) int32 {
	t.Helper()
	for _, op := range page.Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		var b strings.Builder
		for _, g := range op.Text.Run.Glyphs {
			b.WriteRune(g.Rune)
		}
		if b.String() == word {
			return op.Text.Y
		}
	}
	return -1
}

// TestMarginBoxWithHugeMarginStaysOnPage guards RV5-44: the margin box content
// width and positions were plain int32 arithmetic, so a horizontal margin larger
// than the page moved the footer to a wrapped coordinate with no diagnostic.
func TestMarginBoxWithHugeMarginStaysOnPage(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	cases := []struct{ name, value string }{
		{"huge horizontal margin", "10pt 2000000pt"},
		{"no content width", "0 300pt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `<style>
				@page { size: 200pt 200pt; margin: ` + tc.value + ` }
				@page { @bottom-center { content: "FOOT" } }
			</style><p></p>`
			doc, diags := h.runRaw(src, size, margin)
			if len(doc.Pages) == 0 {
				t.Fatal("no pages produced")
			}
			painted := false
			for _, page := range doc.Pages {
				for _, op := range page.Ops {
					if op.Kind != TextOpKind || op.Text == nil {
						continue
					}
					if op.Text.X < 0 || op.Text.X+op.Text.Run.Width > page.Size.Width {
						t.Fatalf("page %d paints a run at x=%d w=%d outside the page",
							page.Number, op.Text.X, op.Text.Run.Width)
					}
					if op.Text.Y < 0 || op.Text.Y > page.Size.Height {
						t.Fatalf("page %d paints a run at y=%d outside the page", page.Number, op.Text.Y)
					}
					painted = true
				}
			}
			if !painted {
				t.Fatal("the footer was dropped instead of clamped")
			}
			named := 0
			for _, d := range diags {
				if d.Code == "layout/overflow" && strings.Contains(d.String(), "bottom-center") {
					named++
				}
			}
			if named != 1 {
				t.Fatalf("the unplaceable margin box reported %d named diagnostics, want 1: %v", named, diags.Lines())
			}
		})
	}
}

// TestOrdinaryMarginBoxesAreUnchanged guards the ordinary case of RV5-44: the
// six margin boxes keep the origins the plain arithmetic produced and report
// nothing.
func TestOrdinaryMarginBoxesAreUnchanged(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 20000, 30000, 40000}
	src := `<style>
		@page { size: 200pt 200pt; margin: 10pt 20pt 30pt 40pt }
		@page {
			@top-left { content: "TL" } @top-center { content: "TC" } @top-right { content: "TR" }
			@bottom-left { content: "BL" } @bottom-center { content: "BC" } @bottom-right { content: "BR" }
		}
		p { margin: 0 }
	</style><p>BODY</p>`
	doc, diags := h.runRaw(src, size, margin)
	if len(diags.Lines()) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags.Lines())
	}
	byLabel := map[string]*TextOp{}
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		var b strings.Builder
		for _, g := range op.Text.Run.Glyphs {
			b.WriteRune(g.Rune)
		}
		byLabel[b.String()] = op.Text
	}
	for _, label := range []string{"TL", "TC", "TR", "BL", "BC", "BR"} {
		if byLabel[label] == nil {
			t.Fatalf("margin box %s was not painted", label)
		}
	}
	contentLeft := int32(40000)
	contentWidth := size.Width - margin[1] - margin[3]
	ascent := byLabel["TL"].Face.Ascent
	topY := margin[0]/2 + ascent/2
	bottomY := size.Height - margin[2]/2 + ascent/2
	boxes := []struct{ name, label string }{
		{"top-left", "TL"}, {"top-center", "TC"}, {"top-right", "TR"},
		{"bottom-left", "BL"}, {"bottom-center", "BC"}, {"bottom-right", "BR"},
	}
	for _, box := range boxes {
		op := byLabel[box.label]
		var wantX int32
		switch {
		case strings.HasSuffix(box.name, "left"):
			wantX = contentLeft
		case strings.HasSuffix(box.name, "center"):
			wantX = contentLeft + (contentWidth-op.Run.Width)/2
		default:
			wantX = contentLeft + contentWidth - op.Run.Width
		}
		wantY := topY
		if strings.HasPrefix(box.name, "bottom") {
			wantY = bottomY
		}
		if op.X != wantX || op.Y != wantY {
			t.Fatalf("margin box %s is at (%d, %d), want (%d, %d)", box.name, op.X, op.Y, wantX, wantY)
		}
	}
}
