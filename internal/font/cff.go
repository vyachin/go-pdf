package font

import (
	"encoding/binary"
	"fmt"

	"github.com/vyachin/go-pdf/internal/budget"
)

// A CFF table is read here, not by the TrueType parser: an OpenType font with
// PostScript outlines carries its glyphs as Type 2 charstrings, which the
// TrueType path cannot read and the PDF writer cannot embed. The conversion in
// cff_convert.go needs only the charstrings and the subroutines that draw them,
// so that is all this file keeps.
//
// golang.org/x/image/font/sfnt is the test oracle for the reader, but it is not
// used at run time: its charstring interpreter implements hflex and hflex1 and
// stops at flex and flex1, which the corpus fonts use in ordinary characters.

// cffMaxSubrDepth is the nesting limit Type 2 puts on callsubr and callgsubr.
const cffMaxSubrDepth = 10

// cffMaxHints is the greatest number of stem hints one charstring may declare.
// The hint mask is a fixed part of the font format, and 96 is the limit the
// reference implementations agree on.
const cffMaxHints = 96

// cffTable is the part of a CFF 1 table the outline conversion needs.
type cffTable struct {
	charStrings [][]byte
	// guard bounds the outline conversion of this table and carries
	// cancellation. Nil means unlimited work.
	guard *budget.Guard
	// instructions counts the charstring instructions executed across every
	// glyph of this font, the font-wide bound on the conversion.
	instructions int
	gsubrs       [][]byte
	fds          []cffFontDict
	// fdSelect maps a glyph to its entry in fds. It is nil for a name-keyed
	// font, which has one font dict.
	fdSelect []uint8
}

// cffFontDict is one font dict of a CID-keyed font, or the only dict of a
// name-keyed one. Only the local subroutines matter for drawing.
type cffFontDict struct {
	subrs [][]byte
}

// The DICT operators read here. A two-byte operator is keyed as 0x0c00 plus
// its second byte, the same way the charstring reader keys the escaped
// operators.
const (
	cffOpCharStrings = 17
	cffOpPrivate     = 18
	cffOpSubrs       = 19
	cffOpROS         = 0x0c1e
	cffOpFDArray     = 0x0c24
	cffOpFDSelect    = 0x0c25
)

// cffDictValue is one DICT operand. A real number is remembered as such so that
// an operator that needs an offset cannot read a silent zero.
type cffDictValue struct {
	integer int32
	real    bool
}

// cffDict maps an operator to its operands.
type cffDict map[uint16][]cffDictValue

