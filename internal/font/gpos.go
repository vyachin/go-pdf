package font

import "encoding/binary"

// kerning holds horizontal pair adjustments from the GPOS table.
type kerning struct {
	pairs map[uint32]int16
	// class based positioning (PairPos format 2)
	class1 map[uint16]uint16
	// covered is the first-glyph Coverage of the class-based subtable: glyphs
	// outside it must not use the class records.
	covered map[uint16]bool
	records [][]int16
	class2  []classRange
	// coverGlyphs counts the glyph ids expanded from every Coverage of the
	// table, so repeated lookups and subtables share one budget.
	coverGlyphs int
}

// maxKerningGlyphs bounds the glyph ids a corrupt GPOS table may expand into
// slices and maps. A range pair costs six bytes in the file but may cover the
// whole glyph space, so the count must be checked before the allocation.
const maxKerningGlyphs = 200000

type classRange struct {
	start, end uint16
	class      uint16
}

func (f *Font) parseGPOS() {
	gpos, ok := f.tables["GPOS"]
	if !ok || len(gpos) < 10 {
		return
	}
	lookupList := int(binary.BigEndian.Uint16(gpos[8:10]))
	if lookupList == 0 || lookupList+2 > len(gpos) {
		return
	}
	k := &kerning{pairs: map[uint32]int16{}}
	count := int(binary.BigEndian.Uint16(gpos[lookupList : lookupList+2]))
	for i := 0; i < count; i++ {
		offPos := lookupList + 2 + i*2
		if offPos+2 > len(gpos) {
			break
		}
		off := lookupList + int(binary.BigEndian.Uint16(gpos[offPos:offPos+2]))
		if off+6 > len(gpos) {
			continue
		}
		lookupType := binary.BigEndian.Uint16(gpos[off : off+2])
		if lookupType != 2 {
			continue
		}
		subCount := int(binary.BigEndian.Uint16(gpos[off+4 : off+6]))
		for j := 0; j < subCount; j++ {
			subOffPos := off + 6 + j*2
			if subOffPos+2 > len(gpos) {
				break
			}
			subOff := off + int(binary.BigEndian.Uint16(gpos[subOffPos:subOffPos+2]))
			k.parsePairPos(gpos, subOff)
		}
	}
	if len(k.pairs) > 0 || len(k.records) > 0 {
		f.gpos = k
	}
}

func (k *kerning) parsePairPos(data []byte, off int) {
	if off+10 > len(data) {
		return
	}
	format := binary.BigEndian.Uint16(data[off : off+2])
	coverageOff := off + int(binary.BigEndian.Uint16(data[off+2:off+4]))
	valueFormat1 := binary.BigEndian.Uint16(data[off+4 : off+6])
	valueFormat2 := binary.BigEndian.Uint16(data[off+6 : off+8])
	remaining := maxKerningGlyphs - k.coverGlyphs
	if remaining <= 0 {
		return
	}
	cover, ok := parseCoverage(data, coverageOff, remaining)
	if !ok {
		return
	}
	k.coverGlyphs += len(cover)
	switch format {
	case 1:
		if off+10 > len(data) {
			return
		}
		pairSetCount := int(binary.BigEndian.Uint16(data[off+8 : off+10]))
		recordSize := valueRecordSize(valueFormat1) + valueRecordSize(valueFormat2)
		for i := 0; i < pairSetCount && i < len(cover); i++ {
			setPos := off + 10 + i*2
			if setPos+2 > len(data) {
				return
			}
			setOff := off + int(binary.BigEndian.Uint16(data[setPos:setPos+2]))
			if setOff+2 > len(data) {
				continue
			}
			n := int(binary.BigEndian.Uint16(data[setOff : setOff+2]))
			cursor := setOff + 2
			for j := 0; j < n; j++ {
				if cursor+2+recordSize > len(data) {
					return
				}
				second := binary.BigEndian.Uint16(data[cursor : cursor+2])
				cursor += 2
				adv, _ := xAdvance(data[cursor:cursor+valueRecordSize(valueFormat1)], valueFormat1)
				cursor += recordSize
				if adv != 0 {
					k.pairs[uint32(cover[i])<<16|uint32(second)] = adv
				}
			}
		}
	case 2:
		if off+16 > len(data) {
			return
		}
		classDef1Off := off + int(binary.BigEndian.Uint16(data[off+8:off+10]))
		classDef2Off := off + int(binary.BigEndian.Uint16(data[off+10:off+12]))
		class1Count := int(binary.BigEndian.Uint16(data[off+12 : off+14]))
		class2Count := int(binary.BigEndian.Uint16(data[off+14 : off+16]))
		classes1, ok := parseClassDef(data, classDef1Off)
		if !ok {
			return
		}
		if c2, ok := parseClassDef(data, classDef2Off); ok {
			k.class2 = c2
		}
		recordSize := valueRecordSize(valueFormat1) + valueRecordSize(valueFormat2)
		if recordSize == 0 {
			return
		}
		if class1Count*class2Count*recordSize+off+16 > len(data) {
			return
		}
		// A corrupt table can describe ranges covering the whole glyph space;
		// expanding them into a map is bounded instead.
		expanded := 0
		for _, r := range classes1 {
			if r.end < r.start {
				return
			}
			expanded += int(r.end-r.start) + 1
		}
		if expanded > maxKerningGlyphs {
			return
		}
		k.records = make([][]int16, class1Count)
		k.class1 = map[uint16]uint16{}
		k.covered = make(map[uint16]bool, len(cover))
		for _, gid := range cover {
			k.covered[gid] = true
		}
		for _, r := range classes1 {
			for g := uint32(r.start); g <= uint32(r.end); g++ {
				k.class1[uint16(g)] = r.class
			}
		}
		cursor := off + 16
		for c1 := 0; c1 < class1Count; c1++ {
			row := make([]int16, class2Count)
			for c2 := 0; c2 < class2Count; c2++ {
				if cursor+recordSize > len(data) {
					return
				}
				adv, _ := xAdvance(data[cursor:cursor+valueRecordSize(valueFormat1)], valueFormat1)
				row[c2] = adv
				cursor += recordSize
			}
			k.records[c1] = row
		}
	}
}

