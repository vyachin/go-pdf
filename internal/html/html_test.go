package html

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/budget"
)

func TestTokenizer(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []Token
	}{
		{
			name:  "start tag with attributes",
			input: `<p class="a" id='b'>`,
			want: []Token{
				{Kind: StartTagToken, Name: "p", Attrs: []Attr{{"class", "a"}, {"id", "b"}}},
			},
		},
		{
			name:  "text with entities",
			input: `x &amp; y &#1057;`,
			want: []Token{
				{Kind: TextToken, Data: "x & y С"},
			},
		},
		{
			name:  "self closing void element",
			input: `<br/>`,
			want: []Token{
				{Kind: StartTagToken, Name: "br", SelfClosing: true},
			},
		},
		{
			name:  "comment",
			input: `<!-- note -->`,
			want: []Token{
				{Kind: CommentToken, Data: " note "},
			},
		},
		{
			name:  "doctype",
			input: `<!DOCTYPE html>`,
			want: []Token{
				{Kind: DoctypeToken, Data: "html"},
			},
		},
		{
			name:  "raw text element",
			input: `<script>if (a<b) {}</script>`,
			want: []Token{
				{Kind: StartTagToken, Name: "script"},
				{Kind: TextToken, Data: "if (a<b) {}"},
				{Kind: EndTagToken, Name: "script"},
			},
		},
		{
			name:  "raw text end tag is case insensitive",
			input: `<script>a</SCRIPT>`,
			want: []Token{
				{Kind: StartTagToken, Name: "script"},
				{Kind: TextToken, Data: "a"},
				{Kind: EndTagToken, Name: "script"},
			},
		},
		{
			name:  "stray less than is text",
			input: `a < b`,
			want: []Token{
				{Kind: TextToken, Data: "a "},
				{Kind: TextToken, Data: "<"},
				{Kind: TextToken, Data: " b"},
			},
		},
		{
			name:  "unquoted attribute value",
			input: `<td colspan=2>`,
			want: []Token{
				{Kind: StartTagToken, Name: "td", Attrs: []Attr{{"colspan", "2"}}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			z := NewTokenizer([]byte(tc.input))
			var got []Token
			for {
				tok := z.Next()
				if tok.Kind == EOFToken {
					break
				}
				got = append(got, Token{Kind: tok.Kind, Name: tok.Name, Attrs: tok.Attrs, Data: tok.Data, SelfClosing: tok.SelfClosing})
				if tok.Kind == StartTagToken && (tok.Name == "script" || tok.Name == "style") {
					z.SetRawTag(tok.Name)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d tokens, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].Kind != tc.want[i].Kind || got[i].Name != tc.want[i].Name || got[i].Data != tc.want[i].Data || got[i].SelfClosing != tc.want[i].SelfClosing {
					t.Fatalf("token %d = %+v, want %+v", i, got[i], tc.want[i])
				}
				if len(got[i].Attrs) != len(tc.want[i].Attrs) {
					t.Fatalf("token %d attrs = %+v, want %+v", i, got[i].Attrs, tc.want[i].Attrs)
				}
				for j := range got[i].Attrs {
					if got[i].Attrs[j] != tc.want[i].Attrs[j] {
						t.Fatalf("token %d attr %d = %+v, want %+v", i, j, got[i].Attrs[j], tc.want[i].Attrs[j])
					}
				}
			}
		})
	}
}

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
			name:  "table text is foster parented",
			input: `<table>stray<tr><td>a</td></tr></table>`,
			want: `#document
  <html>
    <head>
    <body>
      "stray"
      <table>
        <tr>
          <td>
            "a"
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
	textPos := doc.Position(strings.Index(src, "hello"))
	if textPos.Line != 2 || textPos.Col != 1 {
		t.Fatalf("text position = %+v, want line 2 col 1", textPos)
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

func FuzzParse(f *testing.F) {
	f.Add([]byte("<p>hi</p>"))
	f.Add([]byte("<table><tr><td>x</table>"))
	f.Add([]byte("<!-- unterminated"))
	f.Add([]byte("<p class=\"a\" id='b'>x &amp; y</p>"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if _, err := Parse(b, "fuzz.html"); err != nil {
			t.Fatalf("Parse returned an error: %v", err)
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
	if !ancestorTag(inner, "table") || inner.Parent.Parent == nil {
		t.Fatal("inner table is not inside the outer table")
	}
	if got := len(inner.ElementChildren()); got != 1 || inner.ElementChildren()[0].Tag != "tr" {
		t.Fatalf("inner table children = %v", inner.ElementChildren())
	}
	innerRow := inner.ElementChildren()[0]
	if !strings.Contains(innerRow.TextContent(), "INNER") {
		t.Fatalf("inner row text = %q", innerRow.TextContent())
	}
	// The outer table must have exactly one row; the inner row is not its.
	if rows := countTag(outer, "tr"); rows != 2 { // outer row + inner row
		t.Fatalf("outer table has %d tr nodes, want 2 (one own, one nested)", rows)
	}
	if outerRow := outer.ElementChildren()[0]; outerRow.Tag != "tr" || len(outerRow.ElementChildren()) != 1 {
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

func TestDecodeEntitiesLeavesUnterminatedAmpersands(t *testing.T) {
	src := strings.Repeat("&", 1<<20)
	if got := decodeEntities(src); got != src {
		t.Fatalf("unterminated ampersands changed: %d bytes out, want %d", len(got), len(src))
	}
}

// TestTokenizerAllocationIsLinear guards RV2-01. The comment, bogus comment,
// doctype and raw-text readers used to lowercase (and therefore copy) the whole
// remaining input for every such element, which made tokenization quadratic in
// the document size. Total allocation must stay proportional to the input.
func TestTokenizerAllocationIsLinear(t *testing.T) {
	cases := []struct {
		name string
		unit string
	}{
		{"comment", "<!--x-->"},
		{"bogus comment", "<!x>"},
		{"doctype", "<!doctype html>"},
		{"raw text", "<style>x</style>"},
	}
	const size = 32 << 10
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(strings.Repeat(tc.unit, size/len(tc.unit)))
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			tokens := tokenCount(t, src)
			runtime.ReadMemStats(&after)
			if tokens == 0 {
				t.Fatal("no tokens produced")
			}
			allocated := after.TotalAlloc - before.TotalAlloc
			if limit := uint64(len(src)) * 24; allocated > limit {
				t.Fatalf("tokenizing %d bytes allocated %d bytes (limit %d): the per-element cost must not depend on the remaining input", len(src), allocated, limit)
			}
		})
	}
}

// tokenCount runs the tokenizer to EOF and returns the number of tokens, so the
// measurement includes every reader.
func tokenCount(t *testing.T, src []byte) int {
	t.Helper()
	z := NewTokenizer(src)
	n := 0
	for {
		tok := z.Next()
		if tok.Kind == EOFToken {
			return n
		}
		n++
		if tok.Kind == StartTagToken && (tok.Name == "style" || tok.Name == "script") {
			z.SetRawTag(tok.Name)
		}
	}
}

func BenchmarkDecodeEntities(b *testing.B) {
	for _, size := range []int{8192, 32768, 131072} {
		src := strings.Repeat("&", size)
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.SetBytes(int64(size))
			for i := 0; i < b.N; i++ {
				decodeEntities(src)
			}
		})
	}
}

// TestFosterParentKeepsChildContent guards RV2-06: once an element is
// foster-parented out of a table, its own text and children belong to it. They
// used to be fostered again next to the table, which printed a display:none
// block that should have stayed hidden.
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

// TestFosterParentingIsLinear guards RV2-07: every foster-parented node was
// inserted by scanning the parent's children from the start, so N nodes inside
// a table cost O(N²). The check compares the foster case with the same parser
// on the same number of nodes outside a table: the ratio does not depend on the
// machine speed or on the race detector's overhead, while a quadratic insertion
// makes the foster case many times slower than the linear baseline.
func TestFosterParentingIsLinear(t *testing.T) {
	const n = 50000
	unit := strings.Repeat("<div>A</div>", n)
	doc, baseline := parseTimed(t, unit)
	_, fostered := parseTimed(t, "<table>"+unit+"</table>")
	if baseline < time.Millisecond {
		t.Skipf("the parse baseline %v is too small to compare", baseline)
	}
	if limit := 6 * baseline; fostered > limit {
		t.Fatalf("parsing %d foster-parented nodes took %v, %.1f times the %v baseline: the insertion must not be quadratic",
			n, fostered, float64(fostered)/float64(baseline), baseline)
	}
	body := findElement(doc.Root, "body")
	if body == nil {
		t.Fatal("no body")
	}
	divs := 0
	for _, c := range body.Children {
		if c.Type == ElementNode && c.Tag == "div" {
			divs++
		}
	}
	if divs != n {
		t.Fatalf("body has %d divs, want %d", divs, n)
	}
}

// parseTimed parses src and returns the document with the best of three runs,
// so a single scheduling hiccup does not decide the comparison.
func parseTimed(t *testing.T, src string) (*Document, time.Duration) {
	t.Helper()
	var doc *Document
	best := time.Duration(1<<63 - 1)
	data := []byte(src)
	for i := 0; i < 3; i++ {
		start := time.Now()
		d, err := Parse(data, "foster.html")
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		doc = d
		if elapsed < best {
			best = elapsed
		}
	}
	return doc, best
}

// TestNestedTableStartClosesOuterTable guards RV2-08: a <table> start tag
// inside table structure used to become a child of the outer table, and layout
// silently dropped every inner row. The HTML parser closes the outer table and
// opens a sibling instead.
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
