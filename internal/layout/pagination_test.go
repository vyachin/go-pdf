package layout

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/stylesheet"
)

func pageText(doc *Document, number int) string {
	if number < 1 || number > len(doc.Pages) {
		return ""
	}
	var out []rune
	for _, op := range doc.Pages[number-1].Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		for _, g := range op.Text.Run.Glyphs {
			out = append(out, g.Rune)
		}
	}
	return string(out)
}

func smallPage() (css.PageSize, [4]int32) {
	return css.PageSize{Width: 283465, Height: 170079}, [4]int32{14173, 14173, 14173, 14173}
}

func TestTableHeaderRepeatsOnEveryPage(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	var rows strings.Builder
	for i := 0; i < 14; i++ {
		rows.WriteString("<tr><td>строка</td><td>значение</td></tr>")
	}
	src := `<style>
		table { width: 90mm; border-collapse: collapse }
		td, th { padding: 1mm; border: 0.5pt solid #000 }
	</style>
	<table><thead><tr><th>Заголовок</th><th>Колонка</th></tr></thead>
	<tbody>` + rows.String() + `</tbody></table>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(doc.Pages))
	}
	for i := 1; i <= len(doc.Pages); i++ {
		if !strings.HasPrefix(pageText(doc, i), "Заголовок") {
			t.Fatalf("page %d does not start with the repeated header: %q", i, pageText(doc, i))
		}
	}
}

func TestTableRowBreakInsideAvoid(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	src := `<style>
		table { width: 90mm }
		td, th { padding: 1mm; border: 0.5pt solid #000 }
		.big { break-inside: avoid }
		p { margin: 0 }
	</style>
	<p style="height: 45mm">заполнитель</p>
	<table><tbody>
	<tr class="big"><td>высокая строка</td></tr>
	</tbody></table>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("got %d pages, want the row to move to a second page", len(doc.Pages))
	}
	if !strings.Contains(pageText(doc, 2), "высокая") {
		t.Fatalf("the row is not on the second page: %q", pageText(doc, 2))
	}
}

func TestColspanWidensCell(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>
		table { width: 100mm }
		td { border: 0.5pt solid #000; padding: 1mm }
	</style>
	<table>
	  <tr><td colspan="2">объединённая</td></tr>
	  <tr><td>a</td><td>b</td></tr>
	</table>`)
	widest := int32(0)
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == FillOpKind && op.Rect.W > widest {
			widest = op.Rect.W
		}
	}
	if widest == 0 {
		t.Fatal("no cell borders were painted")
	}
	if widest < 250000 {
		t.Fatalf("spanning cell width = %d, want close to the table width", widest)
	}
}

func TestMarginBoxesWithCounters(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	src := `<style>
		@page { size: 100mm 60mm; margin: 12mm 5mm }
		@page { @bottom-center { content: "Стр. " counter(page) " из " counter(pages) } }
		p { margin: 0 0 3mm 0 }
	</style>
	<p>первый</p><p>второй</p><p>третий</p><p>четвёртый</p><p>пятый</p>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(doc.Pages))
	}
	total := len(doc.Pages)
	for i := 1; i <= total; i++ {
		want := "из " + itoa(total)
		text := pageText(doc, i)
		if !strings.Contains(text, "Стр. ") || !strings.Contains(text, want) {
			t.Fatalf("page %d text %q does not contain the counter footer", i, text)
		}
	}
}

func TestOrphansAndWidows(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	var words strings.Builder
	for i := 0; i < 60; i++ {
		words.WriteString("слово ")
	}
	src := `<style>
		@page { size: 60mm 45mm; margin: 5mm }
		p { margin: 0; orphans: 3; widows: 2 }
	</style><p>` + words.String() + `</p>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(doc.Pages))
	}
}

func TestOverflowWarnsAndTerminates(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	doc := h.runWithSize(`<style>
		@page { size: 60mm 40mm; margin: 5mm }
		div { height: 200mm; background-color: #eee }
	</style><div>x</div>`, size, margin)
	if len(doc.Pages) == 0 {
		t.Fatal("no pages produced")
	}
	if !hasWarning(doc, "layout/overflow") {
		t.Fatalf("expected an overflow warning, got %v", doc.Diags.Lines())
	}
}

func hasWarning(doc *Document, code string) bool {
	for _, d := range doc.Diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestRowspanCoversSeveralRows(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>
		table { width: 80mm; border-collapse: collapse }
		td { border: 0.5pt solid #000; padding: 2mm }
	</style>
	<table>
	  <tr><td rowspan="2">объединено</td><td>a</td></tr>
	  <tr><td>b</td></tr>
	</table>`)
	var tallest int32
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == FillOpKind && op.Rect.W > 100 && op.Rect.H > tallest {
			tallest = op.Rect.H
		}
	}
	if tallest < 10000 {
		t.Fatalf("the tallest painted box is %d, the rowspan cell should cover two rows", tallest)
	}
}

func TestCollapsedBordersArePaintedOnce(t *testing.T) {
	h := newHarness(t)
	count := func(collapse string) int {
		doc := h.run(`<style>
			table { width: 60mm; border-collapse: ` + collapse + ` }
			td { border: 0.5pt solid #000; padding: 1mm }
		</style>
		<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`)
		n := 0
		for _, op := range doc.Pages[0].Ops {
			if op.Kind == FillOpKind && (op.Rect.W == 500 || op.Rect.H == 500) {
				n++
			}
		}
		return n
	}
	collapsed := count("collapse")
	separate := count("separate")
	if collapsed >= separate {
		t.Fatalf("collapsed borders %d should be fewer than separate borders %d", collapsed, separate)
	}
}

func TestJustifiedLineReachesContentWidth(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>
		p { text-align: justify; font-size: 12pt; margin: 0 }
	</style><p>Раз два три четыре пять шесть семь восемь девять десять одиннадцать двенадцать тринадцать четырнадцать пятнадцать</p>`)
	firstY := int32(-1)
	var right int32
	var lineOps int
	for _, op := range textOps(doc) {
		if firstY < 0 {
			firstY = op.Y
		}
		if op.Y != firstY {
			break
		}
		lineOps++
		if end := op.X + op.Run.Width; end > right {
			right = end
		}
	}
	if lineOps < 2 {
		t.Skip("the paragraph fit on one line, nothing to justify")
	}
	contentRight := int32(595276 - 56693)
	if diff := contentRight - right; diff > 3000 || diff < -3000 {
		t.Fatalf("justified line ends at %d, want about %d", right, contentRight)
	}
}

// lineExtents returns the right edge of every line a page paints, in painting
// order. All runs of one line share a baseline, so the baseline identifies the
// line even when justification split it into several runs.
func lineExtents(page *Page) []int32 {
	var order []int32
	right := map[int32]int32{}
	for _, op := range page.Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		y := op.Text.Y
		if _, ok := right[y]; !ok {
			order = append(order, y)
		}
		if end := op.Text.X + op.Text.Run.Width; end > right[y] {
			right[y] = end
		}
	}
	out := make([]int32, len(order))
	for i, y := range order {
		out[i] = right[y]
	}
	return out
}

// TestJustifiedPageEndingLinesAreNotStretched guards RV5-42: placeText passed
// "last" only for the paragraph's final line, so the closing line of every page
// was stretched to the content edge. Only the paragraph's own last line and a
// line ended by a forced break may stay unstretched.
func TestJustifiedPageEndingLinesAreNotStretched(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 90000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>
		@page { size: 200pt 90pt; margin: 10pt }
		p { margin: 0; text-align: justify; font-size: 10pt }
	</style><p>` + strings.Repeat("word ", 80) + `</p>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) != 3 {
		t.Fatalf("the fixture spilled over %d pages, want 3", len(doc.Pages))
	}
	contentRight := size.Width - margin[3]
	const slack = int32(3000)
	for _, page := range doc.Pages {
		extents := lineExtents(page)
		if len(extents) < 2 {
			t.Fatalf("page %d has %d lines, want at least 2", page.Number, len(extents))
		}
		last := extents[len(extents)-1]
		if last > contentRight-slack {
			t.Fatalf("page %d closes at %d, want narrower than %d", page.Number, last, contentRight-slack)
		}
		// Every line before the closing one is still justified.
		justified := false
		for _, right := range extents[:len(extents)-1] {
			if abs32(right-contentRight) <= slack {
				justified = true
			}
		}
		if !justified {
			t.Fatalf("page %d has no justified line: %v", page.Number, extents)
		}
	}
	// The paragraph's final line is the article's last line and stays short too.
	final := lineExtents(doc.Pages[len(doc.Pages)-1])
	if got := final[len(final)-1]; got > contentRight-slack {
		t.Fatalf("the final line ends at %d, want narrower than %d", got, contentRight-slack)
	}
}

func TestAnchorsAreRecorded(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	var body strings.Builder
	for i := 1; i <= 5; i++ {
		body.WriteString("<h2 id=\"section-" + itoa(i) + "\">Раздел " + itoa(i) + "</h2><p>Текст раздела.</p>")
	}
	doc := h.runWithSize(`<style>h2 { font-size: 14pt }</style>`+body.String(), size, margin)
	if len(doc.Anchors) != 5 {
		t.Fatalf("got %d anchors, want 5: %v", len(doc.Anchors), doc.Anchors)
	}
	for i := 1; i <= 5; i++ {
		id := "section-" + itoa(i)
		anchor, ok := doc.Anchors[id]
		if !ok {
			t.Fatalf("anchor %q is missing", id)
		}
		if anchor.Page < 1 || anchor.Page > len(doc.Pages) {
			t.Fatalf("anchor %q points at page %d of %d", id, anchor.Page, len(doc.Pages))
		}
	}
}

func TestOutlineFromHeadings(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<h1>Договор</h1><h2>Предмет</h2><p>текст</p><h2>Оплата</h2>`)
	if len(doc.Outline) != 3 {
		t.Fatalf("got %d outline entries, want 3: %+v", len(doc.Outline), doc.Outline)
	}
	wantLevels := []int{1, 2, 2}
	wantTitles := []string{"Договор", "Предмет", "Оплата"}
	for i, e := range doc.Outline {
		if e.Level != wantLevels[i] || e.Title != wantTitles[i] {
			t.Fatalf("entry %d = %+v, want level %d title %q", i, e, wantLevels[i], wantTitles[i])
		}
		if e.Page != 1 {
			t.Fatalf("entry %d page = %d, want 1", i, e.Page)
		}
	}
}

