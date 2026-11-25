package css

import (
	"strings"
	"testing"
)

// TestTextDecorationValues guards the parser: the supported keywords and their
// combination are accepted, and every other value is a profile error.
func TestTextDecorationValues(t *testing.T) {
	cases := map[string]Decoration{
		"none":                   DecorateNone,
		"underline":              DecorateUnderline,
		"line-through":           DecorateLineThrough,
		"underline line-through": DecorateUnderline | DecorateLineThrough,
		"LINE-THROUGH":           DecorateLineThrough,
	}
	for value, want := range cases {
		st := NewStyle(nil)
		if err := ApplyDecl(st, "text-decoration", tokenizeValue(t, value), 12000); err != nil {
			t.Fatalf("%q rejected: %v", value, err)
		}
		if st.TextDecoration != want {
			t.Fatalf("%q = %v, want %v", value, st.TextDecoration, want)
		}
	}
	for _, value := range []string{"overline", "blink", "underline red", "none underline", "underline underline"} {
		st := NewStyle(nil)
		err := ApplyDecl(st, "text-decoration", tokenizeValue(t, value), 12000)
		if err == nil {
			t.Fatalf("%q was accepted", value)
		}
		if !strings.Contains(err.Error(), "text-decoration") {
			t.Fatalf("%q: the error does not name the property: %v", value, err)
		}
	}
}

// tokenizeValue turns a property value into tokens, failing the test on error.
func tokenizeValue(t *testing.T, value string) []Token {
	t.Helper()
	return Tokenize([]byte(value))
}

// TestTextDecorationIsInherited guards propagation: a decorated element's
// children inherit the decoration so their text is painted decorated too.
func TestTextDecorationIsInherited(t *testing.T) {
	doc := mustDoc(t, `<p><u>under <em>emphasis</em></u></p>`)
	em := findElement(doc.Root, "em")
	if em == nil {
		t.Fatal("no em")
	}
	p := findElement(doc.Root, "p")
	u := findElement(doc.Root, "u")
	cascade := NewCascadeForDocument(DefaultStylesheet())
	pStyle := cascade.Compute(p, nil)
	uStyle := cascade.Compute(u, pStyle)
	emStyle := cascade.Compute(em, uStyle)
	if uStyle.TextDecoration != DecorateUnderline {
		t.Fatalf("u = %v, want underline", uStyle.TextDecoration)
	}
	if emStyle.TextDecoration != DecorateUnderline {
		t.Fatalf("inherited = %v, want underline", emStyle.TextDecoration)
	}
}

// TestDecorationElementsHaveDefaultRules guards the engine stylesheet: each new
// element resolves to the decoration the profile documents.
func TestDecorationElementsHaveDefaultRules(t *testing.T) {
	doc := mustDoc(t, `<p><u>a</u><s>b</s><del>c</del><ins>d</ins></p><blockquote>q</blockquote>`)
	cascade := NewCascadeForDocument(DefaultStylesheet())
	p := findElement(doc.Root, "p")
	pStyle := cascade.Compute(p, nil)
	for tag, want := range map[string]Decoration{
		"u": DecorateUnderline, "ins": DecorateUnderline,
		"s": DecorateLineThrough, "del": DecorateLineThrough,
	} {
		n := findElement(doc.Root, tag)
		if n == nil {
			t.Fatalf("no %s", tag)
		}
		if got := cascade.Compute(n, pStyle).TextDecoration; got != want {
			t.Errorf("%s = %v, want %v", tag, got, want)
		}
	}
	q := findElement(doc.Root, "blockquote")
	if q == nil {
		t.Fatal("no blockquote")
	}
	st := cascade.Compute(q, nil)
	if st.Display != DisplayBlock {
		t.Fatalf("blockquote display = %v, want block", st.Display)
	}
	if st.Margin[3].Kind != MP || st.Margin[3].Value == 0 {
		t.Fatalf("blockquote has no left indent: %v", st.Margin[3])
	}
}
