package css

import (
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/html"
)

// TestCSSStringEscapes guards the tokenizer: quotes keep the raw spelling, so
// values must decode hexadecimal, space and newline escapes before rendering.
func TestCSSStringEscapes(t *testing.T) {
	cases := map[string]string{
		`"\2014"`:    "—",
		`"\41"`:      "A",
		`"A\20 B"`:   "A B",
		"\"A\\\nB\"": "AB",
		`"caf\e9"`:   "café",
	}
	for src, want := range cases {
		items, err := ParseContent(Tokenize([]byte(src)))
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if len(items) != 1 || items[0].Kind != ContentText || items[0].Text != want {
			t.Fatalf("%s decoded to %+v, want %q", src, items, want)
		}
	}
	// An ordinary string is unchanged.
	if got := unquote(`"plain"`); got != "plain" {
		t.Fatalf("unquote = %q", got)
	}
}

// TestSelectorListSplitIsTokenAware guards the selector list: a bracket inside a
// string does not swallow the comma, so the div part survives.
func TestSelectorListSplitIsTokenAware(t *testing.T) {
	parts := splitTopLevelComma(`p[title="("], div`)
	if len(parts) != 2 || parts[1] != "div" {
		t.Fatalf("parts = %q, want the div selector kept", parts)
	}
	sheet := Parse([]byte(`p[title="("], div { color: #ff0000 }`))
	found := false
	for _, r := range sheet.Rules {
		if r.Kind == StyleRule && strings.TrimSpace(r.SelectorText) == "div" && r.Err == nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("the div rule was lost: %+v", sheet.Rules)
	}
}

// TestBorderAcceptsEveryColorForm guards the border shorthand: it must accept
// the same colours as the color property and keep transparent invisible.
func TestBorderAcceptsEveryColorForm(t *testing.T) {
	doc := mustDoc(t, `<p id="x">x</p>`)
	for _, tc := range []struct {
		decl string
		want Color
	}{
		{"border: 1px solid rgb(255, 0, 0)", Color{R: 255, Set: true}},
		{"border: 1px solid #ff0000", Color{R: 255, Set: true}},
		{"border: 1px solid red", Color{R: 255, Set: true}},
	} {
		sheet := Parse([]byte("p { " + tc.decl + " }"))
		st := NewCascade(sheet).Compute(doc.FindByID("x"), nil)
		if st.Border[0].Color != tc.want {
			t.Fatalf("%s: border colour = %+v, want %+v", tc.decl, st.Border[0].Color, tc.want)
		}
	}
	sheet := Parse([]byte(`p { border: 1px solid transparent }`))
	st := NewCascade(sheet).Compute(doc.FindByID("x"), nil)
	if !st.Border[0].Color.Transparent || st.Border[0].IsVisible() {
		t.Fatalf("transparent border = %+v, want an invisible side", st.Border[0])
	}
}

// TestPageMarginGrammar guards page margins: lengths only, with em resolved
// against the initial 12 pt font size.
func TestPageMarginGrammar(t *testing.T) {
	box, err := ParsePageMargin(Tokenize([]byte("1em")))
	if err != nil {
		t.Fatal(err)
	}
	if box[0].Kind != MP || box[0].Value != 12000 {
		t.Fatalf("1em page margin = %+v, want 12 pt", box[0])
	}
	for _, value := range []string{"auto", "10%", "-20mm"} {
		if _, err := ParsePageMargin(Tokenize([]byte(value))); err == nil {
			t.Fatalf("page margin %q was accepted", value)
		}
	}
}

