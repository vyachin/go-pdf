package layout

import (
	"context"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/profile"
)

// nestedAvoid builds depth nested break-inside: avoid blocks, each with its own
// paragraph of words words. Every level measures its whole subtree, so the
// document used to re-split and re-shape every descendant once per nesting
// level, which is O(depth²).
func nestedAvoid(depth, words int) string {
	para := "<p>" + strings.Repeat("word ", words) + "</p>"
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString(`<div style="break-inside: avoid">`)
		b.WriteString(para)
	}
	for i := 0; i < depth; i++ {
		b.WriteString("</div>")
	}
	return b.String()
}

// runBounded lays out src under guard and returns the spend and diagnostics.
func (h *harness) runBounded(src string, guard *budget.Guard) (uint64, diag.List) {
	h.t.Helper()
	doc, err := html.Parse([]byte(src), "nested.html")
	if err != nil {
		h.t.Fatal(err)
	}
	sheets, _ := profile.CollectStylesheets(doc, profile.Options{})
	cascade := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	ctx := h.ctx
	ctx.Cascade = cascade
	ctx.PageStyle = ParsePageStyle(sheets, h.ctx.PageSize, h.ctx.Margin)
	ctx.Budget = guard
	_, layoutDiags := Run(doc, ctx)
	return guard.Used(), layoutDiags
}

// TestNestedAvoidBuildsLinesOncePerWidth guards RV4-03: the lines of a box are
// built once per content width and reused by the measurement, so the work of a
// deeply nested break-inside: avoid document grows with its text, not with the
// square of its depth. The assertion is the recorded spend, never a duration.
func TestNestedAvoidBuildsLinesOncePerWidth(t *testing.T) {
	h := newHarness(t)
	spend := func(depth int) uint64 {
		used, diags := h.runBounded(nestedAvoid(depth, 120), budget.New(context.Background(), 0))
		if diags.HasErrors() {
			t.Fatalf("depth %d: %v", depth, diags.Lines())
		}
		return used
	}
	shallow := spend(40)
	deep := spend(160)
	if deep > shallow*8 {
		t.Fatalf("four times the nesting depth spent %d units against %d: the lines are rebuilt per measurement", deep, shallow)
	}

	// The cached lines must not change the result: the nested document paints
	// the same characters as the same paragraphs without the nesting, and no
	// page is empty. The engine-level test additionally compares the two PDFs
	// byte for byte, where an avoid box that fits cannot move a paragraph.
	const depth, words = 40, 120
	nested := h.run(nestedAvoid(depth, words))
	flat := h.run(strings.Repeat("<p>"+strings.Repeat("word ", words)+"</p>", depth))
	var nestedAll, flatAll strings.Builder
	for i := 1; i <= len(nested.Pages); i++ {
		if strings.TrimSpace(pageText(nested, i)) == "" {
			t.Fatalf("page %d is empty", i)
		}
		nestedAll.WriteString(pageText(nested, i))
	}
	for i := 1; i <= len(flat.Pages); i++ {
		flatAll.WriteString(pageText(flat, i))
	}
	if a, b := len([]rune(nestedAll.String())), len([]rune(flatAll.String())); a != b {
		t.Fatalf("the nested document paints %d characters, the flat one %d", a, b)
	}
}

// TestNestedAvoidStopsOnASmallBudget guards the charging half of RV4-03: the
// repeated measurement is charged, so a small budget stops it with one
// resource/limit instead of spending minutes of CPU first.
func TestNestedAvoidStopsOnASmallBudget(t *testing.T) {
	h := newHarness(t)
	const max = 2000
	used, diags := h.runBounded(nestedAvoid(160, 120), budget.New(context.Background(), max))
	if !hasDiagCode(diags, "resource/limit") {
		t.Fatalf("the small budget was not reported: %v", diags.Lines())
	}
	if used > max*4 {
		t.Fatalf("the stopped layout spent %d units against a %d budget", used, max)
	}
}

// hasDiagCode reports whether the list contains the code.
func hasDiagCode(list diag.List, code string) bool {
	for _, d := range list {
		if string(d.Code) == code {
			return true
		}
	}
	return false
}
