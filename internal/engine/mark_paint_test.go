package engine

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
	"github.com/vyachin/go-pdf/internal/testfont"
	"github.com/vyachin/go-pdf/internal/text"
)

// TestCombiningMarkKeepsExtractedSequence guards the independent extraction of
// shaped text with a mark: body text and a margin box both keep the combining
// sequence, so a copy from the PDF carries the original characters.
func TestCombiningMarkKeepsExtractedSequence(t *testing.T) {
	const word = "сло\u0301во"
	src := []byte(`<style>@page { @bottom-center { content: "` + word + `" } } p { font-size: 10pt }</style><p>` + word + `</p>`)
	res, err := Convert(context.Background(), src, Options{File: "marks.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfdebug.Pages(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	extracted := strings.Join(pages, "\n")
	if got := strings.Count(extracted, word); got != 2 {
		t.Fatalf("the extracted text contains the combining sequence %d times, want 2: %q", got, extracted)
	}
	validateWithPDFCPU(t, res.PDF)
}

// textObjectOrigins returns the origin of every painted text object in the
// content streams, in paint order and in millipoints. A glyph carrying a mark
// offset is painted as its own text object, so the fixtures below have one
// entry per shaped glyph.
func textObjectOrigins(t *testing.T, pdf []byte) [][2]int {
	t.Helper()
	var out [][2]int
	for _, stream := range pdfdebug.ContentStreams(pdf) {
		fields := strings.Fields(string(stream))
		for i := 0; i+6 < len(fields); i++ {
			if fields[i] != "1" || fields[i+1] != "0" || fields[i+2] != "0" || fields[i+3] != "1" || fields[i+6] != "Tm" {
				continue
			}
			out = append(out, [2]int{pdfMillipoints(t, fields[i+4]), pdfMillipoints(t, fields[i+5])})
		}
	}
	return out
}

// pdfMillipoints parses one number the PDF writer printed for a text matrix.
func pdfMillipoints(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parsing the PDF number %q: %v", s, err)
	}
	return int(math.Round(v * 1000))
}

// fixtureFont loads the bundled face the position assertions measure against.
// Pixels and glyph ids must not depend on the developer machine, so the test
// uses the bundled font even when the suite runs with another fallback face.
func fixtureFont(t *testing.T) *font.Font {
	t.Helper()
	data, err := os.ReadFile(bundledFont(t))
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestCombiningMarkAttachesToItsOwnBase guards the cluster state machine in the
// painted PDF: every mark's text object starts inside the advance box of the
// base immediately before it, and a mark never jumps back to an earlier base.
func TestCombiningMarkAttachesToItsOwnBase(t *testing.T) {
	fontPath := bundledFont(t)
	f := fixtureFont(t)
	const size = 12000 // the fixture's 10pt in millipoints
	cases := []struct {
		name string
		text string
		// pairs maps each mark's glyph index to the glyph index of the base it
		// must be painted over.
		pairs [][2]int
	}{
		{"second acute in о́о́", "о\u0301о\u0301", [][2]int{{0, 1}, {2, 3}}},
		{"each acute in а́б́", "а\u0301б\u0301", [][2]int{{0, 1}, {2, 3}}},
		{"two marks on one base", "о\u0301\u0302", [][2]int{{0, 1}, {0, 2}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(`<p style="margin:0;font-size:10pt">` + tc.text + `</p>`)
			res, err := Convert(context.Background(), src, Options{File: "mark.html", FallbackFont: fontPath})
			if err != nil {
				t.Fatalf("convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
			}
			origins := textObjectOrigins(t, res.PDF)
			runes := []rune(text.Normalize(tc.text))
			if len(origins) != len(runes) {
				t.Fatalf("painted %d text objects, want one per glyph (%d)", len(origins), len(runes))
			}
			for i, pair := range tc.pairs {
				base, mark := pair[0], pair[1]
				advance := int(text.Scale(f.Advance(glyphIndex(t, f, runes[base])), size, f.UnitsPerEm()))
				if origins[mark][0] < origins[base][0] || origins[mark][0] >= origins[base][0]+advance {
					t.Fatalf("mark %d at x=%d is outside base %q's cell [%d,%d)", i, origins[mark][0], string(runes[base]), origins[base][0], origins[base][0]+advance)
				}
				if origins[mark][1] < origins[base][1] {
					t.Fatalf("mark %d paints at y=%d, below base y=%d", i, origins[mark][1], origins[base][1])
				}
			}
			// The two acutes of о́о́ attach to the same glyph and must take
			// the same offset; before the fix the second one reused the anchor
			// of the first cluster's mark and landed a whole base early.
			if tc.text == "о\u0301о\u0301" {
				if got, want := origins[3][0]-origins[2][0], origins[1][0]-origins[0][0]; got != want {
					t.Fatalf("the second acute is %d millipoints from its base, the first is %d", got, want)
				}
			}
			extracted, err := pdfdebug.Text(res.PDF)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(extracted, tc.text) {
				t.Fatalf("the extracted text %q lost the combining sequence %q", extracted, tc.text)
			}
			validateWithPDFCPU(t, res.PDF)
		})
	}
}

// glyphIndex returns the glyph of a rune, so a test can measure the base's
// advance box from the same font the conversion embeds.
func glyphIndex(t *testing.T, f *font.Font, r rune) uint16 {
	t.Helper()
	gid, ok := f.GlyphIndex(r)
	if !ok {
		t.Fatalf("the fixture font has no glyph for %q", r)
	}
	return gid
}

// TestLeadingCombiningMarkIsDiagnosedNotPainted guards the run-start case end to
// end: the mark has no base, so the conversion reports text/glyph at the
// element and paints no glyph instead of drawing the mark at the pen.
func TestLeadingCombiningMarkIsDiagnosedNotPainted(t *testing.T) {
	res, err := Convert(context.Background(), []byte(`<p style="margin:0">`+"\u0301о"+`</p>`), Options{File: "leading-mark.html", FallbackFont: bundledFont(t)})
	if err == nil {
		t.Fatalf("a leading mark converted without an error:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code != "text/glyph" || !strings.Contains(d.Message, "U+0301") {
			continue
		}
		if d.Pos.Line == 0 || d.Element == "" {
			t.Fatalf("the diagnostic has no element position: %+v", d)
		}
		found = true
	}
	if !found {
		t.Fatalf("the leading mark was not reported:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if origins := textObjectOrigins(t, res.PDF); len(origins) != 0 {
		t.Fatalf("the failed run painted %d text objects", len(origins))
	}
}

// TestCraftedMarkAnchorIsDiagnosed guards the resolved-position check end to
// end: a font whose MarkBase anchor paints the mark away from its base reports
// text/glyph at the element and paints no misplaced glyph.
func TestCraftedMarkAnchorIsDiagnosed(t *testing.T) {
	data, err := os.ReadFile(bundledFont(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bad-anchor.ttf")
	if err := os.WriteFile(path, testfont.WithMarkBaseAnchorsX(t, data, 30000), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), []byte(`<p style="margin:0">`+"о\u0301"+`</p>`), Options{File: "bad-anchor.html", FallbackFont: path})
	if err == nil {
		t.Fatalf("the crafted anchor converted without an error:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code != "text/glyph" || !strings.Contains(d.Message, "U+0301") || !strings.Contains(d.Message, "U+043E") {
			continue
		}
		if d.Pos.Line == 0 || d.Element == "" {
			t.Fatalf("the diagnostic has no element position: %+v", d)
		}
		found = true
	}
	if !found {
		t.Fatalf("the invalid anchor was not reported:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if origins := textObjectOrigins(t, res.PDF); len(origins) != 0 {
		t.Fatalf("the failed run painted %d text objects", len(origins))
	}
}
