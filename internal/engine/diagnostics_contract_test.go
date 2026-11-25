package engine

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestCreationDateRange guards the date option: an unrepresentable year is an
// option error, and a valid date still produces a strict-valid PDF.
func TestCreationDateRange(t *testing.T) {
	res, err := Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "date.html",
		FallbackFont: testFontPath(t),
		Date:         time.Date(10000, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	if err == nil || !hasCode(res.Diagnostics, "options/date") || len(res.PDF) != 0 {
		t.Fatalf("an out-of-range date was accepted: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	valid, err := Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "date.html",
		FallbackFont: testFontPath(t),
		Date:         time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("a valid date failed: %v\n%s", err, strings.Join(valid.Diagnostics.Lines(), "\n"))
	}
	validateWithPDFCPU(t, valid.PDF)
}
