package font

import (
	"encoding/binary"
	"testing"
)

func coverageFormat1Test(gids []uint16) []byte {
	out := make([]byte, 4+2*len(gids))
	binary.BigEndian.PutUint16(out[0:2], 1)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(gids)))
	for i, gid := range gids {
		binary.BigEndian.PutUint16(out[4+2*i:6+2*i], gid)
	}
	return out
}

func classDefFormat1Test(start uint16, classes []uint16) []byte {
	out := make([]byte, 6+2*len(classes))
	binary.BigEndian.PutUint16(out[0:2], 1)
	binary.BigEndian.PutUint16(out[2:4], start)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(classes)))
	for i, class := range classes {
		binary.BigEndian.PutUint16(out[6+2*i:8+2*i], class)
	}
	return out
}

func TestClassKerningRequiresCoverage(t *testing.T) {
	// PairPos format 2: Coverage [65], class1 {65:1, 66:1}, class2 {86:1},
	// one record (-100) for (1,1). Glyph 66 is outside Coverage. The class1
	// records follow the header; coverage and class definitions follow them.
	classDef1 := classDefFormat1Test(65, []uint16{1, 1})
	classDef2 := classDefFormat1Test(86, []uint16{1})
	coverage := coverageFormat1Test([]uint16{65})
	records := make([]byte, 8)
	binary.BigEndian.PutUint16(records[6:8], uint16(0x10000-100)) // (1,1) = -100

	header := make([]byte, 16)
	binary.BigEndian.PutUint16(header[0:2], 2)   // format
	binary.BigEndian.PutUint16(header[4:6], 4)   // valueFormat1: XAdvance
	binary.BigEndian.PutUint16(header[12:14], 2) // class1Count
	binary.BigEndian.PutUint16(header[14:16], 2) // class2Count
	base := 16 + len(records)
	binary.BigEndian.PutUint16(header[2:4], uint16(base))
	binary.BigEndian.PutUint16(header[8:10], uint16(base+len(coverage)))
	binary.BigEndian.PutUint16(header[10:12], uint16(base+len(coverage)+len(classDef1)))

	body := append([]byte{}, header...)
	body = append(body, records...)
	body = append(body, coverage...)
	body = append(body, classDef1...)
	body = append(body, classDef2...)

	f := &Font{gpos: &kerning{}}
	f.gpos.parsePairPos(body, 0)
	if got := f.Kerning(65, 86); got != -100 {
		t.Fatalf("covered pair kerning = %d, want -100", got)
	}
	if got := f.Kerning(66, 86); got != 0 {
		t.Fatalf("glyph outside Coverage must not kern, got %d", got)
	}
}

func TestClassDefRangeIsBounded(t *testing.T) {
	classDef := []byte{
		0, 2, // format 2
		0, 1, // one range
		0, 0, 0xFF, 0xFF, // start 0, end 65535
		0, 1, // class 1
	}
	classes, ok := parseClassDef(classDef, 0)
	if !ok {
		t.Fatal("a well-formed class definition was rejected")
	}
	if len(classes) != 1 {
		t.Fatalf("ranges = %d", len(classes))
	}
	// The format 2 PairPos path must not expand this into 65k map entries.
	f := &Font{gpos: &kerning{}}
	if expanded := int(classes[0].end-classes[0].start) + 1; expanded <= 0 {
		t.Fatal("bad range arithmetic")
	}
	if got := f.Kerning(1, 1); got != 0 {
		t.Fatalf("unexpected kerning %d from an empty table", got)
	}
}

func coverageFormat2Test(ranges [][2]uint16) []byte {
	out := make([]byte, 4+6*len(ranges))
	binary.BigEndian.PutUint16(out[0:2], 2)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(ranges)))
	for i, r := range ranges {
		base := 4 + i*6
		binary.BigEndian.PutUint16(out[base:base+2], r[0])
		binary.BigEndian.PutUint16(out[base+2:base+4], r[1])
		binary.BigEndian.PutUint16(out[base+4:base+6], 0)
	}
	return out
}

// pairPosFormat2Test builds a class-based subtable with one class-0 record and
// the given Coverage.
func pairPosFormat2Test(coverage []byte) []byte {
	return pairPosFormat2WithClassDefs(coverage, classDefFormat1Test(0, []uint16{0}), classDefFormat1Test(0, []uint16{0}))
}

