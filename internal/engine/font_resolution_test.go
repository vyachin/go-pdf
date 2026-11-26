package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
)

// newFallbackRegistry builds a registry with no declared faces, so a request
// resolves only through the configured fallback.
func newFallbackRegistry(t *testing.T, opts Options) *fontRegistry {
	t.Helper()
	ctx := context.Background()
	guard := budget.New(ctx, 0)
	reg := assets.New(guard)
	t.Cleanup(func() { _ = reg.Close() })
	r, err := newFontRegistry(ctx, guard, nil, opts, reg)
	if err != nil {
		t.Fatalf("building the font registry: %v", err)
	}
	return r
}

// writeFont copies one source font to dir/name and returns the new path.
func writeFont(t *testing.T, dir, name, source string) string {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// bundledBoldFont is the second bundled face, used for a fallback family's
// bold and italic siblings.
func bundledBoldFont() string {
	return filepath.Join(repoRoot(), "testdata", "fonts", "DejaVuSans-Bold.ttf")
}

// TestMissingFontFailsWithoutReadingHostFont guards RV5-53: a document that
// declares no font and passes no fallback fails with one font/unavailable
// diagnostic, and the conversion never opens an installed host font.
//
// The asset registry counts every successful read, including the reads the
// removed system search used, so a zero font-read count proves no host font
// was selected even on a machine where the conventional paths exist.
func TestMissingFontFailsWithoutReadingHostFont(t *testing.T) {
	src := []byte(`<p>no font is declared here</p>`)
	opts := Options{File: "no-font.html"}

	res, err := Convert(context.Background(), src, opts)
	if err == nil {
		t.Fatal("a document without a font converted")
	}
	if len(res.PDF) != 0 {
		t.Fatalf("a failed conversion produced %d PDF bytes", len(res.PDF))
	}
	var unavailable []diag.Diagnostic
	for _, d := range res.Diagnostics {
		if d.Code == "font/unavailable" {
			unavailable = append(unavailable, d)
		}
	}
	if len(unavailable) != 1 {
		t.Fatalf("%d font/unavailable diagnostics, want one:\n%s", len(unavailable), strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	message := unavailable[0].Message
	for _, want := range []string{"serif", "Options.FallbackFont", "Options.FallbackFonts"} {
		if !strings.Contains(message, want) {
			t.Fatalf("the diagnostic does not name %q: %s", want, message)
		}
	}

	// Count the font reads of the same conversion through its own registry.
	ctx := context.Background()
	guard := budget.New(ctx, 0)
	reg := assets.New(guard)
	defer func() { _ = reg.Close() }()
	p, diags, err := prepareDocument(ctx, src, Options{File: "no-font.html"}, guard, reg)
	if err != nil || p == nil {
		t.Fatalf("prepare: %v\n%s", err, strings.Join(diags.Lines(), "\n"))
	}
	var layoutDiags diag.List
	if _, err := layoutDocument(ctx, p, Options{File: "no-font.html"}, &layoutDiags); err != nil {
		t.Fatalf("layout: %v", err)
	}
	if got := reg.Reads(assets.Font); got != 0 {
		t.Fatalf("the conversion read %d font files without a configured font", got)
	}
}

// TestFallbackFontsDiscoverFamilyAndSiblings guards the explicit list: the
// first entry that parses becomes the family, a missing first entry falls
// through, and the conventional bold and italic files next to the selected
// regular file are discovered. FallbackFont keeps its single-face meaning.
func TestFallbackFontsDiscoverFamilyAndSiblings(t *testing.T) {
	dir := t.TempDir()
	regular := writeFont(t, dir, "Family.ttf", testFontPath(t))
	writeFont(t, dir, "Family Bold.ttf", bundledBoldFont())
	writeFont(t, dir, "Family Italic.ttf", bundledBoldFont())

	list := newFallbackRegistry(t, Options{FallbackFonts: []string{
		filepath.Join(dir, "missing.ttf"),
		regular,
	}})
	for _, tc := range []struct {
		name      string
		weight    int
		italic    bool
		subfamily string
	}{
		{"regular", 400, false, "Book"},
		{"bold sibling", 700, false, "Bold"},
		{"italic sibling", 400, true, "Bold"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			face, err := list.Face([]string{"NotDeclared"}, tc.weight, tc.italic, 12000)
			if err != nil {
				t.Fatalf("Face: %v", err)
			}
			if got := face.Font.Subfamily(); got != tc.subfamily {
				t.Fatalf("resolved to subfamily %q, want %q", got, tc.subfamily)
			}
		})
	}

	// The bold and italic entries are distinct parses of distinct files, so a
	// wrong selection cannot pass by comparing font names alone.
	regularFace, err := list.Face([]string{"NotDeclared"}, 400, false, 12000)
	if err != nil {
		t.Fatal(err)
	}
	boldFace, err := list.Face([]string{"NotDeclared"}, 700, false, 12000)
	if err != nil {
		t.Fatal(err)
	}
	italicFace, err := list.Face([]string{"NotDeclared"}, 400, true, 12000)
	if err != nil {
		t.Fatal(err)
	}
	if boldFace.Font == regularFace.Font || italicFace.Font == regularFace.Font || italicFace.Font == boldFace.Font {
		t.Fatal("the regular, bold and italic siblings must be three separate faces")
	}

	// FallbackFont is tried first and keeps its documented single-face meaning.
	single := newFallbackRegistry(t, Options{FallbackFont: regular})
	single700, err := single.Face([]string{"NotDeclared"}, 700, false, 12000)
	if err != nil {
		t.Fatal(err)
	}
	if got := single700.Font.Subfamily(); got != "Book" {
		t.Fatalf("FallbackFont resolved to %q for weight 700, want its single regular face", got)
	}
}

// TestFallbackFontTakesPrecedence guards the documented order: when both
// options are set, FallbackFont is selected before the list.
func TestFallbackFontTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	regular := writeFont(t, dir, "Family.ttf", testFontPath(t))
	listFont := writeFont(t, dir, "Other.ttf", bundledBoldFont())
	r := newFallbackRegistry(t, Options{
		FallbackFont:  regular,
		FallbackFonts: []string{listFont},
	})
	face, err := r.Face([]string{"NotDeclared"}, 700, false, 12000)
	if err != nil {
		t.Fatal(err)
	}
	if got := face.Font.Subfamily(); got != "Book" {
		t.Fatalf("resolved to %q, want the FallbackFont face", got)
	}
}

// TestNoFallbackCandidateReportsUnavailable guards the other end of the list:
// when every configured FallbackFonts entry is missing, the conversion still
// reports font/unavailable and names both options.
func TestNoFallbackCandidateReportsUnavailable(t *testing.T) {
	dir := t.TempDir()
	r := newFallbackRegistry(t, Options{FallbackFonts: []string{
		filepath.Join(dir, "missing-one.ttf"),
		filepath.Join(dir, "missing-two.ttf"),
	}})
	_, err := r.Face([]string{"Nope"}, 400, false, 12000)
	if err == nil {
		t.Fatal("Face accepted an unavailable family")
	}
	for _, want := range []string{"Nope", "Options.FallbackFont", "Options.FallbackFonts"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the diagnostic does not name %q: %v", want, err)
		}
	}
}

