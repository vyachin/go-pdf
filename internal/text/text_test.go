package text

import (
	"os"
	"runtime"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
)

func testFont(t *testing.T) *font.Font {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		)
	}
	candidates = append(candidates,
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
	)
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := font.Parse(data)
		if err != nil {
			continue
		}
		return f
	}
	t.Skip("no TrueType font available; set PDFTEST_FONT")
	return nil
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
		// Marks that Unicode cannot compose stay as they are.
		"о\u0301": "о\u0301",
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
			want: []Item{{Kind: Word, Text: "г. Москва"}},
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
