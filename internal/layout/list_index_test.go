package layout

import (
	"context"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// listWork returns the guard spend of laying out src under a fresh budget.
func listWork(t *testing.T, src string, max int) (uint64, []string) {
	t.Helper()
	h := newHarness(t)
	guard := budget.New(context.Background(), max)
	h.ctx.Budget = guard
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	return guard.Used(), diags.Lines()
}

// listDoc builds a document with n empty list items.
func listDoc(n int) string {
	var b strings.Builder
	b.WriteString("<ol>")
	for i := 0; i < n; i++ {
		b.WriteString("<li></li>")
	}
	b.WriteString("</ol>")
	return b.String()
}

// TestListIndexingIsLinear guards the review reproduction: indexing 40,000
// items used to rescan every preceding sibling, so the work grew quadratically.
// Doubling the item count may at most roughly double the guard spend.
func TestListIndexingIsLinear(t *testing.T) {
	small, _ := listWork(t, listDoc(2000), 0)
	large, _ := listWork(t, listDoc(4000), 0)
	if small == 0 || large == 0 {
		t.Fatalf("no work was charged: small=%d large=%d", small, large)
	}
	// Linear indexing doubles the spend; the old quadratic scan would
	// quadruple it. A margin keeps the assertion stable without timing.
	if large > 3*small {
		t.Fatalf("doubling the items took %d units after %d: indexing is not linear", large, small)
	}
}

// TestLongListFitsTheDefaultBudget guards the review reproduction at the
// documented size: 40,000 items under the default operation count must not
// exhaust the guard through indexing alone.
func TestLongListFitsTheDefaultBudget(t *testing.T) {
	h := newHarness(t)
	h.ctx.Budget = budget.New(context.Background(), 20_000_000)
	doc, diags := h.runRaw(listDoc(40_000), h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("a long shallow list failed: %v", diags.Lines())
	}
	if len(doc.Pages) == 0 {
		t.Fatal("the long list produced no pages")
	}
}

// TestListNumberingSurvivesIndexing guards the numbering contract: start, a
// non-list sibling and nesting keep their labels.
func TestListNumberingSurvivesIndexing(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<ol start="5"><li>A</li><div>between</div><li>B</li></ol>`)
	texts := strings.Join(textOpTexts(doc), "")
	for _, want := range []string{"5.", "A", "6.", "B", "between"} {
		if !strings.Contains(texts, want) {
			t.Fatalf("the output is missing %q: %q", want, texts)
		}
	}
	nested := h.run(`<ol><li>OUTER<ol><li>INNER</li></ol></li></ol>`)
	nestedText := strings.Join(textOpTexts(nested), "")
	if !strings.Contains(nestedText, "1.") || !strings.Contains(nestedText, "INNER") {
		t.Fatalf("nested numbering broke: %q", nestedText)
	}
}

// TestListIndexingExhaustsTheBudget guards accounting: the traversal is charged,
// so a budget too small for it stops with resource/limit.
func TestListIndexingExhaustsTheBudget(t *testing.T) {
	work, diags := listWork(t, listDoc(200), 50)
	if work == 0 {
		t.Fatal("the traversal was not charged")
	}
	if !contains(diags, "resource/limit") {
		t.Fatalf("no resource/limit: %v", diags)
	}
}

// contains reports whether any diagnostic line contains want.
func contains(lines []string, want string) bool {
	for _, l := range lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}
