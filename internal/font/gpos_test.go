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

	f := &Font{gpos: &kerning{pairs: map[uint32]int16{}}}
	f.gpos.parsePairPos(body, 0)
	if got := f.Kerning(65, 86); got != -100 {
		t.Fatalf("covered pair kerning = %d, want -100", got)
	}
	if got := f.Kerning(66, 86); got != 0 {
		t.Fatalf("glyph outside Coverage must not kern, got %d", got)
	}
}