func TestPageOrientation(t *testing.T) {
	h := newHarness(t)
	a4 := css.PageSize{Width: 595276, Height: 841890}
	a5 := css.PageSize{Width: 419528, Height: 595276}
	_, margin := smallPage()

	doc := h.runWithSize(`<style>@page { size: A4 landscape }</style><p>альбом</p>`, a4, margin)
	if got := doc.Pages[0].Size; got.Width != a4.Height || got.Height != a4.Width {
		t.Fatalf("A4 landscape = %+v, want %d x %d", got, a4.Height, a4.Width)
	}

	doc = h.runWithSize(`<style>@page { size: portrait }</style><p>портрет</p>`, a4, margin)
	if got := doc.Pages[0].Size; got != a4 {
		t.Fatalf("portrait with A4 fallback = %+v, want A4", got)
	}

	doc = h.runWithSize(`<style>@page { size: landscape }</style><p>альбом</p>`, a5, margin)
	if got := doc.Pages[0].Size; got.Width != a5.Height || got.Height != a5.Width {
		t.Fatalf("landscape with A5 fallback = %+v, want %d x %d", got, a5.Height, a5.Width)
	}
}

func TestBigTableFixture(t *testing.T) {
	src, err := os.ReadFile("../../testdata/bigtable.html")
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	doc := h.run(string(src))
	if len(doc.Pages) < 3 {
		t.Fatalf("got %d pages, want at least 3", len(doc.Pages))
	}
	// The first page starts with the title; every continuation must start with
	// the repeated table header.
	if !strings.Contains(pageText(doc, 1), "Дата") {
		t.Fatalf("the first page has no table header: %q", truncateText(pageText(doc, 1), 80))
	}
	for i := 2; i <= len(doc.Pages); i++ {
		if !strings.HasPrefix(pageText(doc, i), "Дата") {
			t.Fatalf("page %d does not start with the repeated header: %q", i, truncateText(pageText(doc, i), 60))
		}
	}
	last := len(doc.Pages)
	if !strings.Contains(pageText(doc, last), "Итого") {
		t.Fatalf("the totals row is missing from the last page")
	}
	for i := 1; i < last; i++ {
		if strings.Contains(pageText(doc, i), "Итого") {
			t.Fatalf("the totals row must only appear on the last page, found on %d", i)
		}
	}
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func TestPaginationClipsAnOversizedLine(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 100000, Height: 100000} // 100pt square
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>
		@page { size: 100pt 100pt; margin: 10pt }
		p { font-size: 10pt; line-height: 100pt; margin: 0 }
	</style><p>IMPORTANT</p>`
	doc, diags := h.runRaw(src, size, margin)
	if hasError(diags, "layout/no-progress") {
		t.Fatalf("an oversized line was reported as no progress: %v", diags.Lines())
	}
	if !hasWarning(doc, "layout/overflow") {
		t.Fatalf("expected an overflow warning, got %v", diags.Lines())
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("pagination produced %d pages, want 1", len(doc.Pages))
	}
	if len(doc.Pages[0].Ops) == 0 {
		t.Fatal("the clipped line was not placed")
	}
}

func hasError(l diag.List, code string) bool {
	for _, d := range l {
		if d.Code == code && d.Severity == diag.Error {
			return true
		}
	}
	return false
}

func TestPaginationIterationCapIsAnError(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 283465, Height: 170079}
	margin := [4]int32{14173, 14173, 14173, 14173}
	var paragraphs strings.Builder
	for i := 0; i < 40; i++ {
		paragraphs.WriteString("<p>Абзац для проверки предела итераций пагинации.</p>")
	}
	doc, err := html.Parse([]byte(paragraphs.String()), "cap.html")
	if err != nil {
		t.Fatal(err)
	}
	sheets, _ := stylesheet.Collect(doc, stylesheet.Options{})
	cascade := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	ctx := h.ctx
	ctx.Cascade = cascade
	ctx.PageSize = size
	ctx.Margin = margin
	ctx.PageStyle = ParsePageStyle(sheets, size, margin)
	root, _ := Build(doc, ctx)

	l := &layouter{
		ctx:           ctx,
		doc:           &Document{Anchors: map[string]Anchor{}},
		pageStyle:     ctx.PageStyle,
		faces:         map[string]*FontFace{},
		anchors:       map[string]Anchor{},
		maxIterations: 1,
	}
	l.paginate(root)
	if !hasError(l.doc.Diags, "layout/loop") {
		t.Fatalf("expected layout/loop, got %v", l.doc.Diags.Lines())
	}
}

func TestTwoHeaderRowsAppearOnceOnFirstPage(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	var rows strings.Builder
	for i := 0; i < 14; i++ {
		rows.WriteString("<tr><td>строка</td><td>данные</td></tr>")
	}
	src := `<style>
		table { width: 90mm; border-collapse: collapse }
		td, th { padding: 1mm; border: 0.5pt solid #000 }
	</style>
	<table>
	  <thead><tr><th>ПЕРВАЯ</th><th>ШАПКА</th></tr><tr><th>ВТОРАЯ</th><th>ШАПКА</th></tr></thead>
	  <tbody>` + rows.String() + `</tbody></table>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("got %d pages, want at least 2", len(doc.Pages))
	}
	first := pageText(doc, 1)
	if strings.Count(first, "ПЕРВАЯ") != 1 || strings.Count(first, "ВТОРАЯ") != 1 {
		t.Fatalf("the first page must show both header rows once: %q", truncateText(first, 120))
	}
	for i := 2; i <= len(doc.Pages); i++ {
		text := pageText(doc, i)
		if strings.Count(text, "ПЕРВАЯ") != 1 || strings.Count(text, "ВТОРАЯ") != 1 {
			t.Fatalf("page %d must repeat both header rows once: %q", i, truncateText(text, 120))
		}
	}
}

func TestRowspanShiftsFollowingCells(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>
		table { width: 80mm; border-collapse: collapse }
		td { border: 0.5pt solid #000; padding: 1mm }
	</style>
	<table>
	  <tr><td rowspan="2">A</td><td>B</td></tr>
	  <tr><td>C</td></tr>
	</table>`)
	xOf := func(r rune) int32 {
		for _, op := range textOps(doc) {
			if len(op.Run.Glyphs) == 1 && op.Run.Glyphs[0].Rune == r {
				return op.X
			}
		}
		return -1
	}
	bx, cx := xOf('B'), xOf('C')
	if bx < 0 || cx < 0 {
		t.Fatalf("cells not painted: B=%d C=%d", bx, cx)
	}
	if cx != bx {
		t.Fatalf("C starts at %d, want %d (the column of B)", cx, bx)
	}
}

func TestInheritanceThroughHTMLAndRows(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>html { font-size: 30pt }</style><p>текст</p>`)
	ops := textOps(doc)
	if len(ops) == 0 || ops[0].Face.Size != 30000 {
		t.Fatalf("html font-size did not reach body text: %+v", ops[0].Face.Size)
	}

	doc = h.run(`<style>tr { color: #ff0000 }</style>
		<table><tr><td>ячейка</td></tr></table>`)
	ops = textOps(doc)
	want := css.Color{R: 255, Set: true}
	if len(ops) == 0 || ops[0].Color != want {
		t.Fatalf("tr colour did not reach the cell: %+v", ops[0].Color)
	}

	doc = h.run(`<html style="color: #ff0000"><body><p>цвет</p></body></html>`)
	ops = textOps(doc)
	if len(ops) == 0 || ops[0].Color != want {
		t.Fatalf("inline style on html did not inherit: %+v", ops[0].Color)
	}
}