// parseCFFTable reads the CFF table of an sfnt. numGlyphs comes from maxp and
// must agree with the CharStrings index: the converter addresses glyphs by the
// same index, so a disagreement means the font is broken and no glyph id would
// be trustworthy.
func parseCFFTable(data []byte, numGlyphs int) (*cffTable, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("font: CFF table is truncated")
	}
	if data[0] != 1 {
		return nil, fmt.Errorf("font: CFF version %d.%d is not supported", data[0], data[1])
	}
	hdrSize := int(data[2])
	if hdrSize < 4 || hdrSize > len(data) {
		return nil, fmt.Errorf("font: CFF header size %d is outside the table", hdrSize)
	}
	t := &cffTable{}

	// Name INDEX, Top DICT INDEX, String INDEX, Global Subr INDEX, in order.
	off := hdrSize
	if _, next, err := cffIndex(data, off); err != nil {
		return nil, err
	} else {
		off = next
	}
	topDicts, off, err := cffIndex(data, off)
	if err != nil {
		return nil, err
	}
	if len(topDicts) == 0 {
		return nil, fmt.Errorf("font: CFF has no Top DICT")
	}
	if _, off, err = cffIndex(data, off); err != nil {
		return nil, err
	}
	if t.gsubrs, _, err = cffIndex(data, off); err != nil {
		return nil, err
	}
	top, err := parseCFFDict(topDicts[0])
	if err != nil {
		return nil, err
	}

	charStringsOffset, err := cffDictOffset(top, cffOpCharStrings, "CharStrings")
	if err != nil {
		return nil, err
	}
	if t.charStrings, _, err = cffIndex(data, charStringsOffset); err != nil {
		return nil, err
	}
	if len(t.charStrings) != numGlyphs {
		return nil, fmt.Errorf("font: the CFF table has %d charstrings, maxp names %d glyphs", len(t.charStrings), numGlyphs)
	}

	_, isCID := top[cffOpROS]
	if !isCID {
		fd, err := cffFontDictAt(data, top)
		if err != nil {
			return nil, err
		}
		t.fds = []cffFontDict{fd}
		return t, nil
	}

	fdArrayOffset, err := cffDictOffset(top, cffOpFDArray, "FDArray")
	if err != nil {
		return nil, err
	}
	fdDicts, _, err := cffIndex(data, fdArrayOffset)
	if err != nil {
		return nil, err
	}
	if len(fdDicts) == 0 {
		return nil, fmt.Errorf("font: CFF FDArray is empty")
	}
	for i, entry := range fdDicts {
		dict, err := parseCFFDict(entry)
		if err != nil {
			return nil, err
		}
		fd, err := cffFontDictAt(data, dict)
		if err != nil {
			return nil, fmt.Errorf("font: CFF FDArray entry %d: %w", i, err)
		}
		t.fds = append(t.fds, fd)
	}
	fdSelectOffset, err := cffDictOffset(top, cffOpFDSelect, "FDSelect")
	if err != nil {
		return nil, err
	}
	if t.fdSelect, err = parseCFFFDSelect(data, fdSelectOffset, numGlyphs, len(t.fds)); err != nil {
		return nil, err
	}
	return t, nil
}

// cffFontDictAt reads the Private DICT a font dict points at and returns the
// local subroutines it declares.
func cffFontDictAt(data []byte, dict cffDict) (cffFontDict, error) {
	vals, ok := dict.intOperands(cffOpPrivate)
	if !ok || len(vals) != 2 {
		// A font without a Private DICT has no local subroutines; the table is
		// legal and the oracle loads it, so treat it as an empty Private DICT.
		return cffFontDict{}, nil
	}
	// DICT operands are int32. Their sum must be widened before it is
	// compared: on a 32-bit build offset+size wraps negative and would pass a
	// check written in int.
	size, offset := int64(vals[0]), int64(vals[1])
	if size < 0 || offset < 0 || offset+size > int64(len(data)) {
		return cffFontDict{}, fmt.Errorf("font: CFF Private DICT is outside the table")
	}
	private, err := parseCFFDict(data[int(offset):int(offset+size)])
	if err != nil {
		return cffFontDict{}, err
	}
	subrs, ok := private.intOperands(cffOpSubrs)
	if !ok {
		return cffFontDict{}, nil
	}
	if len(subrs) != 1 || subrs[0] < 0 {
		return cffFontDict{}, fmt.Errorf("font: CFF Subrs offset is invalid")
	}
	// Subrs is relative to the start of the Private DICT.
	subrsOffset := offset + int64(subrs[0])
	if subrsOffset < 0 || subrsOffset > int64(len(data)) {
		return cffFontDict{}, fmt.Errorf("font: CFF Subrs offset is outside the table")
	}
	entries, _, err := cffIndex(data, int(subrsOffset))
	if err != nil {
		return cffFontDict{}, err
	}
	return cffFontDict{subrs: entries}, nil
}

