package html

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/diag"
)

func parseDump(t *testing.T, src string) string {
	t.Helper()
	doc, err := Parse([]byte(src), "test.html")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return Dump(doc.Root)
}

func TestParseTree(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "implicit html head body",
			input: `<p>hello</p>`,
			want: `#document
  <html>
    <head>
    <body>
      <p>
        "hello"
`,
		},
		{
			name:  "title goes to head",
			input: `<title>T</title><p>x</p>`,
			want: `#document
  <html>
    <head>
      <title>
        "T"
    <body>
      <p>
        "x"
`,
		},
		{
			name:  "implicit paragraph close",
			input: `<p>a<p>b`,
			want: `#document
  <html>
    <head>
    <body>
      <p>
        "a"
      <p>
        "b"
`,
		},
		{
			name:  "implicit list item close",
			input: `<ul><li>a<li>b</ul>`,
			want: `#document
  <html>
    <head>
    <body>
      <ul>
        <li>
          "a"
        <li>
          "b"
`,
		},
		{
			name:  "end tag closes intermediate elements",
			input: `<div><p>x</div>`,
			want: `#document
  <html>
    <head>
    <body>
      <div>
        <p>
          "x"
`,
		},
		{
			name:  "table text is foster parented and the row group is implied",
			input: `<table>stray<tr><td>a</td></tr></table>`,
			want: `#document
  <html>
    <head>
    <body>
      "stray"
      <table>
        <tbody>
          <tr>
            <td>
              "a"
`,
		},
		{
			name:  "markup inside a title is text",
			input: `<title>a<b>c</title>`,
			want: `#document
  <html>
    <head>
      <title>
        "a<b>c"
    <body>
`,
		},
		{
			name:  "script text stays inside the script",
			input: `<script>var x = 1</script><p>a</p>`,
			want: `#document
  <html>
    <head>
      <script>
        "var x = 1"
    <body>
      <p>
        "a"
`,
		},
		{
			name:  "explicit head and body",
			input: `<html><head><title>T</title></head><body class="b"><h1>H</h1></body></html>`,
			want: `#document
  <html>
    <head>
      <title>
        "T"
    <body class="b">
      <h1>
        "H"
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDump(t, tc.input)
			if got != tc.want {
				t.Fatalf("tree mismatch\ngot:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

// TestCharacterReferences pins the decoding rules the previous hand-written
// table did not implement: names outside the common subset, legacy names
// without a semicolon, windows-1252 numeric references, and unknown names left
// as written so they stay visible.
func TestCharacterReferences(t *testing.T) {
	doc, err := Parse([]byte(`<p>&rarr; &nbsp x &#147; &nosuchentity;</p>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	text := doc.Body().FirstElementChild().TextContent()
	for _, want := range []string{"\u2192", "\u00a0", "\u201c", "&nosuchentity;"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text %q does not contain %q", text, want)
		}
	}
}