func pairPosFormat2WithClassDefs(coverage, class1, class2 []byte) []byte {
	classDef := class1
	classDef2 := class2
	header := make([]byte, 18)
	binary.BigEndian.PutUint16(header[0:2], 2)   // format
	binary.BigEndian.PutUint16(header[4:6], 4)   // valueFormat1: XAdvance
	binary.BigEndian.PutUint16(header[12:14], 1) // class1Count
	binary.BigEndian.PutUint16(header[14:16], 1) // class2Count
	binary.BigEndian.PutUint16(header[2:4], uint16(len(header)))
	binary.BigEndian.PutUint16(header[8:10], uint16(len(header)+len(coverage)))
	binary.BigEndian.PutUint16(header[10:12], uint16(len(header)+len(coverage)+len(classDef)))
	body := append([]byte{}, header...)
	body = append(body, coverage...)
	body = append(body, classDef...)
	body = append(body, classDef2...)
	return body
}

func TestClassDefExpansionIsBoundedThroughPairPos(t *testing.T) {
	// Four overlapping full-space ranges describe 262,144 glyph ids, more than
	// the shared budget. parsePairPos must stop before building the map.
	classDef := []byte{0, 2, 0, 4}
	for i := 0; i < 4; i++ {
		classDef = append(classDef, 0, 0, 0xFF, 0xFF, 0, 1)
	}
	body := pairPosFormat2WithClassDefs(coverageFormat1Test([]uint16{0}), classDef, classDefFormat1Test(0, []uint16{0}))
	k := &kerning{}
	k.parsePairPos(body, 0)
	for _, lookup := range k.lookups {
		if len(lookup.classes) != 0 || len(lookup.pairs) != 0 {
			t.Fatal("an over-budget class definition was expanded")
		}
	}
	if len(k.lookups) != 0 {
		t.Fatalf("an over-budget subtable produced %d lookup(s)", len(k.lookups))
	}
	if k.coverGlyphs > maxKerningGlyphs {
		t.Fatalf("the coverage budget was exceeded: %d", k.coverGlyphs)
	}
}

func TestCoverageFormat2ExpansionIsBounded(t *testing.T) {
	// Sixteen identical ranges 0..65535 are 100 bytes but describe 1,048,576
	// glyph ids. The expansion must be refused before anything is allocated.
	ranges := make([][2]uint16, 16)
	for i := range ranges {
		ranges[i] = [2]uint16{0, 65535}
	}
	coverage := coverageFormat2Test(ranges)
	if _, ok := parseCoverage(coverage, 0, maxKerningGlyphs); ok {
		t.Fatal("overlapping repeated ranges were accepted")
	}
}

func TestCoverageRejectsUnsortedAndOverlappingRanges(t *testing.T) {
	cases := map[string][][2]uint16{
		"overlap":  {{0, 10}, {5, 15}},
		"unsorted": {{20, 30}, {0, 10}},
		"reversed": {{10, 5}},
	}
	for name, ranges := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := parseCoverage(coverageFormat2Test(ranges), 0, maxKerningGlyphs); ok {
				t.Fatalf("%s ranges were accepted", name)
			}
		})
	}
}

func TestCoverageBudgetIsSharedAcrossSubtables(t *testing.T) {
	// Each subtable describes 65,535 glyphs. Three fit into the shared budget;
	// a fourth must be refused instead of allocating another 65k entries.
	coverage := coverageFormat2Test([][2]uint16{{0, 65534}})
	body := pairPosFormat2Test(coverage)
	k := &kerning{}
	for i := 0; i < 3; i++ {
		k.parsePairPos(body, 0)
	}
	if k.coverGlyphs != 3*65535 {
		t.Fatalf("three subtables expanded %d glyphs, want %d", k.coverGlyphs, 3*65535)
	}
	k.parsePairPos(body, 0)
	if k.coverGlyphs > maxKerningGlyphs {
		t.Fatalf("shared budget was exceeded: %d", k.coverGlyphs)
	}
	if k.coverGlyphs != 3*65535 {
		t.Fatalf("the fourth subtable spent budget it did not have: %d", k.coverGlyphs)
	}
}

