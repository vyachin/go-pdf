package font

import "encoding/binary"

// markAnchor is one GPOS anchor point in font units.
type markAnchor struct {
	x, y int16
	set  bool
}

// markRecord is a mark glyph's class and its own anchor.
type markRecord struct {
	class  uint16
	anchor markAnchor
}

// markLookup is one MarkBase (4), MarkLig (5) or MarkMark (6) lookup. bases is
// keyed by the base, ligature or mark2 glyph and holds one anchor per class.
type markLookup struct {
	kind  int
	marks map[uint16]markRecord
	bases map[uint16][]markAnchor
	// ignoreMarks is the lookup's IgnoreMarks flag and filter its mark
	// filtering set: a mark outside the set does not attach.
	ignoreMarks bool
	filter      map[uint16]bool
}

// markAttachment collects the mark lookups of a font in table order.
type markAttachment struct {
	lookups []*markLookup
}

// maxMarkClasses bounds the class count of one mark subtable; real fonts use a
// handful, and the bound keeps a corrupt count from sizing a large table.
const maxMarkClasses = 256

// MarkAttachment returns the offset that places mark's anchor on base's anchor,
// in font units, using the first mark lookup that covers both glyphs. kind is
// the lookup type: MarkBase (4), MarkLig (5) or MarkMark (6). When the previous
// glyph is a base, 4 and 5 apply; when it is a mark, 6 applies.
func (f *Font) MarkAttachment(mark, base uint16) (dx, dy int16, kind int, ok bool) {
	if f.marks == nil {
		return 0, 0, 0, false
	}
	for _, lookup := range f.marks.lookups {
		rec, ok := lookup.marks[mark]
		if !ok {
			continue
		}
		if lookup.filter != nil && !lookup.filter[mark] {
			// The lookup's mark filtering set excludes this mark.
			continue
		}
		anchors, ok := lookup.bases[base]
		if !ok || int(rec.class) >= len(anchors) {
			continue
		}
		baseAnchor := anchors[rec.class]
		if !baseAnchor.set {
			continue
		}
		return baseAnchor.x - rec.anchor.x, baseAnchor.y - rec.anchor.y, lookup.kind, true
	}
	return 0, 0, 0, false
}

// parseMarkLookup parses one MarkBase, MarkLig or MarkMark subtable. A
// malformed table returns nil, so the rest of the font still loads.
func (k *kerning) parseMarkLookup(data []byte, off, kind int) (*markLookup, error) {
	if !fits(off, 0, 12, len(data)) {
		return nil, nil
	}
	if binary.BigEndian.Uint16(data[off:off+2]) != 1 {
		return nil, nil
	}
	markCov := off + int(binary.BigEndian.Uint16(data[off+2:off+4]))
	baseCov := off + int(binary.BigEndian.Uint16(data[off+4:off+6]))
	classCount := int(binary.BigEndian.Uint16(data[off+6 : off+8]))
	if classCount == 0 || classCount > maxMarkClasses {
		return nil, nil
	}
	markArray := off + int(binary.BigEndian.Uint16(data[off+8:off+10]))
	baseArray := off + int(binary.BigEndian.Uint16(data[off+10:off+12]))

	remaining := maxKerningGlyphs - k.coverGlyphs
	markGlyphs, ok, err := k.parseCoverage(data, markCov, remaining)
	if err != nil {
		return nil, err
	}
	if !ok || len(markGlyphs) == 0 {
		return nil, nil
	}
	k.coverGlyphs += len(markGlyphs)
	remaining -= len(markGlyphs)
	baseGlyphs, ok, err := k.parseCoverage(data, baseCov, remaining)
	if err != nil {
		return nil, err
	}
	if !ok || len(baseGlyphs) == 0 {
		return nil, nil
	}
	k.coverGlyphs += len(baseGlyphs)

	marks := k.parseMarkArray(data, markArray, markGlyphs)
	if len(marks) == 0 {
		return nil, nil
	}
	var bases map[uint16][]markAnchor
	if kind == 5 {
		bases = k.parseLigatureArray(data, baseArray, baseGlyphs, classCount)
	} else {
		bases = k.parseBaseArray(data, baseArray, baseGlyphs, classCount)
	}
	if len(bases) == 0 {
		return nil, nil
	}
	return &markLookup{kind: kind, marks: marks, bases: bases}, nil
}

