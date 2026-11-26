package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// stripBidiControls removes the embedding marks pdftotext inserts around a
// right-to-left run. They are bidi formatting characters, not document text.
func stripBidiControls(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return -1
		}
		return r
	}, s)
}

// textOpXPositions returns the x origin of every text object in the content
// stream, in stream order. The stream keeps logical order, so the result
// follows the authored sequence.
func textOpXPositions(t *testing.T, pdf []byte) []float64 {
	t.Helper()
	re := regexp.MustCompile(`1 0 0 1 (-?[0-9.]+) -?[0-9.]+ Tm`)
	var out []float64
	for _, stream := range pdfdebug.ContentStreams(pdf) {
		for _, match := range re.FindAllSubmatch(stream, -1) {
			x, err := strconv.ParseFloat(string(match[1]), 64)
			if err != nil {
				t.Fatalf("parsing a text origin: %v", err)
			}
			out = append(out, x)
		}
	}
	return out
}

// TestHebrewConvertsInVisualOrder guards RV5-04 through the engine: the
// content stream paints the first logical word right to left, the text layer
// extracts the authored sequence, and an independent extractor returns it too.
func TestHebrewConvertsInVisualOrder(t *testing.T) {
	const src = `<p style="margin:0">שלום עולם</p>`
	res, err := Convert(context.Background(), []byte(src), Options{File: "hebrew.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("no PDF was produced")
	}
	text, err := extractText(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if text != "שלום עולם" {
		t.Fatalf("the text layer is %q, want the logical sequence %q", text, "שלום עולם")
	}
	// The first four text objects are the first logical word. Its glyphs must
	// move left as the word advances, which is what a right-to-left run means.
	x := textOpXPositions(t, res.PDF)
	if len(x) < 4 {
		t.Fatalf("%d text objects, want at least the four glyphs of the first word", len(x))
	}
	for i := 1; i < 4; i++ {
		if x[i] >= x[i-1] {
			t.Fatalf("glyph %d x = %g, glyph %d x = %g: the word is not painted right to left", i-1, x[i-1], i, x[i])
		}
	}

	// The independent extractor reads the document too. It reports a
	// right-to-left run, so compare the characters after its embedding marks.
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext is not installed")
	}
	path := filepath.Join(t.TempDir(), "hebrew.pdf")
	if err := os.WriteFile(path, res.PDF, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "-q", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	if got := strings.TrimSpace(stripBidiControls(string(out))); got != "שלום עולם" {
		t.Fatalf("pdftotext returned %q, want the logical sequence %q", got, "שלום עולם")
	}
}

// TestArabicIsRefusedThroughBothEntryPoints guards RV5-04: the joining script
// fails with text/unsupported-script in Convert and in Validate, and Convert
// returns no PDF. A Hebrew document still converts cleanly.
func TestArabicIsRefusedThroughBothEntryPoints(t *testing.T) {
	options := func() Options {
		return Options{File: "arabic.html", FallbackFont: testFontPath(t)}
	}
	for _, src := range []string{
		`<p>مرحبا بالعالم</p>`,
		`<p>hello مرحبا</p>`,
	} {
		res, err := Convert(context.Background(), []byte(src), options())
		if err == nil {
			t.Fatalf("Convert(%q) succeeded, want text/unsupported-script", src)
		}
		if !hasCode(res.Diagnostics, "text/unsupported-script") {
			t.Fatalf("Convert(%q) diagnostics = %v, want text/unsupported-script", src, res.Diagnostics.Lines())
		}
		if len(res.PDF) != 0 {
			t.Fatalf("Convert(%q) returned %d PDF bytes, want no partial PDF", src, len(res.PDF))
		}
		diags, err := Validate(context.Background(), []byte(src), options())
		if err != nil {
			t.Fatalf("Validate(%q): %v", src, err)
		}
		if !hasCode(diags, "text/unsupported-script") {
			t.Fatalf("Validate(%q) diagnostics = %v, want text/unsupported-script", src, diags.Lines())
		}
		for _, d := range diags {
			if d.Code == "text/unsupported-script" && (d.Severity != diag.Error || d.Pos.Line == 0 || d.Element == "") {
				t.Fatalf("the refusal is not an attributed error: %s", d.String())
			}
		}
	}

	// The refusal does not depend on a configured font: a font with isolated
	// Arabic forms must not turn the document into a success.
	res, err := Convert(context.Background(), []byte(`<p>مرحبا</p>`), Options{File: "arabic.html"})
	if err == nil || !hasCode(res.Diagnostics, "text/unsupported-script") {
		t.Fatalf("Arabic without a fallback font: err=%v diagnostics=%v, want text/unsupported-script", err, res.Diagnostics.Lines())
	}

	hebrew := `<p>שלום עולם</p>`
	res, err = Convert(context.Background(), []byte(hebrew), options())
	if err != nil {
		t.Fatalf("Hebrew must convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("Hebrew produced no PDF")
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("Hebrew produced diagnostics: %v", res.Diagnostics.Lines())
	}
}

// TestMixedTextLayerKeepsTheAuthoredSequence guards that reordering only the
// visual placement never reorders the text layer: a mixed line extracts the
// source sequence, with no character duplicated or dropped.
func TestMixedTextLayerKeepsTheAuthoredSequence(t *testing.T) {
	const src = `<p style="margin:0">abc שלום 123</p>`
	res, err := Convert(context.Background(), []byte(src), Options{File: "mixed.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	text, err := extractText(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if text != "abc שלום 123" {
		t.Fatalf("the text layer is %q, want %q", text, "abc שלום 123")
	}
}