// cffIndex reads one INDEX structure at off and returns its entries and the
// offset just past it.
func cffIndex(data []byte, off int) (entries [][]byte, next int, err error) {
	if off < 0 || !fits(off, 0, 2, len(data)) {
		return nil, 0, fmt.Errorf("font: CFF INDEX at offset %d is truncated", off)
	}
	count := int(binary.BigEndian.Uint16(data[off : off+2]))
	off += 2
	if count == 0 {
		return nil, off, nil
	}
	if uint64(off) >= uint64(len(data)) {
		return nil, 0, fmt.Errorf("font: CFF INDEX at offset %d is truncated", off)
	}
	offSize := int(data[off])
	off++
	if offSize < 1 || offSize > 4 {
		return nil, 0, fmt.Errorf("font: CFF INDEX offSize %d is invalid", offSize)
	}
	if !fits(off, 0, uint64(offSize*(count+1)), len(data)) {
		return nil, 0, fmt.Errorf("font: CFF INDEX offsets are truncated")
	}
	// Offsets are one-based and relative to the byte before the data. They are
	// read and added in int64: a four-byte offset can be above MaxInt32, which
	// would wrap negative on a 32-bit build.
	dataStart := int64(off + offSize*(count+1) - 1)
	offsets := make([]int64, count+1)
	for i := range offsets {
		var v int64
		for j := 0; j < offSize; j++ {
			v = v<<8 | int64(data[off+i*offSize+j])
		}
		if v < 1 {
			return nil, 0, fmt.Errorf("font: CFF INDEX offset %d is zero", i)
		}
		offsets[i] = v
	}
	entries = make([][]byte, count)
	for i := 0; i < count; i++ {
		start, end := dataStart+offsets[i], dataStart+offsets[i+1]
		if offsets[i] > offsets[i+1] || start < 0 || end > int64(len(data)) {
			return nil, 0, fmt.Errorf("font: CFF INDEX entry %d is outside the table", i)
		}
		entries[i] = data[int(start):int(end)]
	}
	nextOffset := dataStart + offsets[count]
	if nextOffset < 0 || nextOffset > int64(len(data)) {
		return nil, 0, fmt.Errorf("font: CFF INDEX end is outside the table")
	}
	return entries, int(nextOffset), nil
}

// parseCFFDict reads one DICT. The operand list is kept per operator; operators
// this reader does not use are still parsed so that their operands cannot be
// mistaken for the next operator's.
func parseCFFDict(data []byte) (cffDict, error) {
	dict := cffDict{}
	var operands []cffDictValue
	flush := func(op uint16) {
		dict[op] = append([]cffDictValue(nil), operands...)
		operands = operands[:0]
	}
	for i := 0; i < len(data); {
		b := data[i]
		switch {
		case b == 28:
			if i+3 > len(data) {
				return nil, fmt.Errorf("font: CFF DICT ends inside a number")
			}
			operands = append(operands, cffDictValue{integer: int32(int16(binary.BigEndian.Uint16(data[i+1:])))})
			i += 3
		case b == 29:
			if i+5 > len(data) {
				return nil, fmt.Errorf("font: CFF DICT ends inside a number")
			}
			operands = append(operands, cffDictValue{integer: int32(binary.BigEndian.Uint32(data[i+1:]))})
			i += 5
		case b == 30:
			next, err := skipCFFReal(data, i+1)
			if err != nil {
				return nil, err
			}
			operands = append(operands, cffDictValue{real: true})
			i = next
		case b >= 32 && b <= 246:
			operands = append(operands, cffDictValue{integer: int32(b) - 139})
			i++
		case b >= 247 && b <= 250:
			if i+2 > len(data) {
				return nil, fmt.Errorf("font: CFF DICT ends inside a number")
			}
			operands = append(operands, cffDictValue{integer: int32(b-247)*256 + int32(data[i+1]) + 108})
			i += 2
		case b >= 251 && b <= 254:
			if i+2 > len(data) {
				return nil, fmt.Errorf("font: CFF DICT ends inside a number")
			}
			operands = append(operands, cffDictValue{integer: -(int32(b-251)*256 + int32(data[i+1]) + 108)})
			i += 2
		case b == 12:
			if i+2 > len(data) {
				return nil, fmt.Errorf("font: CFF DICT ends inside an operator")
			}
			flush(0x0c00 | uint16(data[i+1]))
			i += 2
		default:
			// A one-byte operator, including the reserved codes. Nothing here
			// reads a reserved operator, but its operands still belong to it.
			flush(uint16(b))
			i++
		}
	}
	return dict, nil
}