func TestMediaRulesApply(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>@media print { p { display: none } }</style><p>HIDDEN</p>`)
	if text := pageText(doc, 1); strings.Contains(text, "HIDDEN") {
		t.Fatalf("a rule inside @media print was ignored: %q", text)
	}
	doc = h.run(`<style>@media all { @media print { h1 { color: #ff0000 } } }</style><h1>T</h1>`)
	ops := textOps(doc)
	if len(ops) == 0 || ops[0].Color != (css.Color{R: 255, Set: true}) {
		t.Fatalf("nested @media print was ignored: %+v", ops[0].Color)
	}
	doc = h.run(`<style>@media all { p { color: #ff0000 } }</style><p>ALL</p>`)
	if ops := textOps(doc); len(ops) == 0 || ops[0].Color != (css.Color{R: 255, Set: true}) {
		t.Fatal("a rule inside @media all was ignored")
	}
}

func TestPageSizeInsideMedia(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>@media print { @page { size: A5 } }</style><p>x</p>`)
	if len(doc.Pages) == 0 {
		t.Fatal("no pages")
	}
	if doc.Pages[0].Size.Width != 419528 || doc.Pages[0].Size.Height != 595276 {
		t.Fatalf("page size = %dx%d, want A5", doc.Pages[0].Size.Width, doc.Pages[0].Size.Height)
	}
}

func TestChildrenUseParentContentBox(t *testing.T) {
	h := newHarness(t)
	base := textOps(h.run(`<p>X</p>`))[0].X
	padded := textOps(h.run(`<div style="padding:0 0 0 20pt"><p>X</p></div>`))[0].X
	if got := padded - base; got != 20000 {
		t.Fatalf("padding shifted text by %d, want 20000", got)
	}
	nested := textOps(h.run(`<div style="padding:0 0 0 10pt"><div style="padding:0 0 0 10pt"><p>X</p></div></div>`))[0].X
	if got := nested - base; got != 20000 {
		t.Fatalf("nested padding shifted text by %d, want 20000", got)
	}
}

func TestBottomInsetsAdvanceTheFlow(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<div style="padding:10pt">A</div><p style="margin:0">B</p>`)
	var ya, yb int32 = -1, -1
	for _, op := range textOps(doc) {
		if len(op.Run.Glyphs) != 1 {
			continue
		}
		switch op.Run.Glyphs[0].Rune {
		case 'A':
			ya = op.Y
		case 'B':
			yb = op.Y
		}
	}
	if ya < 0 || yb < 0 {
		t.Fatalf("baselines not found: A=%d B=%d", ya, yb)
	}
	if gap := yb - ya; gap < 24400 {
		t.Fatalf("baselines are %d apart, want at least 24400", gap)
	}
}

func TestPreformattedTextLines(t *testing.T) {
	h := newHarness(t)
	baselines := func(doc *Document) map[int32]bool {
		seen := map[int32]bool{}
		for _, op := range textOps(doc) {
			seen[op.Y] = true
		}
		return seen
	}
	doc := h.run("<p style=\"white-space:pre\">A\nB</p>")
	if got := baselines(doc); len(got) != 2 {
		t.Fatalf("LF must produce two baselines, got %d (%q)", len(got), pageText(doc, 1))
	}
	doc2 := h.run("<p style=\"white-space:pre\">A\r\nB</p>")
	if got := baselines(doc2); len(got) != 2 {
		t.Fatalf("CRLF must produce exactly one break, got %d baselines (%q)", len(got), pageText(doc2, 1))
	}
}

func TestTextIndentAppliesToFirstLine(t *testing.T) {
	h := newHarness(t)
	base := textOps(h.run(`<p>X</p>`))[0].X
	indented := textOps(h.run(`<p style="text-indent:20pt">X</p>`))[0].X
	if got := indented - base; got != 20000 {
		t.Fatalf("first line shifted by %d, want 20000", got)
	}
}

func TestUnitlessLineHeightInheritsAsFactor(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<div style="font-size:10pt;line-height:2"><p style="font-size:20pt;margin:0">A<br>B</p></div>`)
	var ys []int32
	for _, op := range textOps(doc) {
		if len(op.Run.Glyphs) == 1 {
			ys = append(ys, op.Y)
		}
	}
	if len(ys) < 2 {
		t.Fatalf("expected two lines, got %d", len(ys))
	}
	if gap := ys[1] - ys[0]; gap != 40000 {
		t.Fatalf("line gap = %d, want 40000", gap)
	}
}

func TestTableSpansAreBounded(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	cases := []string{
		`<table><tr><td rowspan="4294967296">A</td></tr></table>`,
		`<table><tr><td colspan="100000">A</td></tr></table>`,
		`<table><tr><td colspan="-3">A</td></tr></table>`,
	}
	for _, src := range cases {
		doc, diags := h.runRaw(src, size, margin)
		if !hasError(diags, "html/invalid-span") {
			t.Fatalf("%s: expected html/invalid-span, got %v (pages=%d)", src, diags.Lines(), len(doc.Pages))
		}
	}
	// Ordinary spans still lay out.
	doc, diags := h.runRaw(`<table><tr><td rowspan="2">A</td><td colspan="2">B</td></tr><tr><td>C</td><td>D</td></tr></table>`, size, margin)
	if diags.HasErrors() {
		t.Fatalf("ordinary spans: %v", diags.Lines())
	}
	if len(doc.Pages) == 0 || len(textOps(doc)) == 0 {
		t.Fatal("ordinary spans produced no content")
	}
}

func TestMarginBoxReportsMissingGlyph(t *testing.T) {
	h := newHarness(t)
	size, margin := smallPage()
	missing := "\U0010FFFF"
	src := `<style>@page { @bottom-center { content: "` + missing + `" } }</style><p>x</p>`
	_, diags := h.runRaw(src, size, margin)
	if !hasError(diags, "text/glyph") {
		t.Fatalf("a missing glyph in a margin box must be reported, got %v", diags.Lines())
	}
}

func TestNestedTableRendersOnce(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<table><tr><td><table><tr><td>INNER</td></tr></table></td></tr></table>`)
	count := 0
	for _, op := range textOps(doc) {
		if strings.Contains(pageTextRun(op), "INNER") {
			count++
		}
	}
	if count == 0 {
		t.Fatal("the nested table was not rendered")
	}
}

func pageTextRun(op *TextOp) string {
	var b strings.Builder
	for _, g := range op.Run.Glyphs {
		b.WriteRune(g.Rune)
	}
	return b.String()
}

func TestNestedListIndentation(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<ol><li>A<ol><li>B</li></ol>C</li></ol>`)
	xOf := func(r rune) int32 {
		for _, op := range textOps(doc) {
			for _, g := range op.Run.Glyphs {
				if g.Rune == r {
					return op.X
				}
			}
		}
		return -1
	}
	a, b, c := xOf('A'), xOf('B'), xOf('C')
	if a < 0 || b < 0 || c < 0 {
		t.Fatalf("text is missing: A=%d B=%d C=%d", a, b, c)
	}
	if b <= a {
		t.Fatalf("the inner item is not indented: A=%d B=%d", a, b)
	}
	if c != a {
		t.Fatalf("text after the nested list moved: A=%d C=%d", a, c)
	}
}

func TestBodyStylesheetIsNotRendered(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p>A</p><style>p { color: #ff0000 }</style><p>B</p>`)
	text := pageText(doc, 1)
	if strings.Contains(text, "color") || strings.Contains(text, "{") {
		t.Fatalf("the stylesheet source leaked into the document: %q", text)
	}
	if !strings.Contains(text, "A") || !strings.Contains(text, "B") {
		t.Fatalf("paragraphs are missing: %q", text)
	}
	for _, op := range textOps(doc) {
		if op.Color != (css.Color{R: 255, Set: true}) {
			t.Fatalf("the body stylesheet was not applied: %+v", op.Color)
		}
	}
}

func TestHiddenTablePartsAreSkipped(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<table>
		<tr style="display:none"><td>HIDDENROW</td></tr>
		<tr><td style="display:none">HIDDENCELL</td><td>VISIBLE</td></tr>
	</table>`)
	text := pageText(doc, 1)
	if strings.Contains(text, "HIDDENROW") || strings.Contains(text, "HIDDENCELL") {
		t.Fatalf("hidden table content was rendered: %q", text)
	}
	if !strings.Contains(text, "VISIBLE") {
		t.Fatalf("visible cell is missing: %q", text)
	}
	// A hidden row must not consume vertical space.
	withHidden := h.run(`<table><tr style="display:none"><td>X</td></tr><tr><td>V</td></tr></table>`)
	without := h.run(`<table><tr><td>V</td></tr></table>`)
	if len(withHidden.Pages) != len(without.Pages) {
		t.Fatalf("hidden row changed pagination: %d vs %d pages", len(withHidden.Pages), len(without.Pages))
	}
}

func TestHeaderStylesComeFromTheCascade(t *testing.T) {
	h := newHarness(t)
	// The test font has a single weight, so alignment is the observable
	// property: the author rule must win over the UA default.
	styled := h.run(`<style>th { text-align: left }</style>
		<table><tr><th>H</th></tr></table>`)
	ops := textOps(styled)
	if len(ops) == 0 {
		t.Fatal("no header text")
	}
	leftX := ops[0].X

	unstyled := h.run(`<table><tr><th>H</th></tr></table>`)
	ops = textOps(unstyled)
	if len(ops) == 0 {
		t.Fatal("no header text")
	}
	if ops[0].X <= leftX {
		t.Fatalf("the UA centered header is not to the right of the left-aligned one: %d vs %d", ops[0].X, leftX)
	}
}

func TestImageAspectRatioWithPercentageWidth(t *testing.T) {
	img := &ImageData{Width: 80000, Height: 40000}
	style := css.DefaultStyle()
	style.Width = css.Pct(50)
	style.Height = css.Length{Kind: css.Auto}
	w, h := imageSize(style, img, 200000, Context{})
	if w != 100000 || h != 50000 {
		t.Fatalf("50%% width = %dx%d, want 100000x50000", w, h)
	}
	style = css.DefaultStyle()
	style.Width = css.Abs(50000)
	w, h = imageSize(style, img, 200000, Context{})
	if w != 50000 || h != 25000 {
		t.Fatalf("absolute width = %dx%d, want 50000x25000", w, h)
	}
	style = css.DefaultStyle()
	style.Width = css.Pct(50)
	style.Height = css.Abs(30000)
	w, h = imageSize(style, img, 200000, Context{})
	if w != 100000 || h != 30000 {
		t.Fatalf("explicit height was overwritten: %dx%d", w, h)
	}
}

func TestBreakBeforeFirstContentKeepsOnePage(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p style="break-before: page">FIRST</p>`)
	if len(doc.Pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(doc.Pages))
	}
	doc = h.run(`<p>A</p><p style="break-before: page">B</p>`)
	if len(doc.Pages) != 2 {
		t.Fatalf("a break after content must create a second page, got %d", len(doc.Pages))
	}
	if !strings.Contains(pageText(doc, 2), "B") {
		t.Fatalf("B is not on the second page: %q", pageText(doc, 2))
	}
}