func TestPositions(t *testing.T) {
	src := "<p>\nhello</p>"
	doc, err := Parse([]byte(src), "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	body := doc.Body()
	if body == nil {
		t.Fatal("no body")
	}
	p := body.FirstElementChild()
	if p == nil || p.Tag != "p" {
		t.Fatalf("expected p, got %+v", p)
	}
	pos := doc.Pos(p)
	if pos.File != "doc.html" || pos.Line != 1 || pos.Col != 1 {
		t.Fatalf("p position = %+v, want doc.html:1:1", pos)
	}
	text := p.Children[0]
	if text.Type != TextNode {
		t.Fatalf("first child is not text: %+v", text)
	}
	// The character data starts right after the tag, so it keeps the newline.
	if want := strings.Index(src, "\n"); text.Start != want || text.Data != "\nhello" {
		t.Fatalf("text = %q at %d, want %q at %d", text.Data, text.Start, "\nhello", want)
	}
	if got := doc.Position(strings.Index(src, "hello")); got.Line != 2 || got.Col != 1 {
		t.Fatalf("text position = %+v, want line 2 col 1", got)
	}
}

// TestImpliedNodesInheritPosition covers the position rule for elements the tree
// construction creates itself: they have no start tag, so they carry the range
// of their nearest positioned ancestor.
func TestImpliedNodesInheritPosition(t *testing.T) {
	doc, err := Parse([]byte(`<p>a</p><table><tr><td>x</td></tr></table>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	table := findElement(doc.Root, "table")
	if table == nil || table.Start == 0 {
		t.Fatalf("table position = %+v, want a position after the paragraph", table)
	}
	tbody := findElement(table, "tbody")
	if tbody == nil {
		t.Fatal("no implied tbody")
	}
	if tbody.Start != table.Start || tbody.End != table.End {
		t.Fatalf("tbody range = %d..%d, want the table's %d..%d", tbody.Start, tbody.End, table.Start, table.End)
	}
}

// TestAuthoredElementsPointAtTheirStartTag checks the strongest form of the
// position rule on the two corpus documents whose standard tree reorders
// content: an element the document actually wrote must point at its own start
// tag, not at an ancestor and not at a negative offset.
func TestAuthoredElementsPointAtTheirStartTag(t *testing.T) {
	cases := readCorpusCases(t, "Issue532Test_testundefinedindext", "list-style-type-snapshot")
	if len(cases) != 2 {
		t.Fatalf("found %d corpus fixtures, want 2", len(cases))
	}
	implied := map[string]bool{"html": true, "head": true, "body": true, "tbody": true, "thead": true, "tfoot": true}
	for _, tc := range cases {
		src := []byte(tc.HTML)
		doc, err := Parse(src, tc.Name+".html")
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		checked := 0
		walk(doc.Root, func(n *Node) {
			if n.Type != ElementNode {
				return
			}
			if n.Start < 0 || n.End > len(src) || n.Start > n.End {
				t.Fatalf("%s: <%s> has range %d..%d outside the %d-byte document", tc.Name, n.Tag, n.Start, n.End, len(src))
			}
			if implied[n.Tag] {
				return
			}
			checked++
			if !startsWithTag(src[n.Start:n.End], n.Tag) {
				t.Fatalf("%s: <%s> at %d..%d does not point at its start tag: %q", tc.Name, n.Tag, n.Start, n.End, snippet(src[n.Start:n.End]))
			}
		})
		if checked == 0 {
			t.Fatalf("%s: no element was checked", tc.Name)
		}
	}
}

// startsWithTag reports whether b begins with a start tag of the given name,
// ignoring case, so "<TD>" also matches td and "<tdx" does not.
func startsWithTag(b []byte, tag string) bool {
	if len(b) < len(tag)+1 || b[0] != '<' {
		return false
	}
	if !strings.EqualFold(string(b[1:len(tag)+1]), tag) {
		if tag == "img" && strings.EqualFold(string(b[1:5]), "image") {
			return true
		}
		return false
	}
	if len(b) == len(tag)+1 {
		return true
	}
	switch b[len(tag)+1] {
	case ' ', '\t', '\n', '\r', '\f', '>', '/':
		return true
	}
	return false
}

func snippet(b []byte) string {
	if len(b) > 40 {
		return string(b[:40]) + "..."
	}
	return string(b)
}

type corpusCase struct {
	Name string `json:"name"`
	HTML string `json:"html"`
}

// readCorpusCases loads the mPDF fixtures whose names contain one of the given
// substrings. The corpus keeps its documents in cases.json rather than in
// separate files.
func readCorpusCases(t *testing.T, names ...string) []corpusCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "mpdf", "cases.json"))
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var corpus struct {
		Cases []corpusCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	var out []corpusCase
	for _, name := range names {
		found := false
		for _, c := range corpus.Cases {
			if strings.Contains(c.Name, name) && !found {
				out = append(out, c)
				found = true
			}
		}
		if !found {
			t.Fatalf("no corpus fixture matches %q", name)
		}
	}
	return out
}

func walk(n *Node, fn func(*Node)) {
	fn(n)
	for _, c := range n.Children {
		walk(c, fn)
	}
}

func TestElementPath(t *testing.T) {
	doc, err := Parse([]byte(`<body><div class="invoice"><p id="total">x</p></div></body>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	p := doc.FindByID("total")
	if p == nil {
		t.Fatal("no element with id total")
	}
	if got, want := p.Path(), "body > div.invoice > p#total"; got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestTextContent(t *testing.T) {
	doc, err := Parse([]byte(`<p>a <strong>b</strong> c</p>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Body().FirstElementChild()
	if got, want := p.TextContent(), "a b c"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
}

// TestDecodedDocumentPosition checks that a decoded document still reports
// positions in the bytes of the original file.
func TestDecodedDocumentPosition(t *testing.T) {
	// "<meta charset=\"windows-1251\"><p>Привет <b>x</b>" with the word in
	// windows-1251: one source byte per letter, two decoded bytes.
	src := []byte("<meta charset=\"windows-1251\"><p>\xcf\xf0\xe8\xe2\xe5\xf2 <b>x</b>")
	doc, err := Parse(src, "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	var b *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == ElementNode && n.Tag == "b" {
			b = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc.Root)
	if b == nil {
		t.Fatal("no <b> element")
	}
	// Decoded: 29 + 3 + 12 + 1 bytes before it. Source: 29 + 3 + 6 + 1.
	if b.Start != 45 {
		t.Fatalf("start = %d, want the decoded offset 45", b.Start)
	}
	want := diag.Position{File: "doc.html", Line: 1, Col: 40}
	if got := doc.Pos(b); got != want {
		t.Fatalf("position = %+v, want %+v (a decoded column would be 46)", got, want)
	}
	if len(doc.Source()) != len(src) {
		t.Fatalf("Source() returned %d bytes, want the original %d", len(doc.Source()), len(src))
	}
	if got := doc.Charset(); got == nil || got.Name() != "windows-1251" {
		t.Fatalf("Charset() = %v", got)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("<p>hi</p>"))
	f.Add([]byte("<table><tr><td>x</table>"))
	f.Add([]byte("<!-- unterminated"))
	f.Add([]byte("<p class=\"a\" id='b'>x &amp; y</p>"))
	f.Fuzz(func(t *testing.T, b []byte) {
		// Arbitrary bytes end in a document or in one of the two documented
		// rejections: input that stops inside a tag, and bytes that are not a
		// decodable document. Anything else is a defect.
		_, err := Parse(b, "fuzz.html")
		var truncated *TruncatedError
		var encoding *charset.Error
		if err != nil && !errors.As(err, &truncated) && !errors.As(err, &encoding) {
			t.Fatalf("Parse returned an unexpected error: %v", err)
		}
	})
}

func TestHTMLAttributesSurvive(t *testing.T) {
	doc, err := Parse([]byte(`<html lang="ru" class="root" style="color: red"><body><p>x</p></body></html>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	node := doc.HTML()
	if node == nil {
		t.Fatal("no html element")
	}
	if v, _ := node.Attr("lang"); v != "ru" {
		t.Fatalf("lang = %q", v)
	}
	if !node.HasClass("root") {
		t.Fatal("class attribute was lost")
	}
	if v, _ := node.Attr("style"); v != "color: red" {
		t.Fatalf("style = %q", v)
	}
	// An implicit html element must not borrow attributes from the first tag.
	doc, err = Parse([]byte(`<p style="color: red">x</p>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.HTML().Attrs) != 0 {
		t.Fatalf("implicit html borrowed attributes: %+v", doc.HTML().Attrs)
	}
}

func findByID(n *Node, id string) *Node {
	if n.Type == ElementNode {
		if v, ok := n.Attr("id"); ok && v == id {
			return n
		}
	}
	for _, c := range n.Children {
		if found := findByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func ancestorTag(n *Node, tag string) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Tag == tag {
			return true
		}
	}
	return false
}

func TestNestedTableKeepsItsStructure(t *testing.T) {
	doc, err := Parse([]byte(`<table id="outer"><tr><td><table id="inner"><tr><td>INNER</td></tr></table></td></tr></table>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	outer := findByID(doc.Root, "outer")
	inner := findByID(doc.Root, "inner")
	if outer == nil || inner == nil {
		t.Fatal("tables are missing")
	}
	if inner.Parent == nil || inner.Parent.Tag != "td" {
		t.Fatalf("inner table parent = %v, want td", inner.Parent)
	}
	if !ancestorTag(inner, "table") {
		t.Fatal("inner table is not inside the outer table")
	}
	innerRow := findElement(inner, "tr")
	if innerRow == nil || !strings.Contains(innerRow.TextContent(), "INNER") {
		t.Fatalf("inner row text = %q", innerRow.TextContent())
	}
	// The outer table must have exactly one own row; the inner row is not its.
	if rows := countTag(outer, "tr"); rows != 2 { // outer row + inner row
		t.Fatalf("outer table has %d tr nodes, want 2 (one own, one nested)", rows)
	}
	outerRow := findElement(outer, "tr")
	if outerRow == nil || len(outerRow.Children) == 0 || outerRow.Children[0].Tag != "td" {
		t.Fatalf("outer row = %+v", outerRow)
	}
}

func countTag(n *Node, tag string) int {
	count := 0
	if n.Type == ElementNode && n.Tag == tag {
		count++
	}
	for _, c := range n.Children {
		count += countTag(c, tag)
	}
	return count
}

func TestNestedListKeepsItsStructure(t *testing.T) {
	doc, err := Parse([]byte(`<ol><li id="outer">A<ol><li id="inner">B</li></ol>C</li></ol>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	outer := findByID(doc.Root, "outer")
	inner := findByID(doc.Root, "inner")
	if outer == nil || inner == nil {
		t.Fatal("list items are missing")
	}
	if inner.Parent == nil || inner.Parent.Tag != "ol" {
		t.Fatalf("inner item parent = %v, want ol", inner.Parent)
	}
	if inner.Parent.Parent != outer {
		t.Fatal("the inner list is not inside the outer item")
	}
	if text := outer.TextContent(); !strings.Contains(text, "C") {
		t.Fatalf("text after the nested list left the outer item: %q", text)
	}
}

func TestParseRejectsDeepNesting(t *testing.T) {
	src := []byte(strings.Repeat("<div>", 20) + "x")
	_, err := ParseWithOptions(src, ParseOptions{MaxDepth: 8})
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("deep nesting returned %v, want a budget limit error", err)
	}
	// A document at the limit still parses: html, body and one element.
	if _, err := ParseWithOptions([]byte(`<div><p>x</p></div>`), ParseOptions{MaxDepth: 4}); err != nil {
		t.Fatalf("a document at the depth limit failed: %v", err)
	}
}

func TestParseStopsOnExhaustedBudget(t *testing.T) {
	src := []byte(strings.Repeat("<p>строка</p>", 200))
	_, err := ParseWithOptions(src, ParseOptions{Budget: budget.New(context.Background(), 5)})
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("parse returned %v, want a budget limit error", err)
	}
}

func TestParseStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ParseWithOptions([]byte(strings.Repeat("<p>x</p>", 50)), ParseOptions{Budget: budget.New(ctx, 0)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("parse returned %v, want context.Canceled", err)
	}
}

func TestFosterParentKeepsChildContent(t *testing.T) {
	doc, err := Parse([]byte(`<table><div style="display:none">SECRET</div><tr><td>CELL</td></tr></table>`), "foster.html")
	if err != nil {
		t.Fatal(err)
	}
	div := findElement(doc.Root, "div")
	if div == nil {
		t.Fatal("the fostered div is missing")
	}
	if got := div.TextContent(); got != "SECRET" {
		t.Fatalf("the fostered div contains %q, want %q", got, "SECRET")
	}
	if style, _ := div.Attr("style"); style != "display:none" {
		t.Fatalf("the fostered div lost its style: %q", style)
	}
	if body := findElement(doc.Root, "body"); body != nil && strings.Contains(body.TextContent(), "SECRETSECRET") {
		t.Fatal("the text was duplicated next to the table")
	}

	doc, err = Parse([]byte(`<table><div><b>B</b></div><tr><td>CELL</td></tr></table>`), "foster2.html")
	if err != nil {
		t.Fatal(err)
	}
	div = findElement(doc.Root, "div")
	if div == nil || findElement(div, "b") == nil {
		t.Fatalf("the bold element is not inside the fostered div: %s", Dump(doc.Root))
	}

	// A table-structure tag returns to the table context: the fostered element
	// is closed and the row belongs to the table.
	doc, err = Parse([]byte(`<table><div>x</div><tr><td>y</td></tr></table>`), "foster3.html")
	if err != nil {
		t.Fatal(err)
	}
	table := findElement(doc.Root, "table")
	if table == nil || findElement(table, "tr") == nil {
		t.Fatalf("the row is not inside the table: %s", Dump(doc.Root))
	}
	if div := findElement(doc.Root, "div"); div == nil || div.TextContent() != "x" {
		t.Fatalf("the fostered div was lost: %s", Dump(doc.Root))
	}
}

func findElement(n *Node, tag string) *Node {
	if n.Type == ElementNode && n.Tag == tag {
		return n
	}
	for _, c := range n.Children {
		if got := findElement(c, tag); got != nil {
			return got
		}
	}
	return nil
}

// TestNestedTableStartClosesOuterTable guards RV2-08: a <table> start tag
// inside table structure must close the outer table and open a sibling instead
// of becoming a child that layout silently drops.
func TestNestedTableStartClosesOuterTable(t *testing.T) {
	doc, err := Parse([]byte(`<p>BEFORE</p><table><tr><td>A</td></tr><table><tr><td>B</td></tr></table></table><p>AFTER</p>`), "nested.html")
	if err != nil {
		t.Fatal(err)
	}
	body := findElement(doc.Root, "body")
	if body == nil {
		t.Fatal("no body")
	}
	tables := 0
	for _, c := range body.Children {
		if c.Type == ElementNode && c.Tag == "table" {
			tables++
			for _, child := range c.Children {
				if inner := findElement(child, "table"); inner != nil {
					t.Fatalf("a table is nested inside another table:\n%s", Dump(doc.Root))
				}
			}
		}
	}
	if tables != 2 {
		t.Fatalf("body has %d top-level tables, want 2:\n%s", tables, Dump(doc.Root))
	}
	var cells []string
	for _, c := range body.Children {
		if c.Type == ElementNode && c.Tag == "table" {
			cells = append(cells, c.TextContent())
		}
	}
	if len(cells) != 2 || cells[0] != "A" || cells[1] != "B" {
		t.Fatalf("table cells = %q, want [A B]", cells)
	}
}

// TestHeadingStartClosesCurrentHeading guards RV2-30: a heading start tag left
// the previous heading open, so "<h1>Head<h2>Sub" nested the second heading in
// the first instead of closing it.
func TestHeadingStartClosesCurrentHeading(t *testing.T) {
	doc, err := Parse([]byte(`<h1>Head<h2>Sub</h3>`), "headings.html")
	if err != nil {
		t.Fatal(err)
	}
	body := findElement(doc.Root, "body")
	if body == nil {
		t.Fatal("no body")
	}
	var tags []string
	for _, c := range body.Children {
		if c.Type == ElementNode {
			tags = append(tags, c.Tag)
		}
	}
	if len(tags) != 2 || tags[0] != "h1" || tags[1] != "h2" {
		t.Fatalf("body children = %v, want [h1 h2] as siblings:\n%s", tags, Dump(doc.Root))
	}
	if h1 := findElement(body, "h1"); h1 == nil || findElement(h1, "h2") != nil {
		t.Fatalf("the second heading is nested inside the first:\n%s", Dump(doc.Root))
	}

	// A heading inside an element is not closed by a sibling heading of the
	// outer heading: only the current node matters.
	doc, err = Parse([]byte(`<h1><div>text<h2>Sub</h2></div></h1>`), "headings2.html")
	if err != nil {
		t.Fatal(err)
	}
	h1 := findElement(doc.Root, "h1")
	if h1 == nil || findElement(h1, "h2") == nil {
		t.Fatalf("the nested heading was closed too early:\n%s", Dump(doc.Root))
	}
}

// TestTableWhitespaceIgnored covers the normalization: indentation between rows
// must not become a node that layout could place, while white space inside a
// cell is content and stays.
func TestTableWhitespaceIgnored(t *testing.T) {
	indented := "<table>\n  <tbody>\n    <tr>\n      <td>x</td>\n    </tr>\n  </tbody>\n</table>"
	oneLine := "<table><tbody><tr><td>x</td></tr></tbody></table>"
	if a, b := parseDump(t, indented), parseDump(t, oneLine); a != b {
		t.Fatalf("indented and one-line tables differ:\n%s\nwant:\n%s", a, b)
	}

	doc, err := Parse([]byte("<table><tr><td> <b>x</b> </td></tr></table>"), "ws.html")
	if err != nil {
		t.Fatal(err)
	}
	td := findElement(doc.Root, "td")
	if td == nil || len(td.Children) != 3 {
		t.Fatalf("cell children = %+v, want text, element, text", td.Children)
	}
}

// TestNonVoidSelfClosingIsIgnored covers the one deliberate behaviour change:
// the standard ignores "/>" on a non-void element, so the element stays open.
func TestNonVoidSelfClosingIsIgnored(t *testing.T) {
	doc, err := Parse([]byte(`<div/>text`), "sc.html")
	if err != nil {
		t.Fatal(err)
	}
	div := findElement(doc.Root, "div")
	if div == nil {
		t.Fatal("no div")
	}
	if got := div.TextContent(); got != "text" {
		t.Fatalf("div content = %q, want %q", got, "text")
	}
	if len(div.ElementChildren()) != 0 {
		t.Fatalf("div has element children: %+v", div.ElementChildren())
	}

	// A void element still closes and takes no content.
	doc, err = Parse([]byte(`<p>a<br/>b</p>`), "sc2.html")
	if err != nil {
		t.Fatal(err)
	}
	p := findElement(doc.Root, "p")
	if p == nil || p.TextContent() != "ab" {
		t.Fatalf("p content = %q, want %q", p.TextContent(), "ab")
	}
}

// TestStyleTextIsRawSource covers the offset contract for inline CSS: the text
// node's range must slice the document source verbatim, including CRLF, because
// the CSS parser maps its own offsets back through that range.
func TestStyleTextIsRawSource(t *testing.T) {
	src := []byte("<style>\r\np { color: red }\r\n</style><p>x</p>")
	doc, err := Parse(src, "crlf.html")
	if err != nil {
		t.Fatal(err)
	}
	style := findElement(doc.Root, "style")
	if style == nil || len(style.Children) == 0 {
		t.Fatal("no style element with content")
	}
	text := style.Children[0]
	want := "\r\np { color: red }\r\n"
	if got := string(doc.Source()[text.Start:text.End]); got != want {
		t.Fatalf("style text range holds %q, want %q", got, want)
	}
	// The CSS parser adds this node's start to its own offsets.
	idx := strings.Index(want, "color")
	pos := doc.Position(text.Start + idx)
	if pos.Line != 2 || pos.Col != 5 {
		t.Fatalf("inline CSS offset %d maps to %+v, want line 2 col 5", idx, pos)
	}
}

// TestConversionPreservesModel checks the four properties conversion must keep:
// parent links, lower-cased attribute keys, namespace prefixes written back as
// "prefix:name", and adjacent text merged into one node.
func TestConversionPreservesModel(t *testing.T) {
	doc, err := Parse([]byte(`<P CLASS="x">a</P><svg><a xlink:href="#y">b</a></svg>`), "model.html")
	if err != nil {
		t.Fatal(err)
	}
	p := findElement(doc.Root, "p")
	if p == nil {
		t.Fatal("the upper-case tag was not lower-cased")
	}
	if v, ok := p.Attr("class"); !ok || v != "x" {
		t.Fatalf("class attribute = %q, %v", v, ok)
	}
	svg := findElement(doc.Root, "svg")
	if svg == nil {
		t.Fatal("no svg element")
	}
	a := findElement(svg, "a")
	if a == nil {
		t.Fatal("no anchor inside svg")
	}
	if v, ok := a.Attr("xlink:href"); !ok || v != "#y" {
		t.Fatalf("namespaced attribute = %q, %v; want xlink:href", v, ok)
	}
	walk(doc.Root, func(n *Node) {
		for _, c := range n.Children {
			if c.Parent != n {
				t.Fatalf("<%s> child <%s> has parent %p, want %p", n.Tag, c.Tag, c.Parent, n)
			}
		}
	})

	// Content fostered out of a table lands next to the table as consecutive
	// text nodes; the model holds one node for the run.
	doc, err = Parse([]byte(`<table>a<tr><td>x</td></tr>b</table>`), "model2.html")
	if err != nil {
		t.Fatal(err)
	}
	body := doc.Body()
	texts := 0
	var merged string
	for _, c := range body.Children {
		if c.Type == TextNode {
			texts++
			merged += c.Data
		}
	}
	if texts != 1 || merged != "ab" {
		t.Fatalf("fostered text = %d node(s) %q, want one node %q", texts, merged, "ab")
	}
}

// TestTruncatedMarkupIsReported covers the one place where the standard tree
// would silently shorten a document: an incomplete tag at the end of the input
// is dropped by the tree construction, so the parse fails instead.
func TestTruncatedMarkupIsReported(t *testing.T) {
	for _, src := range []string{"<p>a</p><div", `<img src="`, "a<b", "</p"} {
		var trunc *TruncatedError
		_, err := Parse([]byte(src), "cut.html")
		if !errors.As(err, &trunc) {
			t.Fatalf("Parse(%q) = %v, want a TruncatedError", src, err)
		}
		if want := strings.LastIndex(src, "<"); trunc.Offset != want {
			t.Fatalf("Parse(%q) reports offset %d, want %d", src, trunc.Offset, want)
		}
		if trunc.Position.File != "cut.html" || trunc.Position.Line != 1 {
			t.Fatalf("Parse(%q) position = %+v, want cut.html:1", src, trunc.Position)
		}
	}

	// Input that ends between tokens, including a lone '<', is a complete
	// document: the '<' is text and nothing is dropped.
	for _, src := range []string{"<p>a</p>", "5 <", "<", "<!-- note"} {
		if _, err := Parse([]byte(src), "ok.html"); err != nil {
			t.Fatalf("Parse(%q) = %v, want no error", src, err)
		}
	}
}
