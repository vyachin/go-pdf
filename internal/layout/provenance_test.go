package layout

import (
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/profile"
)

// missingGlyph is a code point no test font provides. It is written as a raw
// character because a CSS escape would decode to a plain "U".
const missingGlyph = "\U0010FFFF"

// findLine returns the first diagnostic with the given code on the given line.
func findLine(t *testing.T, l diag.List, code string, line int) diag.Diagnostic {
	t.Helper()
	for _, d := range l {
		if d.Code == code && d.Pos.Line == line {
			return d
		}
	}
	t.Fatalf("no %s diagnostic on line %d:\n%s", code, line, strings.Join(l.Lines(), "\n"))
	return diag.Diagnostic{}
}

// findCode returns the first diagnostic with the given code.
func findCode(t *testing.T, l diag.List, code string) diag.Diagnostic {
	t.Helper()
	for _, d := range l {
		if d.Code == code {
			return d
		}
	}
	t.Fatalf("no %s diagnostic:\n%s", code, strings.Join(l.Lines(), "\n"))
	return diag.Diagnostic{}
}

// TestMissingGlyphReportsSourcePosition guards the body-text half of the
// provenance contract: a glyph the font cannot render is reported at the text
// node that contains it, on the line the author wrote, with the owning element
// path — after several lines of text and inside a nested span.
func TestMissingGlyphReportsSourcePosition(t *testing.T) {
	h := newHarness(t)
	src := "<p>Первый абзац</p>\n" +
		"<p>Второй абзац</p>\n" +
		"<p>Третий <span>абзац \U0010FFFF</span></p>"
	size, margin := h.ctx.PageSize, h.ctx.Margin
	_, diags := h.runRaw(src, size, margin)
	d := findCode(t, diags, "text/glyph")
	if d.Pos.File != "test.html" || d.Pos.Line != 3 || d.Pos.Col < 1 {
		t.Fatalf("glyph diagnostic = %s:%d:%d, want test.html:3:<col>", d.Pos.File, d.Pos.Line, d.Pos.Col)
	}
	if !strings.Contains(d.Element, "span") {
		t.Fatalf("glyph diagnostic element = %q, want the nested span", d.Element)
	}
}

// failingFontsWithError reports a missing face as a plain error.
type failingFontsWithError struct{}

func (failingFontsWithError) Face(families []string, weight int, italic bool, size int32) (*FontFace, error) {
	return nil, &faceError{}
}

type faceError struct{}

func (*faceError) Error() string { return "font family is not available" }

// TestMissingFaceReportsSourcePosition guards the face half: a family that never
// resolves is reported at the text that needed it, with the element path.
func TestMissingFaceReportsSourcePosition(t *testing.T) {
	h := newHarness(t)
	h.ctx.Fonts = failingFontsWithError{}
	src := `<p>first</p>` + "\n" + `<p><span>second</span></p>`
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	// Every word fails without a face, so the second line is what the test
	// checks: the position must follow the text, not the first failure.
	d := findLine(t, diags, "text/font", 2)
	if d.Pos.File != "test.html" || d.Pos.Col < 1 {
		t.Fatalf("face diagnostic = %s:%d:%d, want test.html:2:<col>", d.Pos.File, d.Pos.Line, d.Pos.Col)
	}
	if !strings.Contains(d.Element, "span") {
		t.Fatalf("face diagnostic element = %q, want the nested span", d.Element)
	}
}

// TestGeneratedContentUsesItsDeclaration guards generated text: an a::after
// glyph failure names the declaration that generated the text.
func TestGeneratedContentUsesItsDeclaration(t *testing.T) {
	h := newHarness(t)
	src := "<style>a::after { content: \"" + missingGlyph + "\" }</style>\n<p><a href=\"#x\">link</a></p>"
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	d := findCode(t, diags, "text/glyph")
	if d.Pos.File != "test.html" || d.Pos.Line != 1 || d.Pos.Col < 1 {
		t.Fatalf("generated diagnostic = %s:%d:%d, want the a::after declaration on line 1", d.Pos.File, d.Pos.Line, d.Pos.Col)
	}
	if !strings.Contains(d.Element, "a") {
		t.Fatalf("generated diagnostic element = %q, want the anchor", d.Element)
	}
}

