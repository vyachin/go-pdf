package css

import (
	"strings"
	"testing"
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
