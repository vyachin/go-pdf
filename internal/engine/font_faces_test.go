package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestFaceCacheDistinguishesFamilyLists guards RV4-43: ["a b"] and ["a", "b"]
// printed the same through %v, so the second request received the first
// family's face. Each list must resolve to its own face.
func TestFaceCacheDistinguishesFamilyLists(t *testing.T) {
	src := []byte(`<style>
		@font-face { font-family: "a b"; src: url(DejaVuSans.ttf) }
		@font-face { font-family: "a"; src: url(DejaVuSans-Bold.ttf) }
	</style>
	<p style="font-family: 'a b', a">x</p>
	<p style="font-family: a, b">y</p>`)
	res, err := Convert(context.Background(), src, Options{
		File:    "faces.html",
		FontDir: filepath.Join(repoRoot(), "testdata", "fonts"),
	})
	if err != nil {
		t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	baseFonts := map[string]bool{}
	pdf := string(res.PDF)
	for {
		i := strings.Index(pdf, "/BaseFont /")
		if i < 0 {
			break
		}
		rest := pdf[i+len("/BaseFont /"):]
		end := strings.IndexAny(rest, " \n/>")
		if end <= 0 {
			break
		}
		baseFonts[rest[:end]] = true
		pdf = rest[end:]
	}
	if len(baseFonts) < 2 {
		t.Fatalf("both family lists resolved to the same face: %v", baseFonts)
	}
}

// TestSystemFallbackIsCharged guards RV4-44: parsing a system font spends the
// same budget as an @font-face file, so an exhausted budget stops the fallback
// load instead of parsing it for free.
func TestSystemFallbackIsCharged(t *testing.T) {
	res, err := Convert(context.Background(), []byte("<p>x</p>"), Options{
		File:   "fallback.html",
		Limits: Limits{MaxOperations: 50},
	})
	if !hasCode(res.Diagnostics, "resource/limit") {
		t.Fatalf("want resource/limit, got err=%v diags:\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}