func TestWidowsPullLinesToTheNextPage(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 100000, Height: 60000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>p { margin: 0; font-size: 10pt; line-height: 12pt; orphans: 2; widows: 3 }</style><p>A<br>B<br>C<br>D<br>E</p>`
	doc, diags := h.runRaw(src, size, margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	first, second := pageText(doc, 1), pageText(doc, 2)
	if !strings.Contains(first, "A") || !strings.Contains(first, "B") || strings.Contains(first, "C") {
		t.Fatalf("first page = %q, want AB", first)
	}
	if !strings.Contains(second, "C") || !strings.Contains(second, "D") || !strings.Contains(second, "E") {
		t.Fatalf("second page = %q, want CDE", second)
	}
}

func TestHorizontalMarginsShiftBlocks(t *testing.T) {
	h := newHarness(t)
	base := textOps(h.run(`<p>X</p>`))[0].X
	shifted := textOps(h.run(`<p style="margin:0 0 0 20pt">X</p>`))[0].X
	if got := shifted - base; got != 20000 {
		t.Fatalf("left margin shifted text by %d, want 20000", got)
	}
	percent := textOps(h.run(`<p style="margin:0 0 0 10%">X</p>`))[0].X
	if percent <= base {
		t.Fatalf("percentage margin did not shift text: %d vs %d", percent, base)
	}
}

func TestFixedHeightExtendsDecorations(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<div style="height:40pt;background-color:#ff0000"></div><p>A</p>`)
	found := false
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == FillOpKind && op.Color == (css.Color{R: 255, Set: true}) {
			if op.Rect.H != 40000 {
				t.Fatalf("the background is %d mp high, want 40000", op.Rect.H)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no background was painted")
	}
}

func TestEmptyBlockTakesNoSpace(t *testing.T) {
	h := newHarness(t)
	yOf := func(doc *Document) int32 {
		for _, op := range textOps(doc) {
			for _, g := range op.Run.Glyphs {
				if g.Rune == 'A' {
					return op.Y
				}
			}
		}
		return -1
	}
	plain := yOf(h.run(`<p>A</p>`))
	withEmpty := yOf(h.run(`<div></div><p>A</p>`))
	if plain < 0 || withEmpty < 0 {
		t.Fatal("text is missing")
	}
	if withEmpty != plain {
		t.Fatalf("an empty block moved the text by %d mp", withEmpty-plain)
	}
}

func TestListMarkerSitsBeforeTheText(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<ol><li>A</li><li>B</li></ol>`)
	var markerX, textX int32 = -1, -1
	for _, op := range textOps(doc) {
		label := pageTextRun(op)
		switch {
		case strings.HasPrefix(label, "1."):
			markerX = op.X
		case label == "A":
			textX = op.X
		}
	}
	if markerX < 0 || textX < 0 {
		t.Fatalf("marker or text missing: marker=%d text=%d", markerX, textX)
	}
	if markerX >= textX {
		t.Fatalf("the marker starts at %d, text at %d; the marker overlaps", markerX, textX)
	}
	if markerX < 0 {
		t.Fatalf("the marker is outside the page: %d", markerX)
	}
}

func TestImageMoveUsesOnePage(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 100000, Height: 100000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	img := image.NewRGBA(image.Rect(0, 0, 10, 20))
	for i := range img.Pix {
		img.Pix[i] = 100
	}
	f, err := os.Create(filepath.Join(h.images.dir, "i.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	src := `<p style="height:65pt;margin:0">A</p><img style="display:block;width:10pt;height:20pt" src="i.png"><p>B</p>`
	doc, diags := h.runRaw(src, size, margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if len(doc.Pages) != 2 {
		t.Fatalf("got %d pages, want 2 (ops per page: %d/%d)", len(doc.Pages), len(doc.Pages[0].Ops), len(doc.Pages[1].Ops))
	}
	if len(doc.Pages[1].Ops) == 0 {
		t.Fatal("the second page is empty")
	}
}

func TestTextIndentInherits(t *testing.T) {
	h := newHarness(t)
	plain := textOps(h.run(`<p>X</p>`))[0].X
	indented := textOps(h.run(`<style>body { text-indent: 20pt }</style><p>X</p>`))[0].X
	if got := indented - plain; got != 20000 {
		t.Fatalf("inherited text-indent shifted the line by %d, want 20000", got)
	}
}

func TestPreformattedTextDoesNotWrap(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p style="white-space:pre;width:20pt;margin:0">A B C D E F</p>`)
	baselines := map[int32]bool{}
	for _, op := range textOps(doc) {
		baselines[op.Y] = true
	}
	if len(baselines) != 1 {
		t.Fatalf("preformatted text produced %d baselines, want one line", len(baselines))
	}
}

func TestPageDeclarationsHonourImportance(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 595276, Height: 841890}
	margin := [4]int32{20000, 20000, 20000, 20000}
	src := `<style>
		@page { size: A5 !important; margin: 10pt !important }
		@page { size: A4; margin: 20pt }
	</style><p>x</p>`
	doc, diags := h.runRaw(src, size, margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if len(doc.Pages) == 0 {
		t.Fatal("no pages")
	}
	page := doc.Pages[0]
	if page.Size.Width != 419528 || page.Size.Height != 595276 {
		t.Fatalf("page size = %dx%d, want A5", page.Size.Width, page.Size.Height)
	}
	if page.Margin[0] != 10000 {
		t.Fatalf("top margin = %d, want 10000 (important declaration)", page.Margin[0])
	}
}

func TestCellBlockMarginsAdvanceContent(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<table><tr><td><p style="margin:0 0 20pt">A</p><p style="margin:0">B</p></td></tr></table>`)
	var ya, yb int32 = -1, -1
	for _, op := range textOps(doc) {
		switch pageTextRun(op) {
		case "A":
			ya = op.Y
		case "B":
			yb = op.Y
		}
	}
	if ya < 0 || yb < 0 {
		t.Fatalf("baselines missing: A=%d B=%d", ya, yb)
	}
	if gap := yb - ya; gap < 34400 {
		t.Fatalf("baselines are %d apart, want at least 34400", gap)
	}
}

func TestInlineAnchorsAreRegistered(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p><span id="target">TARGET</span></p>`)
	anchor, ok := doc.Anchors["target"]
	if !ok {
		t.Fatal("an inline target was not registered")
	}
	if anchor.Page != 1 {
		t.Fatalf("anchor page = %d, want 1", anchor.Page)
	}
	doc = h.run(`<table><tr><td><span id="cell">C</span></td></tr></table>`)
	if _, ok := doc.Anchors["cell"]; !ok {
		t.Fatal("a target inside a table cell was not registered")
	}
}

