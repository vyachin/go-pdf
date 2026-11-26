package engine

import (
	"bytes"
	"context"
	"testing"
)

// TestSaturatedAdvanceIsDiagnosedAndReachesOutput guards RV5-09 end to end: a
// font size whose scaled advances leave the int32 range used to wrap the run
// width negative, so the layout/overflow warning disappeared and the document
// was written silently. The conversion now reports the clamp once and still
// produces a structurally valid PDF.
func TestSaturatedAdvanceIsDiagnosedAndReachesOutput(t *testing.T) {
	src := []byte(`<p style="font-size:2000000pt;margin:0">AB</p>`)
	res, err := Convert(context.Background(), src, Options{
		FallbackFont: testFontPath(t),
		File:         "advance-overflow.html",
	})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, joinDiagnostics(res))
	}
	if !bytes.HasPrefix(res.PDF, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF: %q", res.PDF)
	}
	reported := 0
	for _, d := range res.Diagnostics {
		if d.Code == "layout/overflow" {
			reported++
		}
	}
	if reported != 1 {
		t.Fatalf("the saturated advance reported %d layout/overflow diagnostics, want 1:\n%s",
			reported, joinDiagnostics(res))
	}
	validateWithPDFCPU(t, res.PDF)
}

// joinDiagnostics renders the conversion's diagnostics for a failure message.
func joinDiagnostics(res Result) string {
	var b bytes.Buffer
	for _, d := range res.Diagnostics {
		b.WriteString(d.String())
		b.WriteByte('\n')
	}
	return b.String()
}
