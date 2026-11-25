// Package testfont locates the font the test suite converts with, in one place.
//
// Go cannot import test files, so the helper lives in a normal package. Only
// tests import it: the production conversion path never reads a test setting or
// this package, which is what keeps a deployment's rendering independent of the
// environment.
package testfont

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
)

// Candidates returns the font paths the suite searches, in order: the
// PDFTEST_FONT override, the bundled font and the system fonts. The order is
// defined here once, so every suite finds the same face.
func Candidates() []string {
	var out []string
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		out = append(out, p)
	}
	// The bundled font keeps results identical on every platform and in CI.
	if bundled, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf")); err == nil {
		out = append(out, bundled)
	}
	if runtime.GOOS == "darwin" {
		out = append(out,
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
			"/Library/Fonts/Arial.ttf",
		)
	}
	out = append(out,
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
	)
	return out
}

// Path returns the first candidate that exists and parses.
func Path(t testing.TB) string {
	t.Helper()
	for _, path := range Candidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if _, err := font.Parse(data); err != nil {
			t.Logf("skipping %s: %v", path, err)
			continue
		}
		return path
	}
	t.Skip("no TrueType font available; set PDFTEST_FONT")
	return ""
}

// Load returns the parsed test font.
func Load(t testing.TB) *font.Font {
	t.Helper()
	path := Path(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
