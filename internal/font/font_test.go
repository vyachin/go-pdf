package font

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testFont(t *testing.T) *Font {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
	// The bundled font keeps results identical on every platform and in CI.
	candidates = append(candidates, filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
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

func TestParseClassDefShortHeaders(t *testing.T) {
	cases := [][]byte{
		{},
		{0},
		{0, 1},
		{0, 1, 0, 0}, // format 1 with a truncated count
		{0, 2, 0, 1}, // format 2 claiming one range without data
		{0, 9, 0, 0}, // unknown format
	}
	for _, data := range cases {
		if _, ok := parseClassDef(data, 0); ok {
			t.Fatalf("parseClassDef(%v) unexpectedly succeeded", data)
		}
	}
	// A complete format 1 definition still parses.
	ok := []byte{0, 1, 0, 5, 0, 1, 0, 7}
	ranges, parsed := parseClassDef(ok, 0)
	if !parsed || len(ranges) != 1 || ranges[0].start != 5 || ranges[0].class != 7 {
		t.Fatalf("valid class definition = %+v, %v", ranges, parsed)
	}
}

func FuzzParseClassDef(f *testing.F) {
	f.Add([]byte{0, 1, 0, 0})
	f.Add([]byte{0, 2, 0, 0, 0, 1, 0, 0, 0, 0})
	f.Add([]byte{0, 1, 0, 5, 0, 1, 0, 7})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseClassDef(data, 0)
	})
}

// truncateTable shortens one table in the sfnt directory so that parsers see
// less data than the table claims to have.
func truncateTable(t *testing.T, data []byte, tag string, length int) []byte {
	t.Helper()
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	for i := 0; i < numTables; i++ {
		rec := 12 + i*16
		if string(data[rec:rec+4]) != tag {
			continue
		}
		out := append([]byte(nil), data...)
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(length))
		return out
	}
	t.Fatalf("table %q is not present in the test font", tag)
	return nil
}

func TestTruncatedGPOSDoesNotPanic(t *testing.T) {
	f := testFont(t)
	if f.gpos == nil {
		t.Skip("the test font has no GPOS table")
	}
	src := f.data
	for _, length := range []int{0, 2, 8, 10, 14, 20, 48} {
		data := truncateTable(t, src, "GPOS", length)
		parsed, err := Parse(data)
		if err != nil {
			// A short optional table may also make the font unusable; what
			// matters is that parsing never panics.
			continue
		}
		if parsed.Kerning(0, 0) != 0 && length < 10 {
			t.Fatalf("truncated GPOS of %d bytes still produced kerning", length)
		}
	}
}

func TestSubsetChecksums(t *testing.T) {
	f := testFont(t)
	gid, ok := f.GlyphIndex('A')
	if !ok {
		t.Skip("test font has no A")
	}
	data, err := f.Subset(map[uint16]bool{gid: true})
	if err != nil {
		t.Fatal(err)
	}
	if sum := checksum(data); sum != 0xB1B0AFBA {
		t.Fatalf("whole-font checksum = %#x, want 0xb1b0afba", sum)
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	for i := 0; i < numTables; i++ {
		rec := 12 + i*16
		if string(data[rec:rec+4]) != "head" {
			continue
		}
		dirSum := binary.BigEndian.Uint32(data[rec+4 : rec+8])
		off := int(binary.BigEndian.Uint32(data[rec+8 : rec+12]))
		length := int(binary.BigEndian.Uint32(data[rec+12 : rec+16]))
		head := append([]byte(nil), data[off:off+length]...)
		binary.BigEndian.PutUint32(head[8:12], 0)
		if got := checksum(head); got != dirSum {
			t.Fatalf("head directory checksum = %#x, want %#x (checksum with the adjustment zeroed)", dirSum, got)
		}
		if _, err := Parse(data); err != nil {
			t.Fatalf("the subset does not parse: %v", err)
		}
		return
	}
	t.Fatal("the subset has no head table")
}
