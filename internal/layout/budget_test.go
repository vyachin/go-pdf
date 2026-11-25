package layout

import (
	"context"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestInsertOpsIsCharged guards the decoration copy: inserting operations into a
// page is charged by the number of operations copied, and a budget that cannot
// pay keeps the page unchanged.
func TestInsertOpsIsCharged(t *testing.T) {
	l := &layouter{ctx: Context{Budget: budget.New(context.Background(), 10)}}
	dst := make([]Op, 100)
	for i := range dst {
		dst[i].Kind = TextOpKind
	}
	got := l.insertOps(dst, 0, []Op{{Kind: FillOpKind}})
	if len(got) != len(dst) || got[0].Kind != TextOpKind {
		t.Fatalf("the stopped insert changed the page: %d ops", len(got))
	}
	if !l.diags.HasErrors() {
		t.Fatalf("the exhausted budget was not reported: %v", l.diags.Lines())
	}

	l = &layouter{ctx: Context{Budget: budget.New(context.Background(), 1000)}}
	out := l.insertOps(dst, 50, []Op{{Kind: FillOpKind}})
	if len(out) != 101 || out[50].Kind != FillOpKind {
		t.Fatalf("the insert placed %d operations, want 101 with the fill at 50", len(out))
	}
	if l.diags.HasErrors() {
		t.Fatalf("a within-budget insert reported diagnostics: %v", l.diags.Lines())
	}
}
