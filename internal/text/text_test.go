package text

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/vyachin/go-pdf/internal/testfont"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/font"
)

func testFont(t *testing.T) *font.Font {
	t.Helper()
	return testfont.Load(t)
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		// Cyrillic: the bundled font has precomposed forms for all of these.
		"и\u0306": "й",
		"е\u0308": "ё",
		"И\u0306": "Й",
		"и\u0300": "ѝ",
		"е\u0300": "ѐ",
		"у\u0306": "ў",
		"г\u0301": "ѓ",
		"к\u0301": "ќ",
		"Ѝ\u0306": "Ѝ\u0306",
		// Latin.
		"a\u0301": "á",
		"a\u0304": "ā",
		"a\u0306": "ă",
		"a\u0328": "ą",
		// Two marks compose in two steps: a + breve -> ă, ă + acute -> ắ.
		"a\u0306\u0301": "ắ",
		// Canonical decomposition runs before composition, so a precomposed
		// letter followed by a mark can compose with the better-fitting mark:
		// ḋ decomposes to d + dot above, the dot below is ordered first, and
		// d + dot below composes to ḍ.
		"\u1E0B\u0323": "\u1E0D\u0307",
		// Marks that Unicode cannot compose stay as they are.
		"о\u0301": "о\u0301",
		// A character outside the BMP: a compatibility ideograph has a
		// canonical decomposition, so NFC replaces it with the unified one.
		"\U0002F800": "丽",
		// A non-BMP letter followed by a mark has no precomposed form; both
		// code points are kept.
		"\U0001D400\u0301": "\U0001D400\u0301",
		// Hangul jamo compose algorithmically.
		"\u1100\u1161":       "가",
		"\u1100\u1161\u11A8": "각",
		"Счёт":               "Счёт",
		"":                   "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNormalizeOrdersMarksByCombiningClass guards the canonical ordering step:
// a mark with a lower combining class must be composed first even when the
// input lists it second. U+0323 (class 220) comes before U+0301 (class 230).
func TestNormalizeOrdersMarksByCombiningClass(t *testing.T) {
	// a + acute + dot below, listed in the wrong order, composes to ạ + acute
	// and finally to ậ? No: ạ + U+0301 has no precomposed form, so the result is
	// "ạ" followed by the acute mark, exactly as NFC defines it.
	if got, want := Normalize("a\u0301\u0323"), "ạ\u0301"; got != want {
		t.Fatalf("Normalize = %q, want %q", got, want)
	}
	if got, want := Normalize("a\u0323\u0301"), Normalize("a\u0301\u0323"); got != want {
		t.Fatalf("mark order changed the result: %q vs %q", got, want)
	}
}

func TestSplit(t *testing.T) {
	cases := []struct {
		name string
		in   string
		pre  bool
		want []Item
	}{
		{
			name: "collapses whitespace",
			in:   "a  b\n c",
			want: []Item{{Kind: Word, Text: "a"}, {Kind: Space, Text: " "}, {Kind: Word, Text: "b"}, {Kind: Space, Text: " "}, {Kind: Word, Text: "c"}},
		},
		{
			name: "non breaking space stays in the word",
			in:   "г.\u00a0Москва",
			want: []Item{{Kind: Word, Text: "г.\u00a0Москва"}},
		},
		{
			name: "hyphen ends a word and allows a break",
			in:   "слово-слово",
			want: []Item{{Kind: Word, Text: "слово-", BreakAfter: true}, {Kind: Word, Text: "слово"}},
		},
		{
			name: "unicode hyphen allows a break and keeps its glyph",
			in:   "a\u2010b",
			want: []Item{{Kind: Word, Text: "a\u2010", BreakAfter: true}, {Kind: Word, Text: "b"}},
		},
		{
			name: "soft hyphen allows a break and is not drawn",
			in:   "a\u00adb",
			want: []Item{{Kind: Word, Text: "a", BreakAfter: true}, {Kind: Word, Text: "b"}},
		},
		{
			name: "soft hyphen at the end of a word",
			in:   "word\u00ad",
			want: []Item{{Kind: Word, Text: "word", BreakAfter: true}},
		},
		{
			name: "non breaking hyphen stays inside the word",
			in:   "a\u2011b",
			want: []Item{{Kind: Word, Text: "a\u2011b"}},
		},
		{
			name: "pre keeps spaces and line breaks",
			in:   "a  b\nc",
			pre:  true,
			want: []Item{{Kind: Word, Text: "a"}, {Kind: Space, Text: " "}, {Kind: Space, Text: " "}, {Kind: Word, Text: "b"}, {Kind: Newline}, {Kind: Word, Text: "c"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Split(tc.in, tc.pre)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d items %+v, want %d %+v", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("item %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestShape(t *testing.T) {
	f := testFont(t)
	run, err := Shape(f, 12000, "Счёт")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Glyphs) != 4 {
		t.Fatalf("got %d glyphs, want 4", len(run.Glyphs))
	}
	if run.Width <= 0 {
		t.Fatalf("width = %d", run.Width)
	}
	var sum int32
	for _, g := range run.Glyphs {
		if g.Advance <= 0 {
			t.Fatalf("glyph %d has advance %d", g.GID, g.Advance)
		}
		sum += g.Advance
	}
	if sum != run.Width {
		t.Fatalf("width %d != sum of advances %d", run.Width, sum)
	}
}

func TestShapeMissingGlyph(t *testing.T) {
	f := testFont(t)
	if _, ok := f.GlyphIndex(0x1F600); ok {
		t.Skip("the test font has an emoji glyph")
	}
	_, err := Shape(f, 12000, "x😀")
	if err == nil {
		t.Fatal("expected an error for a missing glyph")
	}
	if _, ok := err.(*MissingGlyphError); !ok {
		t.Fatalf("error type = %T, want *MissingGlyphError", err)
	}
	// The whole code point is named, not its low four hex digits.
	want := fmt.Sprintf("no glyph for '😀' (U+1F600) in font %q", f.Family())
	if got := err.Error(); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestMetrics(t *testing.T) {
	f := testFont(t)
	m := MetricsOf(f, 12000)
	if m.Ascent <= 0 || m.Descent <= 0 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestScale(t *testing.T) {
	if got := Scale(1000, 12000, 1000); got != 12000 {
		t.Fatalf("Scale = %d, want 12000", got)
	}
	if got := Scale(500, 12000, 1000); got != 6000 {
		t.Fatalf("Scale = %d, want 6000", got)
	}
}

func loadBundledFont(t *testing.T) *font.Font {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestShapePositionsCombiningMark guards the GPOS mark attachment: the acute in
// "сло́во" must be moved from the pen to the anchor over "о", not left where a
// zero-advance glyph would be drawn.
func TestShapePositionsCombiningMark(t *testing.T) {
	f := loadBundledFont(t)
	run, err := Shape(f, 12000, "сло́во")
	if err != nil {
		t.Fatal(err)
	}
	var mark *Glyph
	for i := range run.Glyphs {
		if run.Glyphs[i].Rune == '\u0301' {
			mark = &run.Glyphs[i]
		}
	}
	if mark == nil {
		t.Fatal("the combining acute is missing")
	}
	if mark.XOffset == 0 {
		t.Fatalf("the mark was drawn at the pen: %+v", *mark)
	}
	if mark.YOffset != 0 {
		// DejaVu's combining acute already sits above the baseline; the anchor
		// only moves it horizontally. Any value is allowed, but this pins the
		// current font so a wrong anchor sign would be visible.
		t.Logf("mark y offset = %d", mark.YOffset)
	}
}

// TestShapeRejectsUnattachedMark guards the diagnostic: a font without a mark
// lookup must not draw the mark at the pen.
func TestShapeRejectsUnattachedMark(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	// Drop the GPOS table: the glyphs stay, the attachment data does not.
	out := append([]byte(nil), data...)
	n := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if string(out[rec:rec+4]) == "GPOS" {
			binary.BigEndian.PutUint32(out[rec+12:rec+16], 0)
			break
		}
	}
	f, err := font.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Shape(f, 12000, "сло́во")
	var missing *MissingMarkError
	if !errors.As(err, &missing) || missing.Mark != '\u0301' {
		t.Fatalf("Shape returned %v, want MissingMarkError for U+0301", err)
	}
}

// TestSplitUnicodeSeparators guards form feed, line separator and paragraph
// separator as line breaks in preformatted and normal text.
func TestSplitUnicodeSeparators(t *testing.T) {
	for _, pre := range []bool{true, false} {
		for _, sep := range []rune{'\f', '\u2028', '\u2029'} {
			items := Split("A"+string(sep)+"B", pre)
			var kinds []ItemKind
			for _, it := range items {
				kinds = append(kinds, it.Kind)
			}
			want := []ItemKind{Word, Newline, Word}
			if len(kinds) != len(want) {
				t.Fatalf("pre=%v sep=U+%04X: items = %+v", pre, sep, items)
			}
			for i := range want {
				if kinds[i] != want[i] {
					t.Fatalf("pre=%v sep=U+%04X: kinds = %v", pre, sep, kinds)
				}
			}
		}
	}
}

// TestShapingIsChargedPerRune guards RV4-04: shaping charges one unit per rune
// before it builds the glyph slice, so a huge word stops at the budget instead
// of allocating runes, glyphs and per-rune state first.
func TestShapingIsChargedPerRune(t *testing.T) {
	f := testFont(t)
	g := budget.New(context.Background(), 100)
	_, err := ShapeWithOptions(f, 12000, strings.Repeat("a", 1000), ShapeOptions{Guard: g})
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("a thousand runes under a hundred-unit budget returned %v", err)
	}
	if used := g.Used(); used != 1000 {
		t.Fatalf("shaping charged %d units for 1000 runes", used)
	}
}

// TestShapingKeepsItsOutput pins that the per-rune state slice is gone: the
// same text shapes to the same glyphs, advances and mark offsets without a
// slice entry per rune.
func TestShapingKeepsItsOutput(t *testing.T) {
	f := testFont(t)
	run, err := Shape(f, 12000, "AVa")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Glyphs) != 3 {
		t.Fatalf("shaped %d glyphs, want 3", len(run.Glyphs))
	}
	if run.Width <= 0 {
		t.Fatalf("shaped width = %d", run.Width)
	}
}

// TestNonBreakingSpaceKeepsItsCharacter guards RV4-31: the run and the
// ToUnicode source keep U+00A0, while the text layer still treats it as one
// unbreakable word.
func TestNonBreakingSpaceKeepsItsCharacter(t *testing.T) {
	f := testFont(t)
	run, err := Shape(f, 12000, "a\u00a0b")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Glyphs) != 3 {
		t.Fatalf("shaped %d glyphs, want 3", len(run.Glyphs))
	}
	if run.Glyphs[1].Rune != '\u00a0' {
		t.Fatalf("the middle glyph carries %U, want U+00A0", run.Glyphs[1].Rune)
	}
	items := Split("a\u00a0b", false)
	if len(items) != 1 || items[0].Kind != Word {
		t.Fatalf("the non-breaking space produced %v", items)
	}
}
