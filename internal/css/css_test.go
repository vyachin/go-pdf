package css

import (
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/html"
)

func mustDoc(t *testing.T, src string) *html.Document {
	t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestTokenize(t *testing.T) {
	toks := Tokenize([]byte(`div.a { margin: 12pt 50%; color: #ff0000; }`))
	var kinds []TokenKind
	var texts []string
	for _, tok := range toks {
		if tok.Kind == WhitespaceToken || tok.Kind == EOFToken {
			continue
		}
		kinds = append(kinds, tok.Kind)
		texts = append(texts, tok.Text)
	}
	wantKinds := []TokenKind{
		IdentToken, DelimToken, IdentToken, LBraceToken,
		IdentToken, ColonToken, DimensionToken, PercentageToken, SemicolonToken,
		IdentToken, ColonToken, HashToken, SemicolonToken, RBraceToken,
	}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("got %d tokens (%v), want %d", len(kinds), texts, len(wantKinds))
	}
	for i := range wantKinds {
		if kinds[i] != wantKinds[i] {
			t.Fatalf("token %d (%q) kind = %v, want %v", i, texts[i], kinds[i], wantKinds[i])
		}
	}
	if texts[6] != "12pt" {
		t.Fatalf("dimension text = %q, want 12pt", texts[6])
	}
}

func TestParseShorthands(t *testing.T) {
	sh := Parse([]byte(`div { margin: 1mm 2mm 3mm 4mm; padding: 5mm; border: 1pt solid #f00 }`))
	if len(sh.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", sh.Problems)
	}
	if len(sh.Rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(sh.Rules))
	}
	st := DefaultStyle()
	st.FontSize = 12000
	for _, d := range sh.Rules[0].Decls {
		if err := ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
			t.Fatalf("%s: %v", d.Property, err)
		}
	}
	wantMargin := [4]int32{2835, 5669, 8504, 11339}
	for i, want := range wantMargin {
		if diff := st.Margin[i].Value - want; diff > 2 || diff < -2 {
			t.Fatalf("margin[%d] = %d, want about %d", i, st.Margin[i].Value, want)
		}
	}
	if st.Padding[0].Value != st.Padding[3].Value {
		t.Fatalf("padding shorthand did not expand: %+v", st.Padding)
	}
	for i := 0; i < 4; i++ {
		b := st.Border[i]
		if b.Style != BorderSolid || b.Width != 1000 || !b.Color.Set || b.Color.R != 255 || b.Color.G != 0 {
			t.Fatalf("border[%d] = %+v", i, b)
		}
	}
}

func TestSelectorSpecificity(t *testing.T) {
	cases := []struct {
		sel  string
		want [3]int
	}{
		{"p", [3]int{0, 0, 1}},
		{".a", [3]int{0, 1, 0}},
		{"#x", [3]int{1, 0, 0}},
		{"div > p.a", [3]int{0, 1, 2}},
		{"div p", [3]int{0, 0, 2}},
		{"*", [3]int{0, 0, 0}},
		{"td.total", [3]int{0, 1, 1}},
	}
	for _, tc := range cases {
		sel, err := ParseSelector(tc.sel)
		if err != nil {
			t.Fatalf("ParseSelector(%q): %v", tc.sel, err)
		}
		if got := sel.Specificity(); got != tc.want {
			t.Fatalf("%q specificity = %v, want %v", tc.sel, got, tc.want)
		}
	}
}

func TestSelectorRejection(t *testing.T) {
	for _, sel := range []string{"tr:nth-child(2n)", "[href]", "div >", "> div", "", "a::", "a::after.b"} {
		if _, err := ParseSelector(sel); err == nil {
			t.Fatalf("ParseSelector(%q) = nil error, want rejection", sel)
		}
	}
}

func TestPseudoElementSelector(t *testing.T) {
	sel, err := ParseSelector("a.toc::after")
	if err != nil {
		t.Fatalf("a.toc::after must parse: %v", err)
	}
	if got := sel.PseudoElement(); got != "after" {
		t.Fatalf("PseudoElement() = %q, want after", got)
	}
	if sel.Compounds[0].Tag != "a" {
		t.Fatalf("tag = %q, want a", sel.Compounds[0].Tag)
	}
	// A pseudo-element counts as an element for specificity.
	if got, want := sel.Specificity(), [3]int{0, 1, 2}; got != want {
		t.Fatalf("specificity = %v, want %v", got, want)
	}
	// It still matches the originating element.
	doc := mustDoc(t, `<a class="toc" href="#x">1</a>`)
	a := doc.FindByID("x")
	if a != nil {
		t.Fatal("unexpected element")
	}
	node := doc.Body().FirstElementChild()
	if node == nil || !sel.Match(node) {
		t.Fatal("the pseudo-element selector must match the anchor itself")
	}
}

func TestSelectorMatching(t *testing.T) {
	doc := mustDoc(t, `<div class="a"><p id="x" class="t">t</p></div>`)
	p := doc.FindByID("x")
	if p == nil {
		t.Fatal("no p")
	}
	cases := map[string]bool{
		"p":        true,
		"div p":    true,
		"div > p":  true,
		".a p":     true,
		".a > p":   true,
		"p#x":      true,
		"p.t":      true,
		"td":       false,
		"div > div": false,
	}
	for sel, want := range cases {
		s, err := ParseSelector(sel)
		if err != nil {
			t.Fatalf("%q: %v", sel, err)
		}
		if got := s.Match(p); got != want {
			t.Fatalf("%q.Match(p) = %v, want %v", sel, got, want)
		}
	}
}

