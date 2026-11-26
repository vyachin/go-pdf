package testfont

import (
	"encoding/binary"
	"testing"
)

// This file edits a real font's GPOS table so the shaping tests can exercise
// paths the bundled faces do not provide: a font without a mark-to-mark anchor
// and a font whose base anchors are wrong. The tables are edited by hand
// because a test cannot import a build helper from internal/font, and a
// hand-built sfnt would carry none of the glyphs and lookups the tests need.

// WithoutMarkMark returns a copy of data with every mark-to-mark lookup
// disabled. MarkBase and MarkLig lookups, kerning and the rest of the font stay
// intact, so the copy is a font in which a second mark has to fall back to the
// base anchor.
func WithoutMarkMark(tb testing.TB, data []byte) []byte {
	tb.Helper()
	out := append([]byte(nil), data...)
	gpos, ok := tableRange(out, "GPOS")
	if !ok {
		tb.Fatal("the font has no GPOS table")
	}
	changed := 0
	walkLookups(tb, out, gpos, func(lookupOff int, lookupType uint16) {
		switch lookupType {
		case 6:
			// An unknown lookup type is skipped by the parser, so the whole
			// mark-to-mark lookup disappears without touching its subtables.
			binary.BigEndian.PutUint16(out[lookupOff:lookupOff+2], 0)
			changed++
		case 9:
			subCount := int(binary.BigEndian.Uint16(out[lookupOff+4 : lookupOff+6]))
			for j := 0; j < subCount; j++ {
				pos := lookupOff + 6 + j*2
				if !fits(out, pos, 2) {
					continue
				}
				subOff := lookupOff + int(binary.BigEndian.Uint16(out[pos:pos+2]))
				if !fits(out, subOff, 8) || binary.BigEndian.Uint16(out[subOff:subOff+2]) != 1 {
					continue
				}
				if binary.BigEndian.Uint16(out[subOff+2:subOff+4]) != 6 {
					continue
				}
				binary.BigEndian.PutUint16(out[subOff+2:subOff+4], 0)
				changed++
			}
		}
	})
	if changed == 0 {
		tb.Fatal("the font has no mark-to-mark lookup to disable")
	}
	return out
}

// WithMarkBaseAnchorsX returns a copy of data with the x coordinate of every
// MarkBase anchor replaced by x. It builds the crafted anchor the validation
// test needs: a font that declares a syntactically valid anchor painting every
// attached mark at the same wrong place.
func WithMarkBaseAnchorsX(tb testing.TB, data []byte, x int16) []byte {
	tb.Helper()
	out := append([]byte(nil), data...)
	gpos, ok := tableRange(out, "GPOS")
	if !ok {
		tb.Fatal("the font has no GPOS table")
	}
	changed := 0
	walkLookups(tb, out, gpos, func(lookupOff int, lookupType uint16) {
		subCount := int(binary.BigEndian.Uint16(out[lookupOff+4 : lookupOff+6]))
		for j := 0; j < subCount; j++ {
			pos := lookupOff + 6 + j*2
			if !fits(out, pos, 2) {
				continue
			}
			subOff := lookupOff + int(binary.BigEndian.Uint16(out[pos:pos+2]))
			subType := lookupType
			if lookupType == 9 {
				var ok bool
				subOff, subType, ok = extensionSubtable(out, subOff)
				if !ok {
					continue
				}
			}
			if subType != 4 {
				continue
			}
			changed += setBaseArrayX(tb, out, subOff, x)
		}
	})
	if changed == 0 {
		tb.Fatal("the font has no MarkBase anchor to edit")
	}
	return out
}

