package font

import (
	"encoding/binary"
	"math"
	"testing"
)

// testCompositeFont builds a two-glyph font: glyph 1 is a triangle whose first
// point is (10,10), glyph 2 is a component of glyph 1 at offset (100,0) with
// scale 0.5. scaled selects SCALED_COMPONENT_OFFSET.
func testCompositeFont(scaled bool) []byte {
	flags := uint16(0x0002 | 0x0008) // ARGS_ARE_XY_VALUES, WE_HAVE_A_SCALE
	if scaled {
		flags |= 0x0800
	}
	return testCompositeFontWithFlags(flags)
}

// testCompositeFontWithFlags is testCompositeFont with the component flags
// chosen by the caller, so a test can build a point-matching component.
func testCompositeFontWithFlags(flags uint16) []byte {
	triangle := make([]byte, 10)
	binary.BigEndian.PutUint16(triangle[0:2], 1)  // one contour
	binary.BigEndian.PutUint16(triangle[2:4], 10) // xMin
	binary.BigEndian.PutUint16(triangle[4:6], 10) // yMin
	binary.BigEndian.PutUint16(triangle[6:8], 30) // xMax
	binary.BigEndian.PutUint16(triangle[8:10], 30)
	triangle = append(triangle, 0, 2)             // endPts[0] = 2
	triangle = append(triangle, 0, 0)             // no instructions
	triangle = append(triangle, 0x37, 0x37, 0x37) // on-curve, short positive x and y
	triangle = append(triangle, 10, 10, 10)
	triangle = append(triangle, 10, 10, 10)

	composite := make([]byte, 10)
	binary.BigEndian.PutUint16(composite[0:2], 0xFFFF) // composite
	binary.BigEndian.PutUint16(composite[6:8], 100)
	composite = append(composite, byte(flags>>8), byte(flags), 0, 1) // component 1
	composite = append(composite, 100, 0)                            // int8 offset (100,0)
	composite = append(composite, 0x20, 0x00)                        // F2Dot14 0.5

	numGlyphs := 3
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
	put(1, triangle)
	put(2, composite)
	loca[numGlyphs] = uint32(len(glyf))

	locaBytes := make([]byte, len(loca)*4)
	for i, v := range loca {
		binary.BigEndian.PutUint32(locaBytes[i*4:i*4+4], v)
	}
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)
	binary.BigEndian.PutUint16(head[50:52], 1) // long loca
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], 1)
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

func firstPoint(t *testing.T, scaled bool) Point {
	t.Helper()
	f, err := Parse(testCompositeFont(scaled))
	if err != nil {
		t.Fatal(err)
	}
	path, err := f.GlyphPath(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) == 0 || len(path[0]) == 0 {
		t.Fatal("the composite has no outline")
	}
	return path[0][0]
}

// TestScaledComponentOffset guards the 0x0800 flag: the component offset is
// scaled with the component, so (100,0) becomes (50,0).
func TestScaledComponentOffset(t *testing.T) {
	scaled := firstPoint(t, true)
	if math.Abs(float64(scaled.X-55)) > 0.01 || math.Abs(float64(scaled.Y-5)) > 0.01 {
		t.Fatalf("scaled point = (%v,%v), want (55,5)", scaled.X, scaled.Y)
	}
	unscaled := firstPoint(t, false)
	if math.Abs(float64(unscaled.X-105)) > 0.01 || math.Abs(float64(unscaled.Y-5)) > 0.01 {
		t.Fatalf("unscaled point = (%v,%v), want (105,5)", unscaled.X, unscaled.Y)
	}
}

// TestPointMatchingIsRejected guards RV4-40: without ARGS_ARE_XY_VALUES the two
// arguments are point indices, not an offset. Placing them as coordinates
// silently misplaces the component, so the parser refuses the glyph instead.
func TestPointMatchingIsRejected(t *testing.T) {
	f, err := Parse(testCompositeFontWithFlags(0x0008)) // WE_HAVE_A_SCALE only
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.GlyphPath(2); err == nil {
		t.Fatal("a point-matching composite was placed as an offset")
	}
}