// TestEmptyInlineAnchorsAreRegistered guards RV5-41: an inline element with no
// content produced no box, so its id never reached the anchor map, and
// buildLines dropped a line that carried only anchors.
func TestEmptyInlineAnchorsAreRegistered(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p>text <a id="x"></a></p><p><span id="split"></span>tail</p>`)
	for _, id := range []string{"x", "split"} {
		anchor, ok := doc.Anchors[id]
		if !ok {
			t.Fatalf("the empty %q anchor is missing: %v", id, doc.Anchors)
		}
		if anchor.Page != 1 {
			t.Fatalf("anchor %q points at page %d, want 1", id, anchor.Page)
		}
	}
	// An empty anchor adds neither a visible line nor a page.
	plain := h.run(`<p>text</p><p>tail</p>`)
	if len(doc.Pages) != len(plain.Pages) {
		t.Fatalf("the empty anchors changed the page count: %d vs %d", len(doc.Pages), len(plain.Pages))
	}
	if got, want := len(textOps(doc)), len(textOps(plain)); got != want {
		t.Fatalf("the empty anchors changed the visible line count: %d runs vs %d", got, want)
	}
	// A paragraph holding nothing but an anchor still resolves, and the
	// zero-height carrier line it needs paints nothing.
	only := h.run(`<p><span id="empty"></span></p>`)
	bare := h.run(`<p></p>`)
	anchor, ok := only.Anchors["empty"]
	if !ok || anchor.Page != 1 {
		t.Fatalf("the anchor of an empty paragraph is %+v (present %v), want page 1", anchor, ok)
	}
	if got := len(textOps(only)); got != 0 {
		t.Fatalf("the empty anchor painted %d runs, want none", got)
	}
	if len(only.Pages) != len(bare.Pages) {
		t.Fatalf("the empty anchor changed the page count: %d vs %d", len(only.Pages), len(bare.Pages))
	}
}

// TestEmptyAnchorBeforeAWrappingWord covers the wrapping case of RV5-41: an
// anchor waits for the first item it introduces, so a word that moves to the
// next line (and the next page) takes the anchor with it.
func TestEmptyAnchorBeforeAWrappingWord(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 90000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	word := strings.Repeat("\u0436", 18)
	src := `<style>
		@page { size: 200pt 90pt; margin: 10pt }
		p { margin: 0; font-size: 10pt; line-height: 40pt }
	</style><p>filler <span id="wrap"></span>` + word + `</p>`
	doc, diags := h.runRaw(src, size, margin)
	if n := countOverflow(diags); n != 0 {
		t.Fatalf("the fixture overflows: %v", diags.Lines())
	}
	anchor, ok := doc.Anchors["wrap"]
	if !ok {
		t.Fatalf("the empty anchor is missing: %v", doc.Anchors)
	}
	// The word does not fit after "filler" and moves to the next page; the
	// anchor must follow it and not stay on the line it was written after.
	fillerPage, wordPage := 0, 0
	var wrapped *TextOp
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			var b strings.Builder
			for _, g := range op.Text.Run.Glyphs {
				b.WriteRune(g.Rune)
			}
			switch b.String() {
			case "filler":
				fillerPage = page.Number
			case word:
				wordPage, wrapped = page.Number, op.Text
			}
		}
	}
	if fillerPage == 0 || wordPage == 0 {
		t.Fatalf("the fixture did not paint both lines (filler page %d, word page %d)", fillerPage, wordPage)
	}
	if wordPage == fillerPage {
		t.Fatalf("the long word stayed on page %d, want it to wrap to the next page", wordPage)
	}
	if anchor.Page != wordPage {
		t.Fatalf("the anchor points at page %d, want the wrapped word's page %d", anchor.Page, wordPage)
	}
	// The anchor is the top of the line the word starts on.
	if want := wrapped.Y - wrapped.Face.Ascent; anchor.Y != want {
		t.Fatalf("the anchor is at y=%d, the wrapped word's line starts at %d", anchor.Y, want)
	}
}

// layoutWithTargets lays out src with a known target map, the way the engine's
// second pass resolves target-counter().
func layoutWithTargets(t *testing.T, h *harness, src string, size css.PageSize, margin [4]int32, targets map[string]int) (*Document, diag.List) {
	t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		t.Fatal(err)
	}
	sheets, _ := stylesheet.Collect(doc, stylesheet.Options{})
	cascade := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	ps := ParsePageStyle(sheets, size, margin)
	ctx := h.ctx
	ctx.Cascade = cascade
	ctx.PageStyle = ps
	ctx.PageSize = size
	ctx.Margin = margin
	ctx.TargetPages = targets
	return Run(doc, ctx)
}

// TestEmptyAnchorOnTheSecondPageResolves guards RV5-41 on a two-page document:
// a reference to an empty id on page 2 must show page 2 instead of the
// pagination/unknown-target the engine reports for an id missing from the
// anchor map.
func TestEmptyAnchorOnTheSecondPageResolves(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>
		@page { size: 200pt 200pt; margin: 10pt }
		p { margin: 0 }
		a::after { content: " " target-counter(attr(href), page) }
	</style>
	<p><a href="#target">Target page</a></p>
	<p style="break-after: page">first page</p>
	<p><span id="target"></span>second page</p>`
	first, _ := layoutWithTargets(t, h, src, size, margin, nil)
	targets := map[string]int{}
	for id, anchor := range first.Anchors {
		targets[id] = anchor.Page
	}
	doc, diags := layoutWithTargets(t, h, src, size, margin, targets)
	if len(doc.Pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(doc.Pages))
	}
	anchor, ok := doc.Anchors["target"]
	if !ok {
		t.Fatalf("the empty anchor on page 2 is missing: %v", doc.Anchors)
	}
	if anchor.Page != 2 {
		// The engine warns pagination/unknown-target exactly when the anchor
		// page is 0, so a non-zero page here rules that diagnostic out.
		t.Fatalf("the empty anchor points at page %d, want 2", anchor.Page)
	}
	if !doc.Targets["target"] {
		t.Fatal("the target-counter reference did not consume the target")
	}
	if text := pageText(doc, 1); !strings.Contains(text, "Target page 2") {
		t.Fatalf("the reference shows %q, want %q", text, "Target page 2")
	}
	if len(diags.Lines()) != 0 {
		t.Fatalf("unexpected layout diagnostics: %v", diags.Lines())
	}
}

func TestSpacesCollapseAcrossInlineNodes(t *testing.T) {
	h := newHarness(t)
	count := func(doc *Document) int {
		n := 0
		for _, op := range textOps(doc) {
			if pageTextRun(op) == " " {
				n++
			}
		}
		return n
	}
	if got := count(h.run(`<p>A <span> B</span> C</p>`)); got != 2 {
		t.Fatalf("collapse across nodes: %d spaces, want 2", got)
	}
	if got := count(h.run(`<p>A<b> B</b><i> C</i></p>`)); got != 2 {
		t.Fatalf("collapse across nested inline: %d spaces, want 2", got)
	}
	if got := count(h.run(`<p>A </p>`)); got != 0 {
		t.Fatalf("a trailing space was kept: %d", got)
	}
	if got := count(h.run(`<p style="white-space:pre">A   B</p>`)); got != 3 {
		t.Fatalf("preformatted spaces were collapsed: %d, want 3", got)
	}
}

func TestCellVerticalAlignment(t *testing.T) {
	h := newHarness(t)
	yOfBottom := func(doc *Document) int32 {
		for _, op := range textOps(doc) {
			if pageTextRun(op) == "BOTTOM" {
				return op.Y
			}
		}
		return -1
	}
	top := yOfBottom(h.run(`<table><tr><td>BOTTOM</td><td>A<br>B<br>C</td></tr></table>`))
	bottom := yOfBottom(h.run(`<table><tr><td style="vertical-align:bottom">BOTTOM</td><td>A<br>B<br>C</td></tr></table>`))
	if top < 0 || bottom < 0 {
		t.Fatalf("text missing: top=%d bottom=%d", top, bottom)
	}
	if bottom <= top {
		t.Fatalf("bottom alignment did not move the text: %d vs %d", bottom, top)
	}
}

// TestRepeatedHeaderDoesNotLoopOnOversizedRow guards RV2-05: painting the
// repeated header marked the page non-empty, so a row taller than the page was
// never clipped: every new page got a header, the row did not fit again, and
// pagination produced thousands of header-only pages.
func TestRepeatedHeaderDoesNotLoopOnOversizedRow(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 100000} // 200x100 pt
	margin := [4]int32{}
	src := `<table><thead><tr><th>HEAD</th></tr></thead>
	<tbody><tr><td><div style="height:200pt">BODY</div></td></tr></tbody></table>`
	doc, diags := h.runRaw(src, size, margin)
	if hasError(diags, "layout/loop") {
		t.Fatalf("pagination did not converge: %v", diags.Lines())
	}
	if hasError(diags, "layout/no-progress") {
		t.Fatalf("content was reported as unplaceable: %v", diags.Lines())
	}
	if len(doc.Pages) > 2 {
		t.Fatalf("an oversized row produced %d pages, want at most 2", len(doc.Pages))
	}
	if !hasWarning(doc, "layout/overflow") {
		t.Fatalf("expected an overflow warning, got %v", diags.Lines())
	}
	// The body row must be painted exactly once, not lost among headers.
	bodyPages := 0
	for i := 1; i <= len(doc.Pages); i++ {
		if strings.Contains(pageText(doc, i), "BODY") {
			bodyPages++
		}
	}
	if bodyPages != 1 {
		t.Fatalf("the oversized row was painted on %d pages, want exactly 1", bodyPages)
	}
}

// TestRepeatedHeaderStillBreaksWhenTheRowFits checks the fix did not turn real
// continuation breaks into clipping: a row that fits a fresh page with the
// repeated header must still move to the next page.
func TestRepeatedHeaderStillBreaksWhenTheRowFits(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 100000}
	margin := [4]int32{}
	src := `<table><thead><tr><th>HEAD</th></tr></thead><tbody>
	<tr><td><div style="height:30pt">A</div></td></tr>
	<tr><td><div style="height:30pt">B</div></td></tr>
	<tr><td><div style="height:30pt">C</div></td></tr>
	</tbody></table>`
	doc := h.runWithSize(src, size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("the rows did not continue on a second page: %d pages", len(doc.Pages))
	}
	seen := 0
	for i := 1; i <= len(doc.Pages); i++ {
		if strings.Contains(pageText(doc, i), "C") {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("row C appears on %d pages, want 1", seen)
	}
}

