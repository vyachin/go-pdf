package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNewlineStylesheetIsBounded guards RV5-12 end to end: a linked 7 MiB
// stylesheet made of newlines must stop with resource/limit and keep the
// allocation a small multiple of the index, not of the input.
func TestNewlineStylesheetIsBounded(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(adversarialDir(), "p3-newline-stylesheet.html"))
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	res, err := Convert(context.Background(), src, Options{
		BaseDir:      adversarialDir(),
		File:         "newlines.html",
		FallbackFont: testFontPath(t),
	})
	runtime.ReadMemStats(&after)
	if !hasCode(res.Diagnostics, "resource/limit") {
		t.Fatalf("the newline stylesheet did not report resource/limit: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; !raceEnabled && alloc > 32<<20 {
		t.Fatalf("the stopped conversion allocated %d bytes", alloc)
	}
}
