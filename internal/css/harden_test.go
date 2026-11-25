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
