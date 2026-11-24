package font

import (
	"encoding/binary"
	"errors"
	"runtime"
	"testing"
)

// The helpers below build a TrueType font whose glyphs are nested composites:
// glyph 1 refers to `components` copies of glyph 2, which refers to as many
// copies of glyph 3, and so on. The last glyph of the chain is a triangle, so
// every level really multiplies the number of contours. Such a font is a few
// hundred bytes but expands to an exponential outline.

type fontBuilder struct {
	tables []struct {
		tag  string
		data []byte
	}
}

func (b *fontBuilder) add(tag string, data []byte) {
	b.tables = append(b.tables, struct {
		tag  string
		data []byte
	}{tag, data})
}

func (b *fontBuilder) bytes() []byte {
	n := len(b.tables)
	out := make([]byte, 12+n*16)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(n))
	off := 12 + n*16
	for i, t := range b.tables {
		rec := 12 + i*16
		copy(out[rec:rec+4], t.tag)
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(off))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(t.data)))
		out = append(out, t.data...)
		off += len(t.data)
		for len(out)%4 != 0 {
			out = append(out, 0)
			off++
		}
	}
	return out
}

func testTriangleGlyph() []byte {
	g := make([]byte, 10)
	binary.BigEndian.PutUint16(g[0:2], 1) // one contour
	binary.BigEndian.PutUint16(g[6:8], 100)
	binary.BigEndian.PutUint16(g[8:10], 100)
	g = append(g, 0, 2) // endPts[0] = 2, three points
	g = append(g, 0, 0) // no instructions
	g = append(g, 1, 1, 1)
	for i := 0; i < 3; i++ {
		g = append(g, 0, byte(10*(i+1)))
	}
	for i := 0; i < 3; i++ {
		g = append(g, 0, byte(10*(i+1)))
	}
	return g
}

func testCompositeGlyph(components int, target uint16) []byte {
	g := make([]byte, 10)
	binary.BigEndian.PutUint16(g[0:2], 0xFFFF) // negative number of contours
	binary.BigEndian.PutUint16(g[6:8], 100)
	binary.BigEndian.PutUint16(g[8:10], 100)
	for i := 0; i < components; i++ {
		flags := uint16(0x0002) // ARGS_ARE_XY_VALUES with byte offsets
		if i+1 < components {
			flags |= 0x0020 // MORE_COMPONENTS
		}
		g = append(g, byte(flags>>8), byte(flags), byte(target>>8), byte(target), 0, 0)
	}
	return g
}

// testBombFont builds a font with `components` copies per level and `chain`
// composite levels.
func testBombFont(t *testing.T, components, chain int) []byte {
	t.Helper()
	numGlyphs := chain + 2
	var glyf []byte
	loca := make([]uint32, numGlyphs+1)
	put := func(gid int, data []byte) {
		loca[gid] = uint32(len(glyf))
		glyf = append(glyf, data...)
		for len(glyf)%4 != 0 {
			glyf = append(glyf, 0)
		}
	}
	put(0, nil)
	for i := 1; i <= chain; i++ {
		if i == chain {
			put(i, testTriangleGlyph())
			break
		}
		put(i, testCompositeGlyph(components, uint16(i+1)))
	}
	put(numGlyphs-1, nil)
	loca[numGlyphs] = uint32(len(glyf))

	locaBytes := make([]byte, len(loca)*4)
	for i, v := range loca {
		binary.BigEndian.PutUint32(locaBytes[i*4:i*4+4], v)
	}
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm
	binary.BigEndian.PutUint16(head[50:52], 1)    // long loca offsets
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:6], 800) // ascent
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], uint16(numGlyphs))
	hmtx := []byte{0, 100, 0, 0}

	b := &fontBuilder{}
	b.add("head", head)
	b.add("hhea", hhea)
	b.add("maxp", maxp)
	b.add("hmtx", hmtx)
	b.add("loca", locaBytes)
	b.add("glyf", glyf)
	return b.bytes()
}

// TestCompositeGlyphExpansionIsBounded guards RV2-04: a font under one kilobyte
// could expand to hundreds of megabytes because every composite component was
// expanded again. The path must be rejected before the allocation grows.
func TestCompositeGlyphExpansionIsBounded(t *testing.T) {
	data := testBombFont(t, 16, 6)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(data) > 2048 {
		t.Fatalf("the test font grew to %d bytes; it is supposed to stay tiny", len(data))
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	contours, err := f.GlyphPath(1)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrGlyphTooComplex) {
		t.Fatalf("GlyphPath returned %d contours and err=%v, want ErrGlyphTooComplex", len(contours), err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 64<<20 {
		t.Fatalf("expanding a %d-byte font allocated %d bytes before reporting the limit", len(data), allocated)
	}
}

// TestOrdinaryCompositeGlyphStillExpands makes sure the limit does not reject a
// real font: DejaVu Sans has composite glyphs such as Ё.
func TestOrdinaryCompositeGlyphStillExpands(t *testing.T) {
	f := testFont(t)
	gid, ok := f.GlyphIndex('Ё')
	if !ok {
		t.Skip("test font has no Ё")
	}
	contours, err := f.GlyphPath(gid)
	if err != nil {
		t.Fatalf("GlyphPath: %v", err)
	}
	if len(contours) == 0 {
		t.Fatal("Ё has no outline")
	}
}
