package css

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestNewlineStylesheetStopsAtTheLineIndexCap guards RV5-12: a 7 MiB
// stylesheet made of newlines is rejected as resource/limit once the bounded
// line index is full, instead of allocating one index entry per newline.
func TestNewlineStylesheetStopsAtTheLineIndexCap(t *testing.T) {
	src := bytes.Repeat([]byte{'\n'}, 7<<20)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	sheet := ParseSheet(SheetOptions{File: "newlines.css"}, src)
	runtime.ReadMemStats(&after)
	if !hasCode(sheet.Problems, "resource/limit") {
		t.Fatalf("a %d-line stylesheet reported %+v, want resource/limit", len(src), sheet.Problems)
	}
	if len(sheet.Rules) != 0 {
		t.Fatalf("the stopped stylesheet kept %d rules", len(sheet.Rules))
	}
	if got := sheet.lines.Len(); got != budget.MaxLineStarts {
		t.Fatalf("the line index stored %d starts, want %d", got, budget.MaxLineStarts)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 8<<20 {
		t.Fatalf("the stopped stylesheet allocated %d bytes, want a small multiple of the index", alloc)
	}
}

// TestPlainStylesheetBelowTheLineCapSucceeds guards the sibling: the same 7 MiB
// in one comment is ordinary CSS and must still parse.
func TestPlainStylesheetBelowTheLineCapSucceeds(t *testing.T) {
	src := append([]byte("/*"), bytes.Repeat([]byte{'x'}, 7<<20)...)
	src = append(src, "*/"...)
	sheet := ParseSheet(SheetOptions{File: "comment.css"}, src)
	if hasCode(sheet.Problems, "resource/limit") {
		t.Fatalf("ordinary CSS was limited: %+v", sheet.Problems)
	}
	if len(sheet.Problems) != 0 {
		t.Fatalf("ordinary CSS reported %+v", sheet.Problems)
	}
	if got := sheet.Position(len(src) - 1); got.Line != 1 || got.Col != len(src) {
		t.Fatalf("the last position is %d:%d, want 1:%d", got.Line, got.Col, len(src))
	}
}

// TestPositionPastCappedIndex guards the scan fallback: a diagnostic on the
// 70,000th line after the capped number of stored starts still names the right
// line and column, and the scan is charged to the work guard.
func TestPositionPastCappedIndex(t *testing.T) {
	const lines = 70_000
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString("x\n")
	}
	guard := budget.New(context.Background(), 0)
	sheet := ParseSheet(SheetOptions{File: "long.css", Guard: guard}, []byte(b.String()))
	if !hasCode(sheet.Problems, "resource/limit") {
		t.Fatalf("the long stylesheet reported %+v, want resource/limit", sheet.Problems)
	}
	offset := (lines - 1) * 2 // start of the 70,000th line
	before := guard.Used()
	pos := sheet.Position(offset)
	if pos.Line != lines || pos.Col != 1 {
		t.Fatalf("the position past the cap is %d:%d, want %d:1", pos.Line, pos.Col, lines)
	}
	if got := guard.Used() - before; got == 0 {
		t.Fatal("the past-cap position scan was not charged")
	}
	if pos = sheet.Position(offset + 1); pos.Line != lines || pos.Col != 2 {
		t.Fatalf("the column after the cap is %d:%d, want %d:2", pos.Line, pos.Col, lines)
	}
}
