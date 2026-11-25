package font

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"slices"
)

// Subset builds a new TrueType font that keeps every glyph index but drops the
// outlines of unused glyphs. Keeping the numbering makes the subset cheap and
// safe: cmap, hmtx and composite references stay valid.
func (f *Font) Subset(used map[uint16]bool) ([]byte, error) {
	need := map[uint16]bool{0: true}
	var visit func(gid uint16, depth int)
	visit = func(gid uint16, depth int) {
		if need[gid] || depth > 8 || gid >= f.numGlyphs {
			return
		}
		need[gid] = true
		for _, comp := range f.components(gid) {
			visit(comp, depth+1)
		}
	}
	for g := range used {
		visit(g, 0)
	}

	newGlyf := make([]byte, 0, len(f.glyf)/4+1024)
	newLoca := make([]uint32, int(f.numGlyphs)+1)
	for gid := 0; gid < int(f.numGlyphs); gid++ {
		newLoca[gid] = uint32(len(newGlyf))
		if !need[uint16(gid)] {
			continue
		}
		data := f.glyphData(uint16(gid))
		newGlyf = append(newGlyf, data...)
		for len(newGlyf)%4 != 0 {
			newGlyf = append(newGlyf, 0)
		}
	}
	newLoca[len(newLoca)-1] = uint32(len(newGlyf))

	locaBytes := make([]byte, len(newLoca)*4)
	for i, v := range newLoca {
		binary.BigEndian.PutUint32(locaBytes[i*4:i*4+4], v)
	}

	head := append([]byte(nil), f.tables["head"]...)
	if len(head) < 54 {
		return nil, fmt.Errorf("font: head table is too short to subset")
	}
	binary.BigEndian.PutUint32(head[8:12], 0)  // checkSumAdjustment
	binary.BigEndian.PutUint16(head[50:52], 1) // long loca offsets

	tables := []outTable{
		{tag: "head", data: head},
		{tag: "hhea", data: append([]byte(nil), f.tables["hhea"]...)},
		{tag: "maxp", data: append([]byte(nil), f.tables["maxp"]...)},
		{tag: "hmtx", data: append([]byte(nil), f.tables["hmtx"]...)},
		{tag: "loca", data: locaBytes},
		{tag: "glyf", data: newGlyf},
	}
	for _, tag := range []string{"name", "OS/2", "cvt ", "fpgm", "prep"} {
		if t, ok := f.tables[tag]; ok {
			tables = append(tables, outTable{tag: tag, data: append([]byte(nil), t...)})
		}
	}

	return assembleFont(tables)
}

// assembleFont writes the tables as one sfnt and fills in the head table's
// checkSumAdjustment.
func assembleFont(tables []outTable) ([]byte, error) {
	out, headOffset, err := buildSFNT(tables)
	if err != nil {
		return nil, err
	}
	// checksumAdjustment is computed over the whole file with the field zeroed.
	sum := checksum(out)
	adjustment := uint32(0xB1B0AFBA) - sum
	// The spec keeps the head directory checksum computed with the adjustment
	// field zeroed; refreshing it here would change the sum again and leave
	// the font structurally invalid.
	binary.BigEndian.PutUint32(out[headOffset+8:headOffset+12], adjustment)
	return out, nil
}

type outTable struct {
	tag  string
	data []byte
}

func buildSFNT(tables []outTable) (out []byte, headOffset int, err error) {
	slices.SortFunc(tables, func(a, b outTable) int { return cmp.Compare(a.tag, b.tag) })
	numTables := len(tables)
	if numTables == 0 {
		return nil, 0, fmt.Errorf("font: nothing to write")
	}
	// searchRange, entrySelector, rangeShift are required by the spec.
	pow2 := 1
	for pow2*2 <= numTables {
		pow2 *= 2
	}
	headerSize := 12 + numTables*16
	out = make([]byte, headerSize)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(numTables))
	binary.BigEndian.PutUint16(out[6:8], uint16(pow2*16))
	log2 := 0
	for 1<<log2 <= pow2 {
		log2++
	}
	binary.BigEndian.PutUint16(out[8:10], uint16(log2-1))
	binary.BigEndian.PutUint16(out[10:12], uint16(numTables*16-pow2*16))

	offset := headerSize
	for i, t := range tables {
		rec := 12 + i*16
		copy(out[rec:rec+4], t.tag)
		binary.BigEndian.PutUint32(out[rec+4:rec+8], checksum(t.data))
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(offset))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(t.data)))
		if t.tag == "head" {
			headOffset = offset
		}
		out = append(out, t.data...)
		offset += len(t.data)
		for len(out)%4 != 0 {
			out = append(out, 0)
			offset++
		}
	}
	return out, headOffset, nil
}

func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var v uint32
		for j := 0; j < 4; j++ {
			var c byte
			if i+j < len(b) {
				c = b[i+j]
			}
			v = v<<8 | uint32(c)
		}
		sum += v
	}
	return sum
}
