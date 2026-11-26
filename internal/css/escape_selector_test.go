package css

import "testing"

// TestSelectorEscapes guards the escape round trip: a decoded punctuation
// character in a selector names the character and never changes the selector's
// structure, while an unescaped space and dot keep their grammar meaning.
func TestSelectorEscapes(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		class string
		want  bool
	}{
		{"escaped dot", `.a\.b`, "a.b", true},
		{"escaped dot is not two classes", `.a\.b`, "a b", false},
		{"escaped space is one name", `.a\ b`, "a b", false},
		{"hex escaped dot", `.a\2e b`, "a.b", true},
		{"two plain classes", `.a.b`, "a b", true},
		{"one token is not two classes", `.a.b`, "a.b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := stringifySelector(Tokenize([]byte(tc.src)))
			sel, err := ParseSelector(text)
			if err != nil {
				t.Fatalf("%s -> %q: %v", tc.src, text, err)
			}
			doc := mustDoc(t, `<p class="`+tc.class+`">x</p>`)
			p := findElement(doc.Root, "p")
			if got := sel.Match(p); got != tc.want {
				t.Fatalf("%s against class %q matched %v, want %v", tc.src, tc.class, got, tc.want)
			}
		})
	}

	// The escaped dot is one class named "a.b", not two classes.
	sel, err := ParseSelector(stringifySelector(Tokenize([]byte(`.a\.b`))))
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Compounds) != 1 || len(sel.Compounds[0].Classes) != 1 || sel.Compounds[0].Classes[0] != "a.b" {
		t.Fatalf("compound = %+v, want one class %q", sel.Compounds, "a.b")
	}

	// An unescaped space still separates compounds.
	sel, err = ParseSelector(stringifySelector(Tokenize([]byte(`.a b`))))
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Compounds) != 2 {
		t.Fatalf("compounds = %d, want 2 for a descendant selector", len(sel.Compounds))
	}
	doc := mustDoc(t, `<div class="a"><b>x</b></div>`)
	if !sel.Match(findElement(doc.Root, "b")) {
		t.Fatal("the descendant selector no longer matches")
	}
}

// TestURLEscapes guards the url() escape handling: quoted and unquoted escapes
// decode, and an escaped parenthesis is part of the path instead of the end of
// the argument.
func TestURLEscapes(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"quoted space", `url("a\20 b.png")`, "a b.png"},
		{"escaped close", `url(a\)b.png)`, "a)b.png"},
		{"escaped open", `url(a\(b.png)`, "a(b.png"},
		{"unterminated", `url(a\(b.png`, "a(b.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			found := false
			for _, tok := range Tokenize([]byte(tc.src)) {
				if tok.Kind == URLToken {
					got = trimURLQuotes(tok.Text)
					found = true
				}
			}
			if !found {
				t.Fatalf("%s produced no URL token", tc.src)
			}
			if got != tc.want {
				t.Fatalf("%s decoded to %q, want %q", tc.src, got, tc.want)
			}
		})
	}

	st := DefaultStyle()
	if err := ApplyDecl(st, "background-image", Tokenize([]byte(`url("a\20 b.png")`)), 12000); err != nil {
		t.Fatalf("background-image: %v", err)
	}
	if st.BackgroundImage != "a b.png" {
		t.Fatalf("background image = %q, want %q", st.BackgroundImage, "a b.png")
	}
}
