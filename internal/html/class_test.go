package html_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/html"
)

// TestClassSelectorNoBreakSpace covers the class-token rule through a selector:
// a no-break space belongs to its token, so ".a" does not match "a\u00a0b" and
// a selector for the whole token does.
func TestClassSelectorNoBreakSpace(t *testing.T) {
	doc, err := html.Parse([]byte(`<div class="a&#160;b">x</div>`), "nbsp.html")
	if err != nil {
		t.Fatal(err)
	}
	div := doc.Body().FirstElementChild()
	if div == nil {
		t.Fatal("no div")
	}
	tokens := div.Class()
	if len(tokens) != 1 || tokens[0] != "a\u00a0b" {
		t.Fatalf("class tokens = %q, want the single token %q", tokens, "a\u00a0b")
	}
	cases := []struct {
		selector string
		want     bool
	}{
		{".a", false},
		{".b", false},
		{".a\\a0 b", true}, // the escaped no-break space names the whole token
	}
	for _, tc := range cases {
		sel, err := css.ParseSelector(tc.selector)
		if err != nil {
			t.Fatalf("%q: %v", tc.selector, err)
		}
		if got := sel.Match(div); got != tc.want {
			t.Fatalf("%q.Match(div) = %v, want %v", tc.selector, got, tc.want)
		}
	}
}

// TestFiftySelectorsSplitClassesOnce guards the cache: fifty class selectors
// matched against one element with twenty thousand classes read the token slice
// split on the first match instead of splitting the attribute again.
func TestFiftySelectorsSplitClassesOnce(t *testing.T) {
	const total = 20_000
	classes := make([]string, total)
	for i := range classes {
		classes[i] = fmt.Sprintf("c%d", i)
	}
	src := `<div class="` + strings.Join(classes, " ") + `">x</div>`
	doc, err := html.Parse([]byte(src), "classes.html")
	if err != nil {
		t.Fatal(err)
	}
	div := doc.Body().FirstElementChild()
	if div == nil {
		t.Fatal("no div")
	}
	first := div.Class()
	if len(first) != total {
		t.Fatalf("class tokens = %d, want %d", len(first), total)
	}
	for i := 0; i < 50; i++ {
		name := first[i*total/50]
		sel, err := css.ParseSelector("." + name)
		if err != nil {
			t.Fatalf(".%s: %v", name, err)
		}
		if !sel.Match(div) {
			t.Fatalf(".%s did not match the element", name)
		}
	}
	again := div.Class()
	if len(again) != total {
		t.Fatalf("class tokens = %d after matching, want %d", len(again), total)
	}
	if &first[0] != &again[0] {
		t.Fatal("the class attribute was split again by repeated selector matching")
	}
}