// TestHyphenAllowsLineBreak guards RV2-11: text.Split ended a word at a hyphen
// but the line builder cleared the break opportunity after every word, so
// "слово-слово" overflowed a narrow block instead of breaking after the hyphen.
func TestHyphenAllowsLineBreak(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 300000, Height: 300000}
	margin := [4]int32{20000, 20000, 20000, 20000}
	src := `<p style="width:60pt;margin:0;font-size:10pt;line-height:12pt">слово-слово</p>`
	doc, diags := h.runRaw(src, size, margin)
	if hasError(diags, "layout/overflow") {
		t.Fatalf("hyphenated text did not break: %v", diags.Lines())
	}
	ys := map[int32]bool{}
	var firstX, lastRight int32
	first := true
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		ys[op.Text.Y] = true
		if first {
			firstX = op.Text.X
			first = false
		}
		lastRight = op.Text.X + op.Text.Run.Width
	}
	if len(ys) != 2 {
		t.Fatalf("hyphenated text produced %d baselines, want 2 (a break after the hyphen)", len(ys))
	}
	if firstX != 20000 {
		t.Fatalf("the text starts at X=%d, want the content left edge 20000", firstX)
	}
	if limit := int32(20000 + 60000); lastRight > limit {
		t.Fatalf("the text ends at X=%d, past the content right edge %d", lastRight, limit)
	}
	// A space still breaks text the same way, and an inline element boundary
	// still does not create a break point (REV-25).
	if got := pageText(doc, 1); got != "слово-слово" {
		t.Fatalf("the text layer = %q", got)
	}
}

// TestCellContentUsesBlockWidth guards RV2-15: measuring a table cell ignored
// the width and horizontal margins of a block inside it, so the block wrapped
// at the cell width while the row height did not cover the painted lines.
func TestCellContentUsesBlockWidth(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 300000, Height: 300000}
	margin := [4]int32{20000, 20000, 20000, 20000}
	src := `<style>table{width:200pt}td{padding:0;border:0}
	p{width:40pt;margin:0;font-size:10pt;line-height:12pt}</style>
	<table><tr><td><p>AAAAA BBBBB</p></td></tr></table><p>AFTER</p>`
	doc, diags := h.runRaw(src, size, margin)
	if hasError(diags, "layout/overflow") {
		t.Fatalf("the cell content overflowed: %v", diags.Lines())
	}
	var runs []struct {
		x, y, right int32
	}
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		runs = append(runs, struct{ x, y, right int32 }{op.Text.X, op.Text.Y, op.Text.X + op.Text.Run.Width})
	}
	if len(runs) != 3 {
		t.Fatalf("got %d text runs, want 3 (two cell lines and AFTER): %+v", len(runs), runs)
	}
	if runs[0].y == runs[1].y {
		t.Fatalf("the cell paragraph did not wrap: %+v", runs)
	}
	for _, r := range runs[:2] {
		if r.x != 20000 {
			t.Fatalf("a cell line starts at X=%d, want the content left edge 20000", r.x)
		}
		if r.right > 20000+40000 {
			t.Fatalf("a run ends at X=%d, past the 40pt content box", r.right)
		}
	}
	if runs[2].y <= runs[1].y {
		t.Fatalf("AFTER baseline %d overlaps the second cell line %d", runs[2].y, runs[1].y)
	}

	// A left margin inside the cell shifts the content and shrinks the box the
	// same way it does outside a table.
	src = `<style>table{width:200pt}td{padding:0;border:0}
	p{width:30pt;margin:0 0 0 10pt;font-size:10pt;line-height:12pt}</style>
	<table><tr><td><p>AAAA BBBB</p></td></tr></table>`
	doc, diags = h.runRaw(src, size, margin)
	if hasError(diags, "layout/overflow") {
		t.Fatalf("the margined cell content overflowed: %v", diags.Lines())
	}
	marginedYs := map[int32]bool{}
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		marginedYs[op.Text.Y] = true
		if op.Text.X != 30000 {
			t.Fatalf("the margined block starts at X=%d, want 30000", op.Text.X)
		}
		if right := op.Text.X + op.Text.Run.Width; right > 60000 {
			t.Fatalf("a run ends at X=%d, past the content right edge 60000", right)
		}
	}
	if len(marginedYs) != 2 {
		t.Fatalf("the margined block produced %d baselines, want 2", len(marginedYs))
	}
}

// TestCollapsedBordersDrawOneLine guards RV2-16: with border-collapse the
// table border and the cell border were painted side by side, so an outer edge
// was twice as thick as declared, and the same doubling left an unpainted
// stripe before the next cell's text.
func TestCollapsedBordersDrawOneLine(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 300000, Height: 200000}
	margin := [4]int32{}
	style := func(collapse string) string {
		return `<style>table{width:100pt;border:2pt solid #ff0000;border-collapse:` + collapse + `}
		td{border:2pt solid #0000ff;padding:0;font-size:10pt}</style>
		<table><tr><td>A</td><td>B</td></tr></table>`
	}
	edgeThickness := func(doc *Document) (left, right, top, bottom int32) {
		for _, op := range doc.Pages[0].Ops {
			if op.Kind != FillOpKind {
				continue
			}
			r := op.Rect
			// Only the strips along each edge count; the frame sides meet in
			// the corners and would otherwise be counted twice.
			if r.X == 0 && r.H == 20000 {
				left += r.W
			}
			if r.X+r.W == 104000 && r.H == 20000 {
				right += r.W
			}
			if r.Y == 0 && r.W == 104000 {
				top += r.H
			}
			if r.Y+r.H == 20000 && r.W == 104000 {
				bottom += r.H
			}
		}
		return
	}

	collapsed, _ := h.runRaw(style("collapse"), size, margin)
	left, right, top, bottom := edgeThickness(collapsed)
	if left != 2000 || right != 2000 || top != 2000 || bottom != 2000 {
		t.Fatalf("collapsed outer edges = %d/%d/%d/%d mp, want 2000 everywhere", left, right, top, bottom)
	}
	// One shared line between the cells, and the second cell's text starts right
	// after it: no unpainted stripe.
	shared := 0
	for _, op := range collapsed.Pages[0].Ops {
		if op.Kind == FillOpKind && op.Rect.X == 52000 && op.Rect.W == 2000 {
			shared++
		}
	}
	if shared != 1 {
		t.Fatalf("the shared vertical border is painted %d times, want 1", shared)
	}
	var textBX int32 = -1
	for _, op := range collapsed.Pages[0].Ops {
		if op.Kind == TextOpKind && op.Text != nil && string(rune(op.Text.Run.Glyphs[0].Rune)) == "B" {
			textBX = op.Text.X
		}
	}
	if textBX != 54000 {
		t.Fatalf("the second cell's text starts at X=%d, want 54000 (right after the shared border)", textBX)
	}

	// Separate borders keep their own edges: the outer edge is twice as thick.
	separate, _ := h.runRaw(style("separate"), size, margin)
	collapseEdge := contiguousEdge(t, collapsed, 10000)
	separateEdge := contiguousEdge(t, separate, 10000)
	if collapseEdge != 2000 {
		t.Fatalf("the collapsed left edge is %d mp thick, want 2000", collapseEdge)
	}
	if separateEdge != 4000 {
		t.Fatalf("the separate left edge is %d mp thick, want 4000 (table border plus cell border)", separateEdge)
	}
}

