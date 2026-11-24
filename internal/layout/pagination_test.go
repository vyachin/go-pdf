package layout

import (
	"os"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/profile"
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

func TestPaginationStopsWithoutProgress(t *testing.T) {
	h := newHarness(t)
	size := css.PageSize{Width: 100000, Height: 100000} // 100pt square
	margin := [4]int32{10000, 10000, 10000, 10000}
	src := `<style>
		@page { size: 100pt 100pt; margin: 10pt }
		p { font-size: 10pt; line-height: 100pt; margin: 0 }
	</style><p>IMPORTANT</p>`
	doc, diags := h.runRaw(src, size, margin)
	if !hasError(diags, "layout/no-progress") {
		t.Fatalf("expected layout/no-progress, got %v", diags.Lines())
	}
	if len(doc.Pages) > 2 {
		t.Fatalf("pagination produced %d pages before stopping", len(doc.Pages))
	}
	if len(doc.Pages) > 0 && len(doc.Pages[0].Ops) > 0 {
		t.Fatal("the overflowing line must not be reported as placed")
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
	sheets, _ := profile.CollectStylesheets(doc, profile.Options{})
	cascade := css.NewCascade(append([]*css.Stylesheet{css.DefaultStylesheet()}, sheets...)...)
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
