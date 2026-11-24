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
	f.Add([]byte(`@media print { td { border: 1pt solid red } }`))
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
