package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestEscapedSelectorAndImagePath guards the end-to-end escape round trip: a
// class name containing a dot is selected with an escaped dot, and an escaped
// space in the background-image URL names the file "a b.png".
func TestEscapedSelectorAndImagePath(t *testing.T) {
	dir := t.TempDir()
	png, err := os.ReadFile(filepath.Join("..", "..", "testdata", "adversarial", "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a b.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`<style>.a\.b { background-image: url("a\20 b.png") }</style><p class="a.b" style="height:20pt">x</p>`)
	res, err := Convert(context.Background(), src, Options{
		BaseDir:      dir,
		File:         "escaped.html",
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if hasCode(res.Diagnostics, "img/load") {
		t.Fatalf("the escaped image path did not resolve:\n%s", res.Diagnostics.Lines())
	}
	if !bytes.Contains(res.PDF, []byte("/Subtype /Image")) {
		t.Fatal("the escaped background image is missing from the PDF")
	}
}
