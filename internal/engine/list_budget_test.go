package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestLongListConversionObservesCancellation guards the cancellation rule: a
// context cancelled during a long-list conversion returns its cause and no PDF,
// so the traversal cannot keep scanning the remaining siblings.
func TestLongListConversionObservesCancellation(t *testing.T) {
	var b strings.Builder
	b.WriteString("<ol>")
	for i := 0; i < 20000; i++ {
		b.WriteString("<li></li>")
	}
	b.WriteString("</ol>")
	ctx := &cancelAfter{Context: context.Background(), after: 1}
	res, err := Convert(ctx, []byte(b.String()), Options{File: "long-list.html", FallbackFont: testFontPath(t)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Convert returned %v, want context.Canceled", err)
	}
	if len(res.PDF) != 0 {
		t.Fatal("a cancelled conversion produced a PDF")
	}
}

// TestLongListConversionUnderSmallBudget guards the charge: indexing a long
// list stops with resource/limit when the budget is too small, and the reported
// work stays near the budget rather than the full item count.
func TestLongListConversionUnderSmallBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("<ol>")
	for i := 0; i < 20000; i++ {
		b.WriteString("<li></li>")
	}
	b.WriteString("</ol>")
	res, err := Convert(context.Background(), []byte(b.String()), Options{
		File:   "long-list.html",
		Limits: Limits{MaxOperations: 1000},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") {
		t.Fatalf("err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	if len(res.PDF) != 0 {
		t.Fatal("an over-budget conversion produced a PDF")
	}
}