// replaceTable writes a crafted table over an existing one. The new table must
// fit into the space of the original.
func replaceTable(t *testing.T, data []byte, tag string, table []byte) []byte {
	t.Helper()
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	for i := 0; i < numTables; i++ {
		rec := 12 + i*16
		if string(data[rec:rec+4]) != tag {
			continue
		}
		off := int(binary.BigEndian.Uint32(data[rec+8 : rec+12]))
		length := int(binary.BigEndian.Uint32(data[rec+12 : rec+16]))
		if len(table) > length {
			t.Skipf("the %s table is only %d bytes; need %d", tag, length, len(table))
		}
		out := append([]byte(nil), data...)
		copy(out[off:off+len(table)], table)
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(table)))
		return out
	}
	t.Skipf("the test font has no %s table", tag)
	return nil
}

// gposWithPairPos wraps one PairPos subtable in a minimal GPOS table.
func gposWithPairPos(subtable []byte) []byte {
	// GPOS header (10), LookupList at 10, one Lookup at 14, one subtable at 22.
	gpos := make([]byte, 22)
	binary.BigEndian.PutUint32(gpos[0:4], 0x00010000)
	binary.BigEndian.PutUint16(gpos[8:10], 10) // LookupList offset
	binary.BigEndian.PutUint16(gpos[10:12], 1) // lookup count
	binary.BigEndian.PutUint16(gpos[12:14], 4) // lookup offset from LookupList
	binary.BigEndian.PutUint16(gpos[14:16], 2) // lookup type 2
	binary.BigEndian.PutUint16(gpos[18:20], 1) // subtable count
	binary.BigEndian.PutUint16(gpos[20:22], 8) // subtable offset from Lookup
	return append(gpos, subtable...)
}

func TestFullFontParseRejectsOversizedCoverage(t *testing.T) {
	f := testFont(t)
	ranges := make([][2]uint16, 16)
	for i := range ranges {
		ranges[i] = [2]uint16{0, 65535}
	}
	gpos := gposWithPairPos(pairPosFormat2Test(coverageFormat2Test(ranges)))
	data := replaceTable(t, f.data, "GPOS", gpos)
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("a font with a corrupt optional GPOS table must stay usable: %v", err)
	}
	if parsed.gpos != nil {
		t.Fatal("the oversized Coverage was accepted")
	}
	if got := parsed.Kerning(0, 0); got != 0 {
		t.Fatalf("kerning = %d, want 0", got)
	}
}

func TestClassKerningUsesImplicitClassZero(t *testing.T) {
	// PairPos format 2: Coverage [65], ClassDef1 {65:1}, ClassDef2 {86:1},
	// records (1,0) = -50 and (1,1) = -100. Glyph 87 has no ClassDef2 entry,
	// so it belongs to class 0 and must use the (1,0) record.
	classDef1 := classDefFormat1Test(65, []uint16{1})
	classDef2 := classDefFormat1Test(86, []uint16{1})
	coverage := coverageFormat1Test([]uint16{65})
	records := make([]byte, 8)
	binary.BigEndian.PutUint16(records[4:6], uint16(0x10000-50))  // (1,0)
	binary.BigEndian.PutUint16(records[6:8], uint16(0x10000-100)) // (1,1)

	header := make([]byte, 16)
	binary.BigEndian.PutUint16(header[0:2], 2)   // format
	binary.BigEndian.PutUint16(header[4:6], 4)   // valueFormat1: XAdvance
	binary.BigEndian.PutUint16(header[12:14], 2) // class1Count
	binary.BigEndian.PutUint16(header[14:16], 2) // class2Count
	base := 16 + len(records)
	binary.BigEndian.PutUint16(header[2:4], uint16(base))
	binary.BigEndian.PutUint16(header[8:10], uint16(base+len(coverage)))
	binary.BigEndian.PutUint16(header[10:12], uint16(base+len(coverage)+len(classDef1)))

	body := append([]byte{}, header...)
	body = append(body, records...)
	body = append(body, coverage...)
	body = append(body, classDef1...)
	body = append(body, classDef2...)

	f := &Font{gpos: &kerning{}}
	f.gpos.parsePairPos(body, 0)
	if got := f.Kerning(65, 87); got != -50 {
		t.Fatalf("kerning for an implicit class 0 right glyph = %d, want -50", got)
	}
	if got := f.Kerning(65, 86); got != -100 {
		t.Fatalf("kerning for an explicit class 1 right glyph = %d, want -100", got)
	}
	if got := f.Kerning(66, 87); got != 0 {
		t.Fatalf("a left glyph outside Coverage must not kern, got %d", got)
	}
}
