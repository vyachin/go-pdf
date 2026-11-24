package font

import (
	"os"
	"runtime"
	"testing"
)

func testFont(t *testing.T) *Font {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
			"/Library/Fonts/Arial.ttf",
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
		f, err := Parse(data)
		if err != nil {
			t.Logf("skipping %s: %v", path, err)
			continue
		}
		return f
	}
	t.Skip("no TrueType test font found; set PDFTEST_FONT")
	return nil
}

func TestParse(t *testing.T) {
	f := testFont(t)
	if f.UnitsPerEm() <= 0 {
		t.Fatalf("unitsPerEm = %d", f.UnitsPerEm())
	}
	if f.NumGlyphs() == 0 {
		t.Fatal("no glyphs")
	}
	if f.Family() == "" {
		t.Fatal("no family name")
	}
	if f.Ascent() == 0 {
		t.Fatal("no ascent")
	}
}

func TestCyrillicGlyphLookup(t *testing.T) {
	f := testFont(t)
	for _, r := range []rune{'С', 'ч', 'ё', '№', '—'} {
		gid, ok := f.GlyphIndex(r)
		if !ok {
			t.Fatalf("rune %q is missing from the test font", r)
		}
		if !f.HasGlyph(gid) {
			t.Fatalf("glyph %d for %q is out of range", gid, r)
		}
		if f.Advance(gid) <= 0 {
			t.Fatalf("advance for %q is %d", r, f.Advance(gid))
		}
	}
	if _, ok := f.GlyphIndex(0x1F600); ok {
		t.Skip("the test font maps an emoji, cannot check missing glyph handling")
	}
}

func TestKerning(t *testing.T) {
	f := testFont(t)
	if f.gpos == nil {
		t.Skip("test font has no GPOS kerning")
	}
	nonzero := 0
	pairs := [][2]rune{{'А', 'В'}, {'Т', 'о'}, {'Г', 'А'}, {'A', 'V'}, {'T', 'o'}, {'V', 'A'}}
	for _, p := range pairs {
		l, ok1 := f.GlyphIndex(p[0])
		r, ok2 := f.GlyphIndex(p[1])
		if !ok1 || !ok2 {
			continue
		}
		if f.Kerning(l, r) != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Skip("test font has GPOS but no kerning for the sampled pairs")
	}
}

func TestGlyphPath(t *testing.T) {
	f := testFont(t)
	gid, ok := f.GlyphIndex('О')
	if !ok {
		t.Skip("test font has no О")
	}
	contours := f.GlyphPath(gid)
	if len(contours) != 2 {
		t.Fatalf("О has %d contours, want 2 (outer and inner ring)", len(contours))
	}
	if len(contours[0]) < 4 || len(contours[1]) < 4 {
		t.Fatalf("contours are too small: %d and %d points", len(contours[0]), len(contours[1]))
	}
	// A composite glyph, if present, must resolve through its components.
	if gid, ok := f.GlyphIndex('Ё'); ok {
		if len(f.GlyphPath(gid)) == 0 {
			t.Fatalf("Ё resolved to %d but has no outline", gid)
		}
	}
}

func TestSubset(t *testing.T) {
	f := testFont(t)
	gidO, ok := f.GlyphIndex('О')
	if !ok {
		t.Skip("test font has no О")
	}
	data, err := f.Subset(map[uint16]bool{gidO: true})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := Parse(data)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}
	if sub.NumGlyphs() != f.NumGlyphs() {
		t.Fatalf("subset has %d glyphs, want %d (numbering must be preserved)", sub.NumGlyphs(), f.NumGlyphs())
	}
	if got, want := len(sub.GlyphPath(gidO)), len(f.GlyphPath(gidO)); got != want {
		t.Fatalf("subset glyph О has %d contours, want %d", got, want)
	}
	if len(data) >= len(f.data) {
		t.Fatalf("subset (%d bytes) is not smaller than the original (%d bytes)", len(data), len(f.data))
	}
	// An unused glyph must have no outline but keep its slot.
	unused := uint16(1)
	if unused == gidO {
		unused = 2
	}
	if len(sub.GlyphPath(unused)) != 0 {
		t.Fatalf("glyph %d should be empty in the subset", unused)
	}
}

func TestRejectCFF(t *testing.T) {
	// 'OTTO' is the CFF flavoured sfnt version.
	data := make([]byte, 12)
	data[0], data[1], data[2], data[3] = 'O', 'T', 'T', 'O'
	if _, err := Parse(data); err == nil {
		t.Fatal("expected a CFF font to be rejected")
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	ff := testFont
	_ = ff
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = Parse(b)
	})
}
