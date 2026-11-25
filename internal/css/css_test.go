package css

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestBoxLonghands(t *testing.T) {
	st := DefaultStyle()
	st.FontSize = 12000
	apply := func(prop, value string) {
		t.Helper()
		if err := ApplyDecl(st, prop, Tokenize([]byte(value)), 12000); err != nil {
			t.Fatalf("%s: %s: %v", prop, value, err)
		}
	}
	apply("margin", "1mm 2mm 3mm 4mm")
	apply("margin-top", "5mm")
	apply("padding", "1pt 2pt 3pt 4pt")
	apply("padding-left", "6pt")
	if got, want := st.Margin[0].Value, int32(14173); got != want {
		t.Fatalf("margin-top = %d, want %d", got, want)
	}
	if st.Margin[1].Value == st.Margin[0].Value {
		t.Fatal("margin-top must not change the other sides")
	}
	if got, want := st.Padding[3].Value, int32(6000); got != want {
		t.Fatalf("padding-left = %d, want %d", got, want)
	}
	for _, c := range []struct{ prop, value string }{
		{"padding-top", "-1pt"},
		{"padding-top", "auto"},
		{"margin-top", "auto"},
	} {
		if err := ApplyDecl(DefaultStyle(), c.prop, Tokenize([]byte(c.value)), 12000); err == nil {
			t.Fatalf("%s: %s must be rejected", c.prop, c.value)
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
		{":root", [3]int{0, 1, 0}},
		{"p + p", [3]int{0, 0, 2}},
		{"div + p.a", [3]int{0, 1, 2}},
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
		"p":         true,
		"div p":     true,
		"div > p":   true,
		".a p":      true,
		".a > p":    true,
		"p#x":       true,
		"p.t":       true,
		"td":        false,
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

func TestAdjacentAndRootSelectors(t *testing.T) {
	doc := mustDoc(t, `<p id="a">a</p><p id="b">b</p><p id="c">c</p>`)
	sel, err := ParseSelector("p + p")
	if err != nil {
		t.Fatalf("p + p: %v", err)
	}
	if sel.Match(doc.FindByID("a")) {
		t.Fatal("the first sibling must not match p + p")
	}
	if !sel.Match(doc.FindByID("b")) || !sel.Match(doc.FindByID("c")) {
		t.Fatal("later siblings must match p + p")
	}
	root, err := ParseSelector(":root")
	if err != nil {
		t.Fatalf(":root: %v", err)
	}
	if !root.Match(doc.HTML()) {
		t.Fatal(":root must match the document element")
	}
	if root.Match(doc.FindByID("a")) {
		t.Fatal(":root must not match a descendant")
	}
	if _, err := ParseSelector("p:nth-child(2)"); err == nil {
		t.Fatal("an unsupported pseudo-class must still be rejected")
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
	spec, err := ParsePageSizeSpec([]Token{{Kind: IdentToken, Text: "A4"}})
	if err != nil {
		t.Fatal(err)
	}
	size := spec.Resolve(PageSize{Width: 1, Height: 2})
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
	// Decoded escapes, a quoted bracket in a selector list and the border
	// colour grammar.
	f.Add([]byte(`a::after { content: "\2014 A\20 B" }`))
	f.Add([]byte(`p[title="("], div { border: 1px solid rgb(255, 0, 0) }`))
	f.Add([]byte(`@media print { td { border: 1pt solid red } }`))
	// Nested at-rules exercise the depth limit: within the limit and past it,
	// balanced and unclosed.
	f.Add([]byte(strings.Repeat("@media { ", MaxAtRuleNesting) + "p { color: #000 }" + strings.Repeat(" }", MaxAtRuleNesting)))
	f.Add([]byte(strings.Repeat("@media {", MaxAtRuleNesting+1)))
	f.Fuzz(func(t *testing.T, b []byte) {
		sh := Parse(b)
		for _, r := range sh.Rules {
			_ = r.Decls
		}
	})
}

func TestPageSizeSpec(t *testing.T) {
	a4 := PageSize{Width: 595276, Height: 841890}
	a5 := PageSize{Width: 419528, Height: 595276}
	cases := []struct {
		name        string
		value       string
		wantErr     bool
		wantBase    PageSize
		hasBase     bool
		orientation Orientation
	}{
		{name: "named", value: "A4", wantBase: a4, hasBase: true},
		{name: "named landscape", value: "A4 landscape", wantBase: a4, hasBase: true, orientation: OrientationLandscape},
		{name: "orientation first", value: "landscape A4", wantBase: a4, hasBase: true, orientation: OrientationLandscape},
		{name: "portrait", value: "A4 portrait", wantBase: a4, hasBase: true, orientation: OrientationPortrait},
		{name: "orientation only", value: "landscape", orientation: OrientationLandscape},
		{name: "two lengths", value: "210mm 297mm", wantBase: a4, hasBase: true},
		{name: "unknown word", value: "A4 sideways", wantErr: true},
		{name: "orientation with lengths", value: "210mm 297mm landscape", wantErr: true},
		{name: "mixed name and length", value: "A4 210mm", wantErr: true},
		{name: "two names", value: "A4 A5", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := ParsePageSizeSpec(Tokenize([]byte(tc.value)))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q: %v", tc.value, err)
			}
			if spec.HasBase != tc.hasBase || spec.Orientation != tc.orientation {
				t.Fatalf("%q: spec = %+v", tc.value, spec)
			}
			if tc.hasBase && spec.Base != tc.wantBase {
				t.Fatalf("%q: base = %+v, want %+v", tc.value, spec.Base, tc.wantBase)
			}
		})
	}
	// Landscape swaps the fallback size when no name is given.
	if got := (PageSizeSpec{Orientation: OrientationLandscape}).Resolve(a5); got.Width != a5.Height || got.Height != a5.Width {
		t.Fatalf("landscape fallback = %+v, want swapped A5", got)
	}
	// A named size is used as is when the run options say otherwise.
	if got := (PageSizeSpec{Base: a4, HasBase: true}).Resolve(a5); got != a4 {
		t.Fatalf("named size = %+v, want A4", got)
	}
}

func TestStylesheetPositions(t *testing.T) {
	src := []byte("p {\n  color: red;\n}\n")
	sheet := ParseSheet(SheetOptions{File: "styles/print.css"}, src)
	offset := strings.Index(string(src), "red")
	pos := sheet.Position(offset)
	if pos.File != "styles/print.css" || pos.Line != 2 || pos.Col != 10 {
		t.Fatalf("position = %+v, want styles/print.css:2:10", pos)
	}
	// A stylesheet without a locator also reports its own file.
	if pos := sheet.Position(0); pos.File != "styles/print.css" || pos.Line != 1 {
		t.Fatalf("position(0) = %+v", pos)
	}
	// Inline sheets delegate to the document locator.
	inline := ParseSheet(SheetOptions{Base: 100, Locate: func(off int) Position {
		return Position{File: "doc.html", Line: 7, Col: off - 100 + 1}
	}}, []byte("p { color: red }"))
	if pos := inline.Position(104); pos.File != "doc.html" || pos.Line != 7 || pos.Col != 5 {
		t.Fatalf("inline position = %+v", pos)
	}
}

func TestRuleKeepsItsSheet(t *testing.T) {
	sheet := ParseSheet(SheetOptions{File: "a.css"}, []byte("p { color: red }"))
	if len(sheet.Rules) != 1 {
		t.Fatalf("got %d rules", len(sheet.Rules))
	}
	rule := sheet.Rules[0]
	if rule.Sheet != sheet {
		t.Fatal("the rule does not reference its stylesheet")
	}
	if len(rule.Decls) != 1 || rule.Decls[0].Sheet != sheet {
		t.Fatal("the declaration does not reference its stylesheet")
	}
}

func TestInvalidDeclarationsAreReported(t *testing.T) {
	cases := []string{
		"p { color red }",
		"p { width 10pt }",
		"p { color: red; broken }",
	}
	for _, src := range cases {
		sheet := ParseSheet(SheetOptions{File: "t.css"}, []byte(src))
		if len(sheet.Problems) == 0 {
			t.Fatalf("%q: the skipped declaration was not reported", src)
		}
	}
	_, probs := ParseDeclarations("color red")
	if len(probs) == 0 {
		t.Fatal("an invalid inline declaration was not reported")
	}
}

func TestPseudoContentFollowsTheCascade(t *testing.T) {
	cssText := `
		a.hot::after { content: "CORRECT" }
		a::after { content: "WRONG" }
	`
	sheet := ParseSheet(SheetOptions{File: "t.css"}, []byte(cssText))
	doc, err := html.Parse([]byte(`<p><a class="hot">A</a></p>`), "t.html")
	if err != nil {
		t.Fatal(err)
	}
	var node *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Tag == "a" {
			node = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc.Root)
	st := NewCascade(sheet).Compute(node, nil)
	if len(st.AfterContent) == 0 || st.AfterContent[0].Text != "CORRECT" {
		t.Fatalf("specificity was ignored: %+v", st.AfterContent)
	}

	cssText = `
		a::after { content: "IMPORTANT" !important }
		a.hot::after { content: "LATER" }
	`
	sheet = ParseSheet(SheetOptions{File: "t.css"}, []byte(cssText))
	st = NewCascade(sheet).Compute(node, nil)
	if len(st.AfterContent) == 0 || st.AfterContent[0].Text != "IMPORTANT" {
		t.Fatalf("!important was ignored: %+v", st.AfterContent)
	}
}

func TestHugeLengthsAreRejected(t *testing.T) {
	toks := Tokenize([]byte("1e100pt"))
	if _, err := ParseLengthToken(toks[0]); err == nil {
		t.Fatal("a huge length was accepted")
	}
	toks = Tokenize([]byte("1e100%"))
	if _, err := ParseLengthToken(toks[0]); err == nil {
		t.Fatal("a huge percentage was accepted")
	}
}

func findElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Tag == tag {
		return n
	}
	for _, c := range n.Children {
		if found := findElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func TestAuthorOriginBeatsUserAgentSpecificity(t *testing.T) {
	doc := mustDoc(t, `<table><tr><td>A</td></tr></table>`)
	td := findElement(doc.Root, "td")
	if td == nil {
		t.Fatal("no td")
	}
	uas := DefaultStylesheet()
	author := Parse([]byte(`* { padding: 0; margin: 0 }`))
	st := NewCascadeForDocument(uas, author).Compute(td, nil)
	for _, side := range st.Padding {
		if side.Value != 0 {
			t.Fatalf("author * reset lost to the UA td rule: padding = %v", st.Padding)
		}
	}
	for _, side := range st.Margin {
		if side.Value != 0 {
			t.Fatalf("author * reset lost to the UA td rule: margin = %v", st.Margin)
		}
	}
	// Without the author reset the UA rule still applies.
	plain := NewCascadeForDocument(uas).Compute(td, nil)
	if plain.Padding[0].Value == 0 {
		t.Fatal("the UA td padding disappeared")
	}
}

func TestUserAgentImportantBeatsAuthorImportant(t *testing.T) {
	doc := mustDoc(t, `<p id="p">A</p>`)
	p := findElement(doc.Root, "p")
	if p == nil {
		t.Fatal("no p")
	}
	ua := Parse([]byte(`p { color: #ff0000 !important }`))
	author := Parse([]byte(`p { color: #00ff00 !important }`))
	st := NewCascadeForDocument(ua, author).Compute(p, nil)
	if want := (Color{R: 255, Set: true}); st.Color != want {
		t.Fatalf("color = %+v, want the UA !important colour", st.Color)
	}
	// A normal author declaration still beats a normal UA one.
	authorNormal := Parse([]byte(`p { color: #00ff00 }`))
	st = NewCascadeForDocument(Parse([]byte(`p { color: #ff0000 }`)), authorNormal).Compute(p, nil)
	if want := (Color{G: 255, Set: true}); st.Color != want {
		t.Fatalf("color = %+v, want the author colour", st.Color)
	}
}

// TestLongSelectorMatchesLikeTheNaiveMatcher guards RV2-02: the dynamic
// programming matcher for three or more compounds must return exactly what the
// recursive matcher returns, for every combinator combination.
func TestLongSelectorMatchesLikeTheNaiveMatcher(t *testing.T) {
	doc := mustDoc(t, `<html><body>
		<div id="a" class="x">a</div>
		<div class="y">b<p class="z">c</p><span>d</span><div class="y z"><p id="b">e</p><em class="e">f</em></div></div>
		<p class="z">g</p>
	</body></html>`)
	selectors := []string{
		"div div div",
		"div > div > div",
		"div div p",
		"div > p span",
		"#a div .z p",
		".x .y .z #b",
		"body div div p span div p",
		"div > div p > span > div > p",
		"html body div .y p span .z",
		// The adjacent combinator in every position, over siblings, ids,
		// classes, :root and the supported pseudo-element.
		"div + div",
		"div + p",
		"div + div p",
		"div p + span",
		"div + div > p",
		"div > div + div",
		"#a + .y p",
		".x + div .z",
		"body div + div p",
		"p + span",
		"p + span div",
		"p + span > div",
		"div + div + p",
		".y + .y .z",
		"div + div p + span",
		"div + div > p span",
		"div > div + div p",
		":root body div",
		":root body div + div",
		":root > body p",
		"div + p::after",
		"p::after + span",
		"div + div .y .z #b",
	}
	for _, text := range selectors {
		sel, err := ParseSelector(text)
		if err != nil {
			t.Fatalf("ParseSelector(%q): %v", text, err)
		}
		walkElements(doc.Root, func(n *html.Node) {
			want := matchFrom(sel, len(sel.Compounds)-1, n)
			if got := sel.Match(n); got != want {
				t.Errorf("selector %q on <%s id=%q class=%q>: Match=%v, naive=%v", text, n.Tag, n.ID(), attrOf(n, "class"), got, want)
			}
		})
	}
}

func attrOf(n *html.Node, key string) string {
	v, _ := n.Attr(key)
	return v
}

func walkElements(n *html.Node, visit func(*html.Node)) {
	if n.Type == html.ElementNode {
		visit(n)
	}
	for _, c := range n.Children {
		walkElements(c, visit)
	}
}

// TestDeepDescendantSelectorFinishes guards RV2-02: a 20-compound descendant
// selector on a deep tree used to take combinatorial time. The naive matcher
// needs C(depth, compounds) steps, so this test must not hang when the
// optimisation is removed.
func TestDeepDescendantSelectorFinishes(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><body>")
	const depth = 200
	for i := 0; i < depth; i++ {
		b.WriteString("<div>")
	}
	b.WriteString("x")
	for i := 0; i < depth; i++ {
		b.WriteString("</div>")
	}
	b.WriteString("</body></html>")
	doc := mustDoc(t, b.String())

	sel, err := ParseSelector("table " + strings.Repeat("div ", 19) + "div")
	if err != nil {
		t.Fatalf("ParseSelector: %v", err)
	}
	// The first compound never matches, so every ancestor chain is explored.
	var deepest *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Tag == "div" && (deepest == nil || depthOf(n) > depthOf(deepest)) {
			deepest = n
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(doc.Root)
	if deepest == nil {
		t.Fatal("no div found")
	}
	done := make(chan bool, 1)
	go func() { done <- sel.Match(deepest) }()
	select {
	case got := <-done:
		if got {
			t.Fatal("selector matched a document without a table")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("matching a 20-compound descendant selector on a 200-deep tree did not finish in 5s")
	}
}

// TestAdjacentSelectorMatchingIsLinear guards the bounded matcher with a step
// counter instead of a clock: for eight compounds and a 30-deep tree the
// recursive matcher explores every distribution of the descendant compounds
// over the ancestors, while the segmented matcher compares each compound with
// each ancestor once.
func TestAdjacentSelectorMatchingIsLinear(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><body>")
	const depth = 30
	for i := 0; i < depth; i++ {
		b.WriteString("<div>")
	}
	b.WriteString("x")
	for i := 0; i < depth; i++ {
		b.WriteString("</div>")
	}
	b.WriteString("</body></html>")
	doc := mustDoc(t, b.String())

	sel, err := ParseSelector("div + div div div div div div div")
	if err != nil {
		t.Fatalf("ParseSelector: %v", err)
	}
	var deepest *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Tag == "div" && (deepest == nil || depthOf(n) > depthOf(deepest)) {
			deepest = n
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(doc.Root)
	if deepest == nil {
		t.Fatal("no div found")
	}
	m := &matcher{}
	_ = sel.match(deepest, m)
	if bound := 4 * len(sel.Compounds) * depth; m.steps > bound {
		t.Fatalf("matching took %d compound comparisons, want at most %d", m.steps, bound)
	}
}

// BenchmarkSelectorAdjacentWide measures the adjacent hop across many siblings.
func BenchmarkSelectorAdjacentWide(b *testing.B) {
	var bld strings.Builder
	bld.WriteString("<html><body>")
	for i := 0; i < 1000; i++ {
		bld.WriteString(`<p class="x">t</p>`)
	}
	bld.WriteString("</body></html>")
	doc, err := html.Parse([]byte(bld.String()), "wide.html")
	if err != nil {
		b.Fatal(err)
	}
	sel, err := ParseSelector("p + p")
	if err != nil {
		b.Fatal(err)
	}
	var last *html.Node
	walkElements(doc.Root, func(n *html.Node) {
		if n.Tag == "p" {
			last = n
		}
	})
	if last == nil {
		b.Fatal("no p found")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sel.Match(last)
	}
}

// BenchmarkSelectorAdjacentDeep measures the segmented matcher on a deep tree.
func BenchmarkSelectorAdjacentDeep(b *testing.B) {
	var bld strings.Builder
	bld.WriteString("<html><body>")
	const depth = 200
	for i := 0; i < depth; i++ {
		bld.WriteString("<div>")
	}
	bld.WriteString("x")
	for i := 0; i < depth; i++ {
		bld.WriteString("</div>")
	}
	bld.WriteString("</body></html>")
	doc, err := html.Parse([]byte(bld.String()), "deep.html")
	if err != nil {
		b.Fatal(err)
	}
	sel, err := ParseSelector("div div div + div div div")
	if err != nil {
		b.Fatal(err)
	}
	var deepest *html.Node
	walkElements(doc.Root, func(n *html.Node) {
		if n.Tag == "div" && (deepest == nil || depthOf(n) > depthOf(deepest)) {
			deepest = n
		}
	})
	if deepest == nil {
		b.Fatal("no div found")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sel.Match(deepest)
	}
}

func depthOf(n *html.Node) int {
	d := 0
	for p := n.Parent; p != nil; p = p.Parent {
		d++
	}
	return d
}

// TestAutoIsRejectedOutsideWidthAndHeight guards RV2-13: "margin: 0 auto" was
// accepted and then resolved to zero, so a block that should have been centred
// silently printed left aligned. Auto margins, auto text-indent and auto
// line-height are not part of the closed profile; width and height still accept
// auto.
func applyTestDecl(st *Style, decl string) error {
	i := strings.Index(decl, ":")
	if i < 0 {
		return fmt.Errorf("bad declaration %q", decl)
	}
	return ApplyDecl(st, strings.TrimSpace(decl[:i]), Tokenize([]byte(decl[i+1:])), 12000)
}

func TestAutoIsRejectedOutsideWidthAndHeight(t *testing.T) {
	rejected := []string{
		"margin: 0 auto",
		"margin: auto",
		"text-indent: auto",
		"line-height: auto",
	}
	for _, decl := range rejected {
		t.Run(decl, func(t *testing.T) {
			var st Style
			if err := applyTestDecl(&st, decl); err == nil {
				t.Fatalf("%q was accepted", decl)
			}
		})
	}
	accepted := []string{
		"width: auto",
		"height: auto",
		"line-height: normal",
		"margin: 0 10pt",
		"text-indent: 12pt",
	}
	for _, decl := range accepted {
		t.Run(decl, func(t *testing.T) {
			var st Style
			if err := applyTestDecl(&st, decl); err != nil {
				t.Fatalf("%q was rejected: %v", decl, err)
			}
		})
	}
}