func valueRecordSize(vf uint16) int {
	size := 0
	for bit := uint16(1); bit <= 0x00FF; bit <<= 1 {
		if vf&bit != 0 {
			size += 2
		}
	}
	return size
}

// xAdvance extracts the XAdvance field from a value record.
func xAdvance(record []byte, vf uint16) (int16, bool) {
	const xAdvanceBit = 0x0004
	if vf&xAdvanceBit == 0 {
		return 0, false
	}
	offset := 0
	if vf&0x0001 != 0 {
		offset += 2
	}
	if vf&0x0002 != 0 {
		offset += 2
	}
	if offset+2 > len(record) {
		return 0, false
	}
	return int16(binary.BigEndian.Uint16(record[offset : offset+2])), true
}

// parseCoverage reads a Coverage table, expanding format 2 ranges. limit is
// the number of glyph ids the caller can still accept; ranges are counted
// before anything is appended, so a tiny table cannot request a huge slice.
func parseCoverage(data []byte, off int, limit int) ([]uint16, bool) {
	if off+4 > len(data) || limit <= 0 {
		return nil, false
	}
	format := binary.BigEndian.Uint16(data[off : off+2])
	switch format {
	case 1:
		n := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		if n > limit || off+4+n*2 > len(data) {
			return nil, false
		}
		out := make([]uint16, n)
		for i := 0; i < n; i++ {
			out[i] = binary.BigEndian.Uint16(data[off+4+i*2 : off+6+i*2])
			// The format requires strictly increasing glyph ids.
			if i > 0 && out[i] <= out[i-1] {
				return nil, false
			}
		}
		return out, true
	case 2:
		n := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		if off+4+n*6 > len(data) {
			return nil, false
		}
		// Count the expansion first: a repeated 0..65535 range would otherwise
		// append 65536 entries per six bytes.
		total := 0
		prevEnd := -1
		for i := 0; i < n; i++ {
			base := off + 4 + i*6
			start := int(binary.BigEndian.Uint16(data[base : base+2]))
			end := int(binary.BigEndian.Uint16(data[base+2 : base+4]))
			if end < start || start <= prevEnd {
				return nil, false
			}
			total += end - start + 1
			if total > limit {
				return nil, false
			}
			prevEnd = end
		}
		out := make([]uint16, 0, total)
		for i := 0; i < n; i++ {
			base := off + 4 + i*6
			start := binary.BigEndian.Uint16(data[base : base+2])
			end := binary.BigEndian.Uint16(data[base+2 : base+4])
			for g := uint32(start); g <= uint32(end); g++ {
				out = append(out, uint16(g))
			}
		}
		return out, true
	}
	return nil, false
}

func parseClassDef(data []byte, off int) ([]classRange, bool) {
	if off+4 > len(data) {
		return nil, false
	}
	format := binary.BigEndian.Uint16(data[off : off+2])
	switch format {
	case 1:
		// The header of a format 1 class definition is six bytes long; the
		// four-byte check at the top of this function is not enough.
		if off+6 > len(data) {
			return nil, false
		}
		start := binary.BigEndian.Uint16(data[off+2 : off+4])
		n := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
		if off+6+n*2 > len(data) {
			return nil, false
		}
		out := make([]classRange, n)
		for i := 0; i < n; i++ {
			g := start + uint16(i)
			out[i] = classRange{start: g, end: g, class: binary.BigEndian.Uint16(data[off+6+i*2 : off+8+i*2])}
		}
		return out, true
	case 2:
		n := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		if off+4+n*6 > len(data) {
			return nil, false
		}
		out := make([]classRange, n)
		for i := 0; i < n; i++ {
			base := off + 4 + i*6
			out[i] = classRange{
				start: binary.BigEndian.Uint16(data[base : base+2]),
				end:   binary.BigEndian.Uint16(data[base+2 : base+4]),
				class: binary.BigEndian.Uint16(data[base+4 : base+6]),
			}
		}
		return out, true
	}
	return nil, false
}

// classOf returns the class of a glyph for a sorted range table.
func classOf(ranges []classRange, gid uint16) (uint16, bool) {
	lo, hi := 0, len(ranges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		r := ranges[mid]
		switch {
		case gid < r.start:
			hi = mid - 1
		case gid > r.end:
			lo = mid + 1
		default:
			return r.class, true
		}
	}
	return 0, false
}

// Kerning returns the horizontal adjustment between two glyphs in font units.
func (f *Font) Kerning(left, right uint16) int32 {
	if f.gpos == nil {
		return 0
	}
	if v, ok := f.gpos.pairs[uint32(left)<<16|uint32(right)]; ok {
		return int32(v)
	}
	if f.gpos.records == nil {
		return 0
	}
	if f.gpos.covered != nil && !f.gpos.covered[left] {
		return 0
	}
	c1, ok := f.gpos.class1[left]
	if !ok || int(c1) >= len(f.gpos.records) {
		return 0
	}
	c2, ok := classOf(f.gpos.class2, right)
	if !ok || int(c2) >= len(f.gpos.records[c1]) {
		return 0
	}
	return int32(f.gpos.records[c1][c2])
}