// contiguousEdge returns how far the painted area extends from X=0 along the
// given horizontal line, so an empty stripe stops the count.
func contiguousEdge(t *testing.T, doc *Document, y int32) int32 {
	t.Helper()
	var spans [][2]int32
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != FillOpKind {
			continue
		}
		r := op.Rect
		if y >= r.Y && y < r.Y+r.H {
			spans = append(spans, [2]int32{r.X, r.X + r.W})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var end int32
	for _, sp := range spans {
		if sp[0] > end {
			break
		}
		if sp[1] > end {
			end = sp[1]
		}
	}
	return end
}

// TestInlineVerticalAlignShiftsTheRun guards RV2-17: every inline run was
// painted on the line baseline, so vertical-align: top, middle and bottom on a
// span had no effect at all.
func TestInlineVerticalAlignShiftsTheRun(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>
		p{margin:0;font-size:20pt;line-height:24pt}
		.small{font-size:6pt;line-height:6pt}
	</style><p>AA<span class="small" style="vertical-align:top">T</span>
	<span class="small" style="vertical-align:middle">M</span>
	<span class="small" style="vertical-align:bottom">D</span>
	<span class="small">B</span></p>`)
	ys := map[rune]int32{}
	for _, op := range textOps(doc) {
		for _, g := range op.Run.Glyphs {
			ys[g.Rune] = op.Y
		}
	}
	top, okTop := ys['T']
	middle, okMiddle := ys['M']
	bottom, okBottom := ys['D']
	baseline, okBase := ys['B']
	if !okTop || !okMiddle || !okBottom || !okBase {
		t.Fatalf("not every span was placed: %v", ys)
	}
	if top >= middle || middle >= baseline || baseline >= bottom {
		t.Fatalf("vertical-align offsets are wrong: top=%d middle=%d baseline=%d bottom=%d", top, middle, baseline, bottom)
	}
	if centre := (top + bottom) / 2; abs32(middle-centre) > 2000 {
		t.Fatalf("middle %d is not near the centre %d of top %d and bottom %d", middle, centre, top, bottom)
	}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// TestTablePaddingShiftsRows guards RV2-18: columns were computed for the
// table's content width but the rows started at the border, so a padded table
// drew its cells over the padding and left the padding space empty on the right.
func TestTablePaddingShiftsRows(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 300000, Height: 200000}
	margin := [4]int32{}
	doc, diags := h.runRaw(`<style>
		table{width:100pt;padding:0 0 0 20pt;border:1pt solid #ff0000;border-collapse:separate}
		td{padding:0;border:0;font-size:10pt;background-color:#00ff00}
	</style><table><tr><td>A</td><td>B</td></tr></table>`, size, margin)
	if hasError(diags, "layout/overflow") {
		t.Fatalf("unexpected diagnostics: %v", diags.Lines())
	}
	xs := map[string]int32{}
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == TextOpKind && op.Text != nil && len(op.Text.Run.Glyphs) == 1 {
			xs[string(op.Text.Run.Glyphs[0].Rune)] = op.Text.X
		}
	}
	if xs["A"] != 21000 {
		t.Fatalf("the first cell starts at X=%d, want 21000 (border plus padding)", xs["A"])
	}
	if xs["B"] != 71000 {
		t.Fatalf("the second cell starts at X=%d, want 71000", xs["B"])
	}
	// The last column ends at the content edge, not past it.
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != FillOpKind || op.Color != (css.Color{R: 0, G: 255, Set: true}) {
			continue
		}
		right := op.Rect.X + op.Rect.W
		if right > 121000 {
			t.Fatalf("a cell background ends at X=%d, past the content right edge 121000", right)
		}
	}
}

// TestInlineImageAnchorIsRegistered guards RV2-19: an id on an inline image was
// not recorded as an anchor, so a target-counter in the table of contents
// printed 0 and validation warned about an unknown id.
func TestInlineImageAnchorIsRegistered(t *testing.T) {
	h := newHarness(t)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	path := filepath.Join(h.images.dir, "fig.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	doc, diags := h.runRaw(`<style>p{margin:0}</style>
	<a href="#fig">Figure page </a><p><img id="fig" src="fig.png" style="width:20pt;height:20pt"></p>`,
		css.PageSize{Width: 200000, Height: 200000}, [4]int32{10000, 10000, 10000, 10000})
	if hasWarning(doc, "pagination/unknown-target") || hasError(diags, "pagination/unknown-target") {
		t.Fatalf("the image anchor was reported as unknown: %v", diags.Lines())
	}
	anchor, ok := doc.Anchors["fig"]
	if !ok {
		t.Fatalf("the image id was not registered as an anchor: %v", doc.Anchors)
	}
	if anchor.Page != 1 {
		t.Fatalf("the image anchor points at page %d, want 1", anchor.Page)
	}
}

// TestHugeMarginsDoNotWrapTheCursor guards RV2-20: two margins at the maximum
// accepted length overflowed the int32 cursor, so the following paragraph was
// painted at a negative Y with no diagnostic.
func TestHugeMarginsDoNotWrapTheCursor(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	doc, diags := h.runRaw(`<style>
		@page { size: 200pt 200pt; margin: 10pt }
		p { margin: 0 }
		div { margin: 0 0 2000000pt 0 }
	</style><p>TOP</p><div></div><div></div><p>AFTER</p>`, size, margin)
	if len(diags.Lines()) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags.Lines())
	}
	var afterPage int
	var afterY int32
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			if op.Text.Y < 0 {
				t.Fatalf("a run was placed at Y=%d", op.Text.Y)
			}
			for _, g := range op.Text.Run.Glyphs {
				if g.Rune == 'A' && op.Text.X == 10000 {
					afterPage = page.Number
					afterY = op.Text.Y
				}
			}
		}
	}
	if afterPage == 0 {
		t.Fatal("the paragraph after the huge margins was not placed")
	}
	if afterPage == 1 && afterY > 150000 {
		t.Fatalf("the paragraph after the huge margins was painted below the page at Y=%d", afterY)
	}
}

// TestTableRowBreak guards RV2-29: break-before/after: page on a tr was accepted
// by the profile and then ignored by layout, so the table stayed on one page.
func TestTableRowBreak(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	base := `<style>table{width:100pt}td{padding:0;border:0;font-size:10pt}</style>`

	doc, diags := h.runRaw(base+`<p>FIRST</p><table><tr><td>ROW1</td></tr>
	<tr style="break-before:page"><td>ROW2</td></tr></table>`, size, margin)
	if len(diags.Lines()) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags.Lines())
	}
	if len(doc.Pages) != 2 {
		t.Fatalf("break-before on a row produced %d pages, want 2", len(doc.Pages))
	}
	if strings.Contains(pageText(doc, 1), "ROW2") || !strings.Contains(pageText(doc, 2), "ROW2") {
		t.Fatalf("ROW2 was not moved to the second page: %q / %q", pageText(doc, 1), pageText(doc, 2))
	}

	// A trailing break-after on the last row must not create an empty page.
	doc, _ = h.runRaw(base+`<table><tr style="break-after:page"><td>ONLY</td></tr></table>`, size, margin)
	if len(doc.Pages) != 1 {
		t.Fatalf("a trailing row break produced %d pages, want 1", len(doc.Pages))
	}

	// break-after in the middle breaks before the next row and repeats the
	// header on the new page.
	doc, _ = h.runRaw(base+`<table><thead><tr><th>HEAD</th></tr></thead>
	<tbody><tr style="break-after:page"><td>A</td></tr><tr><td>B</td></tr></tbody></table>`, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("break-after on a row produced %d pages, want 2", len(doc.Pages))
	}
	if !strings.HasPrefix(pageText(doc, 2), "HEAD") || !strings.Contains(pageText(doc, 2), "B") {
		t.Fatalf("the second page does not repeat the header and continue the rows: %q", pageText(doc, 2))
	}
}

// TestTableGridLimitCountsRowspanShift guards RV2-28: MaxTableCells counted the
// widest row's colspan sum, but a rowspan pushes the following rows to the
// right, so a table could allocate a grid far wider than the limit allowed.
func TestTableGridLimitCountsRowspanShift(t *testing.T) {
	h := newHarness(t)
	h.ctx.MaxTableCells = 50
	size := css.PageSize{Width: 400000, Height: 400000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	var b strings.Builder
	b.WriteString(`<table>`)
	for r := 0; r < 5; r++ {
		b.WriteString("<tr>")
		for c := 0; c < 10; c++ {
			b.WriteString(`<td rowspan="100">x</td>`)
		}
		b.WriteString("</tr>")
	}
	b.WriteString(`</table><p>AFTER</p>`)

	_, diags := h.runRaw(b.String(), size, margin)
	if !hasError(diags, "resource/limit") {
		t.Fatalf("a table with a 50-column grid passed a 50-cell limit: %v", diags.Lines())
	}

	// A table that really fits the same limit still renders.
	doc, diags := h.runRaw(`<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`,
		css.PageSize{Width: 200000, Height: 200000}, [4]int32{10000, 10000, 10000, 10000})
	if hasError(diags, "resource/limit") {
		t.Fatalf("a 2x2 table was rejected: %v", diags.Lines())
	}
	if !strings.Contains(pageText(doc, 1), "d") {
		t.Fatalf("the small table did not render: %q", pageText(doc, 1))
	}
}

// TestTableBottomDecorationIsClipped guards RV5-43: finishFragment added the
// table's bottom padding and border to the fragment bottom and painted them
// without comparing with the page, so a 200 pt padding painted 220 pt of
// background on a 100 pt page with no diagnostic.
func TestTableBottomDecorationIsClipped(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 100000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	contentBottom := size.Height - margin[2]
	src := `<style>
		table { width: 100pt; padding-bottom: 200pt; background-color: #0000ff; border: 2pt solid #000000 }
		td { padding: 0 }
	</style><table><tr><td>ROW</td></tr></table>`
	doc, diags := h.runRaw(src, size, margin)
	if n := countOverflow(diags); n != 1 {
		t.Fatalf("the clipped table reported %d layout/overflow, want 1: %v", n, diags.Lines())
	}
	assertInsideContentBox(t, doc)
	blue := css.Color{B: 255, Set: true}
	fill := spanFill(doc.Pages[0], blue)
	if fill == nil {
		t.Fatal("the table background was not painted")
	}
	if bottom := fill.Y + fill.H; bottom != contentBottom {
		t.Fatalf("the table background ends at %d, want the page content bottom %d", bottom, contentBottom)
	}
	// The bottom border is clipped to the same edge instead of crossing it.
	if !hasFill(doc.Pages[0], css.Color{Set: true}, Rect{X: fill.X, Y: contentBottom - 2000, W: fill.W, H: 2000}) {
		t.Fatalf("the clipped bottom border is missing from %+v", *fill)
	}
}

// TestOrdinaryTableDecorationIsUnchanged guards the ordinary case of RV5-43: a
// bottom padding and border that fit are painted in full and report nothing.
func TestOrdinaryTableDecorationIsUnchanged(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 200000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	blue := css.Color{B: 255, Set: true}
	table := func(decoration string) (*Document, diag.List) {
		return h.runRaw(`<style>
			table { width: 100pt; background-color: #0000ff; `+decoration+` }
			td { padding: 0 }
		</style><table><tr><td>ROW</td></tr></table>`, size, margin)
	}
	plain, _ := table("")
	plainFill := spanFill(plain.Pages[0], blue)
	if plainFill == nil {
		t.Fatal("the plain table background was not painted")
	}
	doc, diags := table("padding-bottom: 2pt; border: 2pt solid #000000")
	if n := countOverflow(diags); n != 0 {
		t.Fatalf("an ordinary table reported %d layout/overflow: %v", n, diags.Lines())
	}
	fill := spanFill(doc.Pages[0], blue)
	if fill == nil {
		t.Fatal("the table background was not painted")
	}
	// The fragment background covers the whole border box: the row plus the
	// 2 pt padding and both 2 pt borders.
	if got, want := fill.H, plainFill.H+6000; got != want {
		t.Fatalf("the table background is %d high, want %d (row + 2pt padding + 2pt borders)", got, want)
	}
	if !hasFill(doc.Pages[0], css.Color{Set: true}, Rect{X: fill.X, Y: fill.Y + fill.H - 2000, W: fill.W, H: 2000}) {
		t.Fatalf("the bottom border is missing from %+v", *fill)
	}
}

// TestSplitTableKeepsDecorationPerPage guards the split case of RV5-43: only the
// last fragment owns the table's bottom border, and no fragment paints its
// decoration past the page content bottom.
func TestSplitTableKeepsDecorationPerPage(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 200000, Height: 90000}
	margin := [4]int32{10000, 10000, 10000, 10000}
	var rows strings.Builder
	for i := 0; i < 30; i++ {
		rows.WriteString("<tr><td>row " + itoa(i) + "</td></tr>")
	}
	src := `<style>
		table { width: 100pt; padding-bottom: 4pt; border: 2pt solid #000000; background-color: #0000ff }
		td { padding: 0; line-height: 12pt; font-size: 8pt }
	</style><table>` + rows.String() + `</table>`
	doc, diags := h.runRaw(src, size, margin)
	if len(doc.Pages) < 2 {
		t.Fatalf("the table did not split: %d page(s)", len(doc.Pages))
	}
	if n := countOverflow(diags); n != 0 {
		t.Fatalf("the split table reported %d layout/overflow: %v", n, diags.Lines())
	}
	assertInsideContentBox(t, doc)
	blue := css.Color{B: 255, Set: true}
	// A bottom border spans the table width and sits on the fragment's bottom
	// edge; the top border, painted on the first fragment, sits on its top edge.
	bottomPage := 0
	for _, page := range doc.Pages {
		fill := spanFill(page, blue)
		if fill == nil {
			t.Fatalf("page %d has no table background", page.Number)
		}
		for _, op := range page.Ops {
			if op.Kind != FillOpKind || op.Color != (css.Color{Set: true}) {
				continue
			}
			r := op.Rect
			if r.W != fill.W || r.H != 2000 || fill.H <= 2000 {
				continue
			}
			if r.Y+r.H == fill.Y+fill.H {
				if bottomPage != 0 {
					t.Fatalf("the table's bottom border is painted on pages %d and %d", bottomPage, page.Number)
				}
				bottomPage = page.Number
			}
		}
	}
	if bottomPage != len(doc.Pages) {
		t.Fatalf("the bottom border belongs to page %d, want the last page %d", bottomPage, len(doc.Pages))
	}
}

// TestEmptyTableDoesNotConsumeForcedBreak guards RV5-18: boxHasContent answered
// from a table's row count alone, so an empty table counted as page body and
// consumed a pending break-before, appending a blank page after the paragraph.
func TestEmptyTableDoesNotConsumeForcedBreak(t *testing.T) {
	h := newHarness(t)
	size, margin := forcedBreakPage()
	src := `<style>p { margin: 0 }</style>
	<p>A</p>
	<table style="break-before: page"><tr><td></td></tr></table>`
	doc := h.runRawOrFail(src, size, margin)
	if len(doc.Pages) != 1 {
		t.Fatalf("the empty table produced %d pages, want 1", len(doc.Pages))
	}
	if got := pageText(doc, 1); !strings.Contains(got, "A") {
		t.Fatalf("page 1 = %q, want A", got)
	}
	// An empty table is not page body either, so the first content after it
	// still takes its own break on an otherwise fresh page.
	src = `<style>p { margin: 0 }</style>
	<table><tr><td></td></tr></table>
	<p style="break-before: page">B</p>`
	doc = h.runRawOrFail(src, size, margin)
	if len(doc.Pages) != 1 {
		t.Fatalf("the empty table counted as page body and produced %d pages, want 1", len(doc.Pages))
	}
	if got := pageText(doc, 1); !strings.Contains(got, "B") {
		t.Fatalf("page 1 = %q, want B", got)
	}
}

// TestHeightOnlyTableConsumesForcedBreak guards the other half of RV5-18: a
// table that paints only a height and a background did not count as page body,
// so the pending break-after was consumed by the paragraph that followed and the
// table stayed on the page the break was leaving.
func TestHeightOnlyTableConsumesForcedBreak(t *testing.T) {
	h := newHarness(t)
	size, margin := forcedBreakPage()
	src := `<style>p { margin: 0 }</style>
	<p style="break-after: page">A</p>
	<table style="height: 50pt; background-color: #ff0000"></table>
	<p>B</p>`
	doc := h.runRawOrFail(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("the height-only table produced %d pages, want 2", len(doc.Pages))
	}
	if got := pageText(doc, 1); !strings.Contains(got, "A") || strings.Contains(got, "B") {
		t.Fatalf("page 1 = %q, want only A", got)
	}
	if got := pageText(doc, 2); !strings.Contains(got, "B") {
		t.Fatalf("page 2 = %q, want B", got)
	}
	// The table itself is painted on the second page, at its top and with the
	// declared height, before the paragraph that follows it.
	found := false
	for _, op := range doc.Pages[1].Ops {
		if op.Kind != FillOpKind || op.Color != (css.Color{R: 255, Set: true}) {
			continue
		}
		found = true
		if op.Rect.Y != margin[0] || op.Rect.H != 50000 {
			t.Fatalf("the moved table background is %+v, want Y=%d H=50000", op.Rect, margin[0])
		}
	}
	if !found {
		t.Fatal("the moved table was not painted on the second page")
	}
}

// TestTextRowTableConsumesForcedBreak guards the third delta scenario: a table
// with visible cell text is page content and takes a pending break as before.
func TestTextRowTableConsumesForcedBreak(t *testing.T) {
	h := newHarness(t)
	size, margin := forcedBreakPage()
	src := `<style>p { margin: 0 }</style>
	<p style="break-after: page">A</p>
	<table><tr><td>T</td></tr></table>
	<p>B</p>`
	doc := h.runRawOrFail(src, size, margin)
	if len(doc.Pages) != 2 {
		t.Fatalf("the table with text produced %d pages, want 2", len(doc.Pages))
	}
	if got := pageText(doc, 1); !strings.Contains(got, "A") || strings.Contains(got, "T") || strings.Contains(got, "B") {
		t.Fatalf("page 1 = %q, want only A", got)
	}
	if got := pageText(doc, 2); !strings.Contains(got, "T") || !strings.Contains(got, "B") {
		t.Fatalf("page 2 = %q, want T and B", got)
	}
}

// findBoxKind returns the first box of the given kind in the tree, including
// the cells of every table.
func findBoxKind(b *Box, kind Kind) *Box {
	if b.Kind == kind {
		return b
	}
	for _, row := range b.Rows {
		for _, cell := range row.Cells {
			if got := findBoxKind(cell, kind); got != nil {
				return got
			}
		}
	}
	for _, c := range b.Children {
		if got := findBoxKind(c, kind); got != nil {
			return got
		}
	}
	return nil
}

// TestTableBoxHasContent guards the break predicate itself (RV5-18): a table
// counts only when it can paint something, so an empty table answers false and
// a table whose own height and background paint answers true.
func TestTableBoxHasContent(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"empty", `<table><tr><td></td></tr></table>`, false},
		{"height and background", `<table style="height:50pt;background-color:#ff0000"></table>`, true},
		{"text row", `<table><tr><td>T</td></tr></table>`, true},
		{"row background", `<table><tr style="background-color:#ff0000"><td></td></tr></table>`, true},
		{"cell border", `<table><tr><td style="border:1pt solid #000000"></td></tr></table>`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := findBoxKind(h.buildTree(tc.src), Table)
			if table == nil {
				t.Fatalf("%s: the table box is missing", tc.src)
			}
			if got := boxHasContent(table); got != tc.want {
				t.Fatalf("boxHasContent(%s) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}