func TestCascadePriority(t *testing.T) {
	doc := mustDoc(t, `<div class="a" style="color: blue"><p id="x" class="t">t</p></div>`)
	sheet := Parse([]byte(`
		.a { color: red; font-size: 20pt }
		#x { font-weight: 700 }
		p.t { text-align: right }
		p { text-align: center }
	`))
	c := NewCascade(sheet)
	styles := computeAll(doc, c)
	div := doc.Body().FirstElementChild()
	if div.Tag != "div" {
		t.Fatalf("unexpected first child %q", div.Tag)
	}
	if got := styles[div].Color; got != (Color{R: 0, G: 0, B: 255, Set: true}) {
		t.Fatalf("div color = %+v, want the inline blue", got)
	}
	if styles[div].FontSize != 20000 {
		t.Fatalf("div font-size = %d, want 20000", styles[div].FontSize)
	}
	p := doc.FindByID("x")
	if got := styles[p].Color; got != (Color{R: 0, G: 0, B: 255, Set: true}) {
		t.Fatalf("p color = %+v, want inherited blue", got)
	}
	if styles[p].FontWeight != 700 {
		t.Fatalf("p font-weight = %d, want 700", styles[p].FontWeight)
	}
	if styles[p].TextAlign != AlignRight {
		t.Fatalf("p text-align = %v, want right (higher specificity wins over later rule)", styles[p].TextAlign)
	}
}

func TestImportantBeatsInline(t *testing.T) {
	doc := mustDoc(t, `<div class="a" style="color: blue">x</div>`)
	sheet := Parse([]byte(`.a { color: red !important }`))
	c := NewCascade(sheet)
	div := doc.Body().FirstElementChild()
	if got := c.Compute(div, nil).Color; got != (Color{R: 255, G: 0, B: 0, Set: true}) {
		t.Fatalf("color = %+v, want important red", got)
	}
}

func TestUnits(t *testing.T) {
	doc := mustDoc(t, `<div>x</div>`)
	sheet := Parse([]byte(`div { width: 100mm; padding: 1.5em; font-size: 12pt }`))
	c := NewCascade(sheet)
	div := doc.Body().FirstElementChild()
	st := c.Compute(div, nil)
	if diff := st.Width.Value - 283465; diff > 2 || diff < -2 {
		t.Fatalf("width = %d, want about 283465", st.Width.Value)
	}
	if st.Padding[0].Value != 18000 {
		t.Fatalf("padding = %d, want 18000 (1.5em at 12pt)", st.Padding[0].Value)
	}
}

func TestUnsupportedUnitsAndValues(t *testing.T) {
	cases := []string{
		`div { margin: 2rem }`,
		`div { display: flex }`,
		`div { box-shadow: 0 0 4px black }`,
		`div { width: 50 }`,
	}
	for _, src := range cases {
		st := DefaultStyle()
		sh := Parse([]byte(src))
		if len(sh.Rules) != 1 || len(sh.Rules[0].Decls) != 1 {
			t.Fatalf("%q: unexpected parse result", src)
		}
		d := sh.Rules[0].Decls[0]
		if err := ApplyDecl(st, d.Property, d.Value, 12000); err == nil {
			t.Fatalf("%q: expected an error", src)
		} else if !strings.Contains(err.Error(), "") {
			t.Fatal("unreachable")
		}
	}
}

func TestPageSizeAndContent(t *testing.T) {
	size, err := ParsePageSize([]Token{{Kind: IdentToken, Text: "A4"}})
	if err != nil {
		t.Fatal(err)
	}
	if size.Width != 595276 || size.Height != 841890 {
		t.Fatalf("A4 = %+v", size)
	}
	items, err := ParseContent([]Token{
		{Kind: StringToken, Text: `"Стр. "`},
		{Kind: WhitespaceToken, Text: " "},
		{Kind: IdentToken, Text: "counter"},
		{Kind: LParenToken, Text: "("},
		{Kind: IdentToken, Text: "page"},
		{Kind: RParenToken, Text: ")"},
		{Kind: StringToken, Text: `" из "`},
		{Kind: IdentToken, Text: "counter"},
		{Kind: LParenToken, Text: "("},
		{Kind: IdentToken, Text: "pages"},
		{Kind: RParenToken, Text: ")"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []ContentKind{ContentText, ContentPageNumber, ContentText, ContentPageCount}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d", len(items), len(want))
	}
	for i := range want {
		if items[i].Kind != want[i] {
			t.Fatalf("item %d kind = %v, want %v", i, items[i].Kind, want[i])
		}
	}
}

func computeAll(doc *html.Document, c *Cascade) map[*html.Node]*Style {
	out := map[*html.Node]*Style{}
	var walk func(n *html.Node, parent *Style)
	walk = func(n *html.Node, parent *Style) {
		var st *Style
		if n.Type == html.ElementNode {
			st = c.Compute(n, parent)
			out[n] = st
		}
		for _, ch := range n.Children {
			walk(ch, st)
		}
	}
	walk(doc.Root, nil)
	return out
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(`p { margin: 1mm; color: #000 }`))
	f.Add([]byte(`@page { size: A4; @bottom-center { content: counter(page) } }`))
	f.Add([]byte(`@font-face { font-family: "x"; src: url(a.ttf) }`))
	f.Add([]byte(`a::after { content: target-counter(attr(href), page) }`))
	f.Add([]byte(`@media print { td { border: 1pt solid red } }`))
	f.Fuzz(func(t *testing.T, b []byte) {
		sh := Parse(b)
		for _, r := range sh.Rules {
			_ = r.Decls
		}
	})
}
