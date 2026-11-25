package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/font"
)

// openTypeFontDir is the corpus directory that holds the Barlow OpenType fonts.
func openTypeFontDir() string {
	return filepath.Join(repoRoot(), "testdata", "weasyprint", "ticket")
}

// TestOpenTypeFontFaceIsUsed converts a document that declares OpenType fonts
// with CFF outlines in two weights and checks the result end to end: the PDF
// embeds TrueType programs, the text layer extracts the document text, and an
// independent validator accepts the file.
//
// The text includes "8" and "r", the two characters whose charstrings use the
// flex1 operator that the Go project's decoder cannot read.
func TestOpenTypeFontFaceIsUsed(t *testing.T) {
	src := `<style>
		@font-face { font-family: Barlow; src: url(barlowcondensed-regular.otf) }
		@font-face { font-family: Barlow; font-weight: 700; src: url(barlowcondensed-bold.otf) }
		body { font-family: Barlow }
		b { font-weight: 700 }
	</style>
	<p>Barlow 128</p>
	<p><b>B8 r128</b></p>`
	res, err := Convert(context.Background(), []byte(src), Options{
		File:    "otf.html",
		FontDir: openTypeFontDir(),
	})
	if err != nil {
		t.Fatalf("convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	for _, want := range []string{"/Subtype /Type0", "/CIDFontType2", "/FontFile2", "/CIDToGIDMap"} {
		if !bytes.Contains(res.PDF, []byte(want)) {
			t.Fatalf("the PDF does not contain %q", want)
		}
	}
	// One embedded program per declared weight, and both are TrueType: a CFF
	// program would appear as /FontFile3.
	if got := bytes.Count(res.PDF, []byte("/FontFile2")); got != 2 {
		t.Fatalf("%d TrueType font programs embedded, want 2", got)
	}
	if bytes.Contains(res.PDF, []byte("/FontFile3")) {
		t.Fatal("the PDF embeds a CFF program instead of the converted TrueType one")
	}
	text, err := extractText(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Barlow 128", "B8 r128"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the extracted text does not contain %q: %q", want, text)
		}
	}
	validateWithPDFCPU(t, res.PDF)
}

// TestOpenTypeFontFaceIsDeterministic checks that the same document converts to
// the same bytes twice: a float in the conversion, a map iteration order or a
// random value would show up here.
func TestOpenTypeFontFaceIsDeterministic(t *testing.T) {
	src := []byte(`<style>
		@font-face { font-family: Barlow; src: url(barlowcondensed-regular.otf) }
		body { font-family: Barlow }
	</style><p>Barlow 128 g r</p>`)
	opts := Options{File: "otf.html", FontDir: openTypeFontDir()}
	first, err := Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.PDF, second.PDF) {
		t.Fatal("two conversions of the same OpenType document differ")
	}
}

// TestOpenTypeFontRejections checks the three formats the profile and the loader
// refuse, each with a diagnostic that names the format.
func TestOpenTypeFontRejections(t *testing.T) {
	fontDir := t.TempDir()
	writeFont := func(name string, version uint32) {
		data := make([]byte, 64)
		data[0] = byte(version >> 24)
		data[1] = byte(version >> 16)
		data[2] = byte(version >> 8)
		data[3] = byte(version)
		if err := os.WriteFile(filepath.Join(fontDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFont("font.woff", 0x774F4646)
	writeFont("font.woff2", 0x774F4632)

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "WOFF",
			src:  `<style>@font-face { font-family: X; src: url(font.woff) } p { font-family: X }</style><p>x</p>`,
			want: "WOFF web fonts are not supported",
		},
		{
			name: "WOFF2",
			src:  `<style>@font-face { font-family: X; src: url(font.woff2) } p { font-family: X }</style><p>x</p>`,
			want: "WOFF2 web fonts are not supported",
		},
		{
			name: "remote",
			src:  `<style>@font-face { font-family: X; src: url(https://example.com/font.otf) } p { font-family: X }</style><p>x</p>`,
			want: "remote fonts are not supported",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(tc.src), Options{File: "reject.html", FontDir: fontDir})
			if err == nil {
				t.Fatalf("the document converted; diagnostics: %v", res.Diagnostics.Lines())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %q, want a diagnostic containing %q", err.Error(), tc.want)
			}
		})
	}
}

// TestOpenTypeFontLimitIsResourceLimit checks the engine's mapping: a conversion
// that exceeds a documented font bound becomes a resource/limit diagnostic, not
// an unavailable font.
func TestOpenTypeFontLimitIsResourceLimit(t *testing.T) {
	err := loadFontError("dense.otf", fmt.Errorf("%w: glyph 3 needs 9000 points", font.ErrLimit))
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("a conversion limit is not a resource/limit diagnostic: %v", err)
	}
	for _, want := range []string{"dense.otf", "glyph 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the diagnostic does not name %q: %v", want, err)
		}
	}
	// A damaged font keeps the parser's own explanation and is not a limit.
	broken := loadFontError("broken.otf", fmt.Errorf("font: CFF table is truncated"))
	if errors.Is(broken, budget.ErrExceeded) {
		t.Fatalf("a damaged font was reported as a limit: %v", broken)
	}
	if !strings.Contains(broken.Error(), "CFF table is truncated") {
		t.Fatalf("the parser's explanation was lost: %v", broken)
	}
}
