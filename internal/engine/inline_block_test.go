package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// TestWrappedBlockImageIsEmbedded guards the review reproduction through the
// public API: a block-level image inside an inline wrapper produces an image
// object, and the missing-source variant is an img/load error in both entry
// points.
func TestWrappedBlockImageIsEmbedded(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "logo.png"))
	src := []byte(`<p><span><img src="logo.png" style="display:block"></span></p>`)
	opts := Options{BaseDir: dir, File: "wrapped.html", FallbackFont: testFontPath(t)}
	res, err := Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(res.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("the wrapped image produced %d image objects, want 1", n)
	}
	// The independent extractor sees the page as well.
	if pages, err := pdfdebug.Pages(res.PDF); err != nil || len(pages) != 1 {
		t.Fatalf("the extractor returned %d pages, err=%v", len(pages), err)
	}
}

// TestWrappedMissingImageIsReported guards asset validation through the split:
// a missing image inside an inline wrapper is an img/load error from both entry
// points.
func TestWrappedMissingImageIsReported(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`<p><span><img src="missing.png" style="display:block"></span></p>`)
	opts := Options{BaseDir: dir, File: "missing.html", FallbackFont: testFontPath(t)}
	res, err := Convert(context.Background(), src, opts)
	if err == nil || !hasCode(res.Diagnostics, "img/load") {
		t.Fatalf("Convert: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	diags, verr := Validate(context.Background(), src, opts)
	if verr != nil {
		t.Fatal(verr)
	}
	if !hasCode(diags, "img/load") {
		t.Fatalf("Validate missed the missing image: %v", diags.Lines())
	}
}