// parseMarkArray reads the mark anchors in coverage order. A mark record with
// no anchor is skipped: it cannot be attached.
func (k *kerning) parseMarkArray(data []byte, off int, glyphs []uint16) map[uint16]markRecord {
	if !fits(off, 0, 2, len(data)) {
		return nil
	}
	count := int(binary.BigEndian.Uint16(data[off : off+2]))
	if count > len(glyphs) {
		count = len(glyphs)
	}
	out := make(map[uint16]markRecord, count)
	for i := 0; i < count; i++ {
		rec := off + 2 + i*4
		if !fits(rec, 0, 4, len(data)) {
			break
		}
		class := binary.BigEndian.Uint16(data[rec : rec+2])
		anchorOff := int(binary.BigEndian.Uint16(data[rec+2 : rec+4]))
		if anchorOff == 0 {
			continue
		}
		anchor, ok := readMarkAnchor(data, off+anchorOff)
		if !ok {
			continue
		}
		out[glyphs[i]] = markRecord{class: class, anchor: anchor}
	}
	return out
}

// parseBaseArray reads the base or mark2 anchors, one row of classCount
// anchors per covered glyph.
func (k *kerning) parseBaseArray(data []byte, off int, glyphs []uint16, classCount int) map[uint16][]markAnchor {
	if !fits(off, 0, 2, len(data)) {
		return nil
	}
	count := int(binary.BigEndian.Uint16(data[off : off+2]))
	if count > len(glyphs) {
		count = len(glyphs)
	}
	if !fits(off, 2, uint64(count)*uint64(classCount)*2, len(data)) {
		return nil
	}
	out := make(map[uint16][]markAnchor, count)
	for i := 0; i < count; i++ {
		anchors := make([]markAnchor, classCount)
		for c := 0; c < classCount; c++ {
			pos := off + 2 + (i*classCount+c)*2
			anchorOff := int(binary.BigEndian.Uint16(data[pos : pos+2]))
			if anchorOff == 0 {
				continue
			}
			if anchor, ok := readMarkAnchor(data, off+anchorOff); ok {
				anchors[c] = anchor
			}
		}
		out[glyphs[i]] = anchors
	}
	return out
}

// parseLigatureArray reads the first component's anchors for every ligature
// glyph. The engine does not form ligatures, so only component 0 can receive a
// mark; the offsets of the other components are still validated by the table
// bounds checks.
func (k *kerning) parseLigatureArray(data []byte, off int, glyphs []uint16, classCount int) map[uint16][]markAnchor {
	if !fits(off, 0, 2, len(data)) {
		return nil
	}
	count := int(binary.BigEndian.Uint16(data[off : off+2]))
	if count > len(glyphs) {
		count = len(glyphs)
	}
	out := make(map[uint16][]markAnchor, count)
	for i := 0; i < count; i++ {
		pos := off + 2 + i*2
		if !fits(pos, 0, 2, len(data)) {
			break
		}
		attachOff := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if attachOff == 0 {
			continue
		}
		attach := off + attachOff
		if !fits(attach, 0, 2, len(data)) {
			continue
		}
		components := int(binary.BigEndian.Uint16(data[attach : attach+2]))
		if components == 0 || !fits(attach, 2, uint64(classCount)*2, len(data)) {
			continue
		}
		anchors := make([]markAnchor, classCount)
		for c := 0; c < classCount; c++ {
			apos := attach + 2 + c*2
			anchorOff := int(binary.BigEndian.Uint16(data[apos : apos+2]))
			if anchorOff == 0 {
				continue
			}
			if anchor, ok := readMarkAnchor(data, attach+anchorOff); ok {
				anchors[c] = anchor
			}
		}
		out[glyphs[i]] = anchors
	}
	return out
}

// readMarkAnchor reads an Anchor table. Formats 2 and 3 add information after
// the x/y point that mark attachment does not need.
func readMarkAnchor(data []byte, off int) (markAnchor, bool) {
	if !fits(off, 0, 6, len(data)) {
		return markAnchor{}, false
	}
	format := binary.BigEndian.Uint16(data[off : off+2])
	if format < 1 || format > 3 {
		return markAnchor{}, false
	}
	return markAnchor{
		x:   int16(binary.BigEndian.Uint16(data[off+2 : off+4])),
		y:   int16(binary.BigEndian.Uint16(data[off+4 : off+6])),
		set: true,
	}, true
}