// TestDeclaredFamilyKeepsPriority guards the first step of the resolution
// order: a family the document declares resolves from @font-face even when a
// fallback is configured, and the fallback serves only the families the
// document does not declare.
func TestDeclaredFamilyKeepsPriority(t *testing.T) {
	dir := t.TempDir()
	writeFont(t, dir, "Declared.ttf", testFontPath(t))
	fallback := writeFont(t, dir, "Fallback.ttf", bundledBoldFont())
	sheet := css.ParseSheet(css.SheetOptions{File: "faces.css"}, []byte(`
		@font-face { font-family: "Declared"; src: url(Declared.ttf) }
	`))
	ctx := context.Background()
	guard := budget.New(ctx, 0)
	reg := assets.New(guard)
	defer func() { _ = reg.Close() }()
	r, err := newFontRegistry(ctx, guard, []*css.Stylesheet{sheet}, Options{
		FontDir:       dir,
		FallbackFonts: []string{fallback},
	}, reg)
	if err != nil {
		t.Fatal(err)
	}
	declared, err := r.Face([]string{"Declared"}, 400, false, 12000)
	if err != nil {
		t.Fatalf("the declared family did not resolve: %v", err)
	}
	if got := declared.Font.Subfamily(); got != "Book" {
		t.Fatalf("the declared family resolved to %q, want the declared Book face", got)
	}
	missing, err := r.Face([]string{"Missing"}, 400, false, 12000)
	if err != nil {
		t.Fatalf("the fallback did not cover an undeclared family: %v", err)
	}
	if got := missing.Font.Subfamily(); got != "Bold" {
		t.Fatalf("the undeclared family resolved to %q, want the fallback face", got)
	}
}