// skipCFFReal walks the nibbles of a real number operand and returns the offset
// just past its terminating 0x0f nibble.
func skipCFFReal(data []byte, off int) (int, error) {
	for ; off < len(data); off++ {
		for _, nib := range [2]byte{data[off] >> 4, data[off] & 0x0f} {
			switch nib {
			case 0x0f:
				return off + 1, nil
			case 0x0d:
				return 0, fmt.Errorf("font: CFF DICT real number contains a reserved nibble")
			}
		}
	}
	return 0, fmt.Errorf("font: CFF DICT ends inside a real number")
}

// parseCFFFDSelect reads the font dict selection of a CID-keyed font.
func parseCFFFDSelect(data []byte, off, numGlyphs, numFDs int) ([]uint8, error) {
	if off < 0 || uint64(off) >= uint64(len(data)) {
		return nil, fmt.Errorf("font: CFF FDSelect is outside the table")
	}
	switch format := data[off]; format {
	case 0:
		if !fits(off, 1, uint64(numGlyphs), len(data)) {
			return nil, fmt.Errorf("font: CFF FDSelect is truncated")
		}
		out := make([]uint8, numGlyphs)
		for gid := 0; gid < numGlyphs; gid++ {
			if fd := data[off+1+gid]; int(fd) >= numFDs {
				return nil, fmt.Errorf("font: CFF FDSelect names font dict %d for glyph %d, there are %d", fd, gid, numFDs)
			}
			out[gid] = data[off+1+gid]
		}
		return out, nil
	case 3:
		if !fits(off, 0, 3, len(data)) {
			return nil, fmt.Errorf("font: CFF FDSelect is truncated")
		}
		nRanges := int(binary.BigEndian.Uint16(data[off+1:]))
		if nRanges == 0 {
			return nil, fmt.Errorf("font: CFF FDSelect has no ranges")
		}
		pos := off + 3
		if !fits(pos, 0, uint64(nRanges)*3+2, len(data)) {
			return nil, fmt.Errorf("font: CFF FDSelect ranges are truncated")
		}
		firsts := make([]int, nRanges)
		fds := make([]uint8, nRanges)
		for r := 0; r < nRanges; r++ {
			firsts[r] = int(binary.BigEndian.Uint16(data[pos:]))
			fds[r] = data[pos+2]
			pos += 3
		}
		// The sentinel is the glyph count: a value that stops early leaves
		// glyphs without a font dict, and one that overshoots names glyphs the
		// font does not have.
		end := int(binary.BigEndian.Uint16(data[pos:]))
		if firsts[0] != 0 || end != numGlyphs {
			return nil, fmt.Errorf("font: CFF FDSelect does not cover the %d glyphs", numGlyphs)
		}
		out := make([]uint8, numGlyphs)
		for r := 0; r < nRanges; r++ {
			next := end
			if r+1 < nRanges {
				next = firsts[r+1]
			}
			if firsts[r] >= next || int(fds[r]) >= numFDs {
				return nil, fmt.Errorf("font: CFF FDSelect range %d is invalid", r)
			}
			for gid := firsts[r]; gid < next; gid++ {
				out[gid] = fds[r]
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("font: CFF FDSelect format %d is not supported", format)
	}
}

// intOperands returns the operands of op when every one of them is an integer.
func (d cffDict) intOperands(op uint16) ([]int32, bool) {
	vals, ok := d[op]
	if !ok {
		return nil, false
	}
	out := make([]int32, len(vals))
	for i, v := range vals {
		if v.real {
			return nil, false
		}
		out[i] = v.integer
	}
	return out, true
}

// cffDictOffset returns the one non-negative offset operand of op.
func cffDictOffset(d cffDict, op uint16, name string) (int, error) {
	vals, ok := d.intOperands(op)
	if !ok || len(vals) != 1 || vals[0] < 0 {
		return 0, fmt.Errorf("font: CFF %s offset is missing or invalid", name)
	}
	return int(vals[0]), nil
}