// TestPortraitFlipsLandscapeFallback guards both orientations of the fallback.
func TestPortraitFlipsLandscapeFallback(t *testing.T) {
	portrait, err := ParsePageSizeSpec(Tokenize([]byte("portrait")))
	if err != nil {
		t.Fatal(err)
	}
	if got := portrait.Resolve(PageSize{Width: 850394, Height: 566929}); got.Width != 566929 || got.Height != 850394 {
		t.Fatalf("portrait resolved to %v", got)
	}
	landscape, err := ParsePageSizeSpec(Tokenize([]byte("landscape")))
	if err != nil {
		t.Fatal(err)
	}
	if got := landscape.Resolve(PageSize{Width: 566929, Height: 850394}); got.Width != 850394 || got.Height != 566929 {
		t.Fatalf("landscape resolved to %v", got)
	}
}

// TestFontFamilyRejectsCSSWideKeywords guards RV4-11: "inherit" used to be
// stored as a literal family name and then fell through to a system font. The
// property rejects the shared keywords instead.
func TestFontFamilyRejectsCSSWideKeywords(t *testing.T) {
	for _, value := range []string{"inherit", "initial", "unset", "revert"} {
		st := DefaultStyle()
		if err := ApplyDecl(st, "font-family", Tokenize([]byte(value)), 12000); err == nil {
			t.Fatalf("font-family accepted %q as a family", value)
		}
	}
	// A quoted string is a family name, even when it spells a keyword.
	st := DefaultStyle()
	if err := ApplyDecl(st, "font-family", Tokenize([]byte(`"inherit"`)), 12000); err != nil {
		t.Fatalf("a quoted family named inherit was rejected: %v", err)
	}
	if len(st.FontFamily) != 1 || st.FontFamily[0] != "inherit" {
		t.Fatalf("families = %v", st.FontFamily)
	}
}

// TestPseudoElementOnlyInTheFinalCompound guards RV4-19: `a::after span`
// validated and painted the span red, because matching ignored the
// pseudo-element and continued with the next compound. The selector is rejected
// instead.
func TestPseudoElementOnlyInTheFinalCompound(t *testing.T) {
	for _, text := range []string{"a::after span", "a::after + p", "a::after > p"} {
		if _, err := ParseSelector(text); err == nil {
			t.Fatalf("%q was accepted", text)
		}
	}
	if _, err := ParseSelector("div + p::after"); err != nil {
		t.Fatalf("a pseudo-element in the final compound was rejected: %v", err)
	}
}

// TestHeightRejectsPercentage guards RV4-20: height:50% validated and then
// behaved as auto, because layout resolves only an absolute height. The value is
// rejected instead of being silently ignored.
func TestHeightRejectsPercentage(t *testing.T) {
	st := DefaultStyle()
	if err := ApplyDecl(st, "height", Tokenize([]byte("50%")), 12000); err == nil {
		t.Fatal("height:50% was accepted")
	}
	st = DefaultStyle()
	if err := ApplyDecl(st, "height", Tokenize([]byte("50pt")), 12000); err != nil {
		t.Fatalf("an absolute height was rejected: %v", err)
	}
	if st.Height.Kind != MP || st.Height.Value != 50000 {
		t.Fatalf("height = %+v, want 50pt", st.Height)
	}
}

// TestNestedAtRuleIsReported guards RV4-46: a nested at-rule its parent cannot
// represent used to be dropped silently, and @page inside @page was hoisted and
// applied.
func TestNestedAtRuleIsReported(t *testing.T) {
	cases := map[string]string{
		"@font-face with a media rule": `@font-face { font-family: X; src: url(x.ttf); @media print { p { color: red } } }`,
		"@page inside @page":           `@page { size: A4; @page { margin: 0 } }`,
		"a margin box with @media":     `@page { @top-center { content: "x"; @media print { p { color: red } } } }`,
	}
	for name, src := range cases {
		sh := Parse([]byte(src))
		found := false
		for _, p := range sh.Problems {
			if p.Code == "css/unsupported-at-rule" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no css/unsupported-at-rule in %v", name, sh.Problems)
		}
	}
	// A page margin box is still represented, and a nested media rule keeps
	// working where the parent can hold it.
	sh := Parse([]byte(`@page { @top-center { content: "x" } } @media print { p { color: red } }`))
	for _, p := range sh.Problems {
		if p.Code == "css/unsupported-at-rule" {
			t.Fatalf("a supported nesting was reported: %v", p)
		}
	}
}