// TestMarginBoxGlyphUsesItsDeclaration guards the margin-box half: a missing
// glyph in a header names the content declaration and the box.
func TestMarginBoxGlyphUsesItsDeclaration(t *testing.T) {
	h := newHarness(t)
	src := "<style>@page { @bottom-center { content: \"" + missingGlyph + "\" } }</style><p>x</p>"
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	d := findCode(t, diags, "text/glyph")
	if d.Pos.File != "test.html" || d.Pos.Line != 1 || d.Pos.Col < 1 {
		t.Fatalf("margin diagnostic = %s:%d:%d, want the content declaration on line 1", d.Pos.File, d.Pos.Line, d.Pos.Col)
	}
	if d.Element != "bottom-center" {
		t.Fatalf("margin diagnostic element = %q, want bottom-center", d.Element)
	}
}

// TestOverflowWarningNamesItsElement guards the overflow half: the warning that
// a block overflows keeps the element position instead of an empty one.
func TestOverflowWarningNamesItsElement(t *testing.T) {
	h := newHarness(t)
	src := "<p>first</p>\n<div style=\"height: 400mm\">tall</div>"
	_, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	var found bool
	for _, d := range diags {
		if d.Code != "layout/overflow" {
			continue
		}
		found = true
		if d.Pos.File != "test.html" || d.Pos.Line != 2 {
			t.Fatalf("overflow warning = %s:%d, want test.html:2", d.Pos.File, d.Pos.Line)
		}
		if !strings.Contains(d.Element, "div") {
			t.Fatalf("overflow element = %q, want the div", d.Element)
		}
	}
	if !found {
		t.Fatalf("no overflow warning: %v", diags.Lines())
	}
}

// TestLegacyEncodingKeepsOriginalBytes guards the decoded-coordinate half: a
// text failure in a windows-1251 document names the original file position the
// author sees, not a decoded offset.
func TestLegacyEncodingKeepsOriginalBytes(t *testing.T) {
	// Two Cyrillic lines, then a fallback that cannot serve the family.
	legacy := []byte("<p>Привет</p>\n<p>Мир</p>")
	doc, err := html.ParseWithOptions(legacy, html.ParseOptions{File: "legacy.html", Charset: "windows-1251"})
	if err != nil {
		t.Fatal(err)
	}
	if diags := profile.Validate(doc, profile.Options{}); diags.HasErrors() {
		t.Fatalf("validation: %v", diags.Lines())
	}
	sheets, _ := profile.CollectStylesheets(doc, profile.Options{})
	ctx := Context{
		Fonts:    failingFontsWithError{},
		PageSize: css.PageSize{Width: 595276, Height: 841890},
		Margin:   [4]int32{56693, 56693, 56693, 56693},
		Cascade:  css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...),
		PageStyle: ParsePageStyle(sheets, css.PageSize{Width: 595276, Height: 841890},
			[4]int32{56693, 56693, 56693, 56693}),
	}
	_, diags := Run(doc, ctx)
	d := findLine(t, diags, "text/font", 2)
	if d.Pos.File != "legacy.html" || d.Pos.Col < 1 {
		t.Fatalf("legacy diagnostic = %s:%d:%d, want legacy.html:2:<col>", d.Pos.File, d.Pos.Line, d.Pos.Col)
	}
}

// TestProvenanceReportsTheOwningSourceNotACharacterOffset guards the precision
// rule: entity expansion, canonical decomposition and multibyte text move the
// glyph away from a raw byte offset, so the diagnostic names the start of the
// text node that owns it — the same column for ASCII, Cyrillic and a decomposed
// accent — instead of inventing a character position.
func TestProvenanceReportsTheOwningSourceNotACharacterOffset(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		name string
		src  string
		col  int
	}{
		// An entity-expanded text node has no span of its own, so the parser
		// gives it its parent's range: the diagnostic names the element, not an
		// invented character offset.
		{"entity", "<p>&amp;" + missingGlyph + "</p>", 1},
		{"decomposed", "<p>e\u0301" + missingGlyph + "</p>", 4},
		{"cyrillic", "<p>Привет " + missingGlyph + "</p>", 4},
		{"nested-span", "<p><span>А" + missingGlyph + "</span></p>", 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := h.runRaw(tc.src, h.ctx.PageSize, h.ctx.Margin)
			d := findCode(t, diags, "text/glyph")
			if d.Pos.File != "test.html" || d.Pos.Line != 1 || d.Pos.Col != tc.col {
				t.Fatalf("%s: diagnostic = %s:%d:%d, want test.html:1:%d",
					tc.name, d.Pos.File, d.Pos.Line, d.Pos.Col, tc.col)
			}
		})
	}
}