// walkLookups calls fn for every lookup in the GPOS lookup list, with the
// lookup's absolute offset in data and its lookup type.
func walkLookups(tb testing.TB, data []byte, gpos int, fn func(lookupOff int, lookupType uint16)) {
	tb.Helper()
	if !fits(data, gpos, 10) {
		tb.Fatal("the GPOS table is too short")
	}
	listOff := gpos + int(binary.BigEndian.Uint16(data[gpos+8:gpos+10]))
	if !fits(data, listOff, 2) {
		tb.Fatal("the GPOS lookup list is outside the table")
	}
	count := int(binary.BigEndian.Uint16(data[listOff : listOff+2]))
	for i := 0; i < count; i++ {
		pos := listOff + 2 + i*2
		if !fits(data, pos, 2) {
			tb.Fatalf("GPOS lookup %d is outside the table", i)
		}
		lookupOff := listOff + int(binary.BigEndian.Uint16(data[pos:pos+2]))
		if !fits(data, lookupOff, 6) {
			continue
		}
		fn(lookupOff, binary.BigEndian.Uint16(data[lookupOff:lookupOff+2]))
	}
}

// extensionSubtable resolves an ExtensionPos (type 9) subtable to the real
// lookup type and the real subtable's absolute offset.
func extensionSubtable(data []byte, off int) (int, uint16, bool) {
	if !fits(data, off, 8) || binary.BigEndian.Uint16(data[off:off+2]) != 1 {
		return 0, 0, false
	}
	kind := binary.BigEndian.Uint16(data[off+2 : off+4])
	real := off + int(binary.BigEndian.Uint32(data[off+4:off+8]))
	if !fits(data, real, 2) {
		return 0, 0, false
	}
	return real, kind, true
}

// setBaseArrayX replaces the x coordinate of every anchor in a MarkBase
// subtable's BaseArray and returns how many anchors changed.
func setBaseArrayX(tb testing.TB, data []byte, subOff int, x int16) int {
	tb.Helper()
	// format, markCoverage, baseCoverage, classCount, markArray, baseArray
	if !fits(data, subOff, 12) || binary.BigEndian.Uint16(data[subOff:subOff+2]) != 1 {
		return 0
	}
	classCount := int(binary.BigEndian.Uint16(data[subOff+6 : subOff+8]))
	if classCount == 0 {
		return 0
	}
	arrayOff := subOff + int(binary.BigEndian.Uint16(data[subOff+10:subOff+12]))
	if !fits(data, arrayOff, 2) {
		return 0
	}
	count := int(binary.BigEndian.Uint16(data[arrayOff : arrayOff+2]))
	if !fits(data, arrayOff+2, uint64(count)*uint64(classCount)*2) {
		return 0
	}
	changed := 0
	for i := 0; i < count; i++ {
		for c := 0; c < classCount; c++ {
			pos := arrayOff + 2 + (i*classCount+c)*2
			rel := int(binary.BigEndian.Uint16(data[pos : pos+2]))
			if rel == 0 {
				continue
			}
			anchor := arrayOff + rel
			if !fits(data, anchor, 6) {
				continue
			}
			binary.BigEndian.PutUint16(data[anchor+2:anchor+4], uint16(x))
			changed++
		}
	}
	return changed
}

// tableRange finds the offset of a table in the sfnt directory and checks that
// its declared length stays inside the file.
func tableRange(data []byte, tag string) (int, bool) {
	if !fits(data, 0, 12) {
		return 0, false
	}
	count := int(binary.BigEndian.Uint16(data[4:6]))
	for i := 0; i < count; i++ {
		rec := 12 + i*16
		if !fits(data, rec, 16) {
			return 0, false
		}
		if string(data[rec:rec+4]) != tag {
			continue
		}
		off := int(binary.BigEndian.Uint32(data[rec+8 : rec+12]))
		length := uint64(binary.BigEndian.Uint32(data[rec+12 : rec+16]))
		if !fits(data, off, length) {
			return 0, false
		}
		return off, true
	}
	return 0, false
}

// fits reports whether the range [off, off+size) is inside data.
func fits(data []byte, off int, size uint64) bool {
	if off < 0 {
		return false
	}
	return uint64(off)+size <= uint64(len(data))
}
