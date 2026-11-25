package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMissingAssetRootStopsReads guards the failed root: a BaseDir that does
// not exist is options/asset-root, and an absolute stylesheet outside it is
// never read.
func TestMissingAssetRootStopsReads(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside.css")
	if err := os.WriteFile(outside, []byte("p { color: #ff0000 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	src := []byte(fmt.Sprintf(`<link rel="stylesheet" href=%q><p>x</p>`, outside))
	res, err := Convert(context.Background(), src, Options{
		BaseDir:      missing,
		File:         "root.html",
		FallbackFont: testFontPath(t),
	})
	if err == nil || !hasCode(res.Diagnostics, "options/asset-root") || len(res.PDF) != 0 {
		t.Fatalf("want options/asset-root and no PDF, got err=%v diags:\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	for _, d := range res.Diagnostics {
		if d.Code == "css/link-load" || d.Code == "css/link-path" {
			t.Fatalf("a link code escaped: %s", d.String())
		}
	}
}

// TestMissingFontDirStopsReads guards the font root with the same code.
func TestMissingFontDirStopsReads(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`<p>x</p>`)
	res, err := Convert(context.Background(), src, Options{
		BaseDir:      dir,
		FontDir:      filepath.Join(dir, "missing"),
		File:         "root.html",
		FallbackFont: testFontPath(t),
	})
	if err == nil || !hasCode(res.Diagnostics, "options/asset-root") || len(res.PDF) != 0 {
		t.Fatalf("want options/asset-root and no PDF, got err=%v diags:\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}
