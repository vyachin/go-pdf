package html

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestLineIndexIsBoundedAndResolvesPastTheCap guards RV5-12 for HTML: a
// document with more lines than MaxLineStarts stores only the capped index, but
// a diagnostic on the 70,000th line still names the right line and column and
// the scan is charged.
func TestLineIndexIsBoundedAndResolvesPastTheCap(t *testing.T) {
	const lines = budget.MaxLineStarts + 5_000
	var src strings.Builder
	src.WriteString("<p>")
	for i := 0; i < lines; i++ {
		src.WriteString("x\n")
	}
	src.WriteString("</p>")

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	guard := budget.New(context.Background(), 0)
	doc, err := ParseWithOptions([]byte(src.String()), ParseOptions{File: "long.html", Budget: guard})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("a document above the line cap failed: %v", err)
	}
	if !doc.lines.Capped() {
		t.Fatal("the line index is not capped")
	}
	if got := doc.lines.Len(); got != budget.MaxLineStarts {
		t.Fatalf("the line index stored %d starts, want %d", got, budget.MaxLineStarts)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 8<<20 {
		t.Fatalf("parsing the document allocated %d bytes, want a bounded index", alloc)
	}
	// The 70,000th line starts after "<p>" and 69,999 "x\n" pairs.
	offset := 3 + (lines-1)*2
	spent := guard.Used()
	pos := doc.Position(offset)
	if pos.Line != lines || pos.Col != 1 {
		t.Fatalf("the position past the cap is %d:%d, want %d:1", pos.Line, pos.Col, lines)
	}
	if guard.Used() == spent {
		t.Fatal("the past-cap position scan was not charged")
	}
}
