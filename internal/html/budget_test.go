package html

import (
	"context"
	"errors"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestPathIsCharged guards the diagnostic path: every segment and class costs a
// unit, and an exhausted budget stops the string instead of building it for
// free.
func TestPathIsCharged(t *testing.T) {
	root := &Node{Type: DocumentNode}
	body := &Node{Type: ElementNode, Tag: "body", Parent: root}
	div := &Node{Type: ElementNode, Tag: "div", Parent: body, Attrs: []Attr{{Key: "class", Value: "a b c d e"}}}
	root.Children = []*Node{body}
	body.Children = []*Node{div}

	guard := budget.New(context.Background(), 3)
	attachBudget(root, guard)
	if got := div.Path(); got != "" {
		t.Fatalf("path = %q, want it stopped by the budget", got)
	}
	if !errors.Is(guard.Err(), budget.ErrExceeded) {
		t.Fatalf("the path did not charge the guard: %v", guard.Err())
	}

	attachBudget(root, nil)
	if got := div.Path(); got != "body > div.a.b.c.d.e" {
		t.Fatalf("path = %q, want body > div.a.b.c.d.e", got)
	}
}

// TestParseAttachesTheGuardToTheTree keeps the parser honest: the guard must
// reach every node, or Path would be free in a real conversion.
func TestParseAttachesTheGuardToTheTree(t *testing.T) {
	guard := budget.New(context.Background(), 0)
	doc, err := ParseWithOptions([]byte(`<p id="x">text</p>`), ParseOptions{Budget: guard})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Root.guard != guard {
		t.Fatal("the root node did not receive the parser's guard")
	}
}
