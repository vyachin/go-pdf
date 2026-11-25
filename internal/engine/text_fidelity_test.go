package engine

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// TestCombiningMarkExtraction guards that a positioned mark still extracts as
// its combining sequence.
func TestCombiningMarkExtraction(t *testing.T) {
	res, err := Convert(context.Background(), []byte(`<p>сло́во</p>`), Options{
		File:         "mark.html",
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "сло́во") {
		t.Fatalf("extracted text = %q, want the combining sequence", text)
	}
}

// TestUnattachedMarkIsDiagnosed guards the diagnostic: without GPOS mark
// lookups the conversion stops and names the mark instead of drawing it at the
// pen.
func TestUnattachedMarkIsDiagnosed(t *testing.T) {
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	out := append([]byte(nil), data...)
	n := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if string(out[rec:rec+4]) == "GPOS" {
			binary.BigEndian.PutUint32(out[rec+12:rec+16], 0)
			break
		}
	}
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "no-gpos.ttf")
	if err := os.WriteFile(fontPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), []byte(`<p>сло́во</p>`), Options{
		File:         "mark.html",
		FallbackFont: fontPath,
	})
	if err == nil || !hasCode(res.Diagnostics, "text/glyph") {
		t.Fatalf("want text/glyph, got err=%v diags:\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !strings.Contains(res.Diagnostics.Lines()[0], "0301") {
		t.Fatalf("the diagnostic does not name the mark: %s", res.Diagnostics.Lines()[0])
	}
}