// TestCompoundWithTwoIDsIsRejected guards RV4-47: #x#y matched an element with
// only the second id, and its specificity counted one id. The compound cannot
// match anything, so it is rejected instead.
func TestCompoundWithTwoIDsIsRejected(t *testing.T) {
	if _, err := ParseSelector("#x#y"); err == nil {
		t.Fatal("#x#y was accepted")
	}
	sel, err := ParseSelector("#y .z")
	if err != nil {
		t.Fatal(err)
	}
	if got := sel.Specificity(); got[0] != 1 {
		t.Fatalf("specificity = %v, want one id", got)
	}
}

// TestIdentifiersDecodeEscapes guards RV4-48: backslash escapes were decoded in
// strings but not in identifiers, so caf\e9 named nothing.
func TestIdentifiersDecodeEscapes(t *testing.T) {
	toks := filterWhitespace(Tokenize([]byte(`caf\e9`)))
	if len(toks) != 1 || toks[0].Kind != IdentToken {
		t.Fatalf("tokens = %v", toks)
	}
	if toks[0].Text != "café" {
		t.Fatalf("the identifier decoded to %q, want café", toks[0].Text)
	}
	st := DefaultStyle()
	if err := ApplyDecl(st, "font-family", Tokenize([]byte(`caf\e9`)), 12000); err != nil {
		t.Fatalf("font-family: %v", err)
	}
	if len(st.FontFamily) != 1 || st.FontFamily[0] != "café" {
		t.Fatalf("families = %v, want café", st.FontFamily)
	}
	// The same escape in a selector names the class. A parsed rule carries the
	// tokenizer's decoded text, which is what ParseSelector receives.
	sel, err := ParseSelector(stringify(Tokenize([]byte(`.caf\e9`))))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse([]byte(`<p class="café">x</p>`), "esc.html")
	if err != nil {
		t.Fatal(err)
	}
	p := findElement(doc.Root, "p")
	if p == nil || !sel.Match(p) {
		t.Fatal("the escaped class selector did not match the element")
	}
}

// TestOneMediaListParserMatchesCollection guards IMP-12: validation and
// collection call one media-list parser, so they agree on every list.
func TestOneMediaListParserMatchesCollection(t *testing.T) {
	lists := []string{"", "print", "all", "screen", "print, screen", "screen, print", "PRINT , all", "speech"}
	for _, list := range lists {
		applies := MediaAppliesToPrint(list)
		// RulesForPrint uses the same function: a media rule with this list is
		// collected exactly when it applies.
		sh := Parse([]byte("@media " + list + " { p { color: red } }"))
		got := len(RulesForPrint([]*Stylesheet{sh})) == 1
		if got != applies {
			t.Fatalf("list %q: collection says %v, the parser says %v", list, got, applies)
		}
	}
}

// TestOneEscapeDecoder guards the escape half of IMP-12: a string, an
// identifier and a hash decode the same escape the same way.
func TestOneEscapeDecoder(t *testing.T) {
	for _, in := range []string{`caf\e9`, `"caf\e9"`} {
		toks := filterWhitespace(Tokenize([]byte(in)))
		if len(toks) != 1 {
			t.Fatalf("%s: tokens = %v", in, toks)
		}
		got := toks[0].Text
		if toks[0].Kind == StringToken {
			got = unquote(got)
		}
		if got != "café" {
			t.Fatalf("%s decoded to %q", in, got)
		}
	}
	toks := filterWhitespace(Tokenize([]byte(`#caf\e9`)))
	if len(toks) != 1 || toks[0].Kind != HashToken || toks[0].Text != "#café" {
		t.Fatalf("hash token = %v", toks)
	}
}
