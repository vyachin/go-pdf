// Package font parses TrueType fonts and produces the data needed to embed
// them in a PDF: glyph lookup, metrics, kerning, outlines and subsetting.
package font

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Font is a parsed TrueType font.
type Font struct {
	data       []byte
	tables     map[string][]byte
	unitsPerEm int32
	ascent     int16
	descent    int16
	numGlyphs  uint16
	loca       []uint32
	glyf       []byte
	advances   []uint16
	numHMetric uint16
	cmap       *cmapSubtable
	family     string
	subfamily  string
	gpos       *kerning
	bbox       [4]int16
}

func (f *Font) UnitsPerEm() int32 { return f.unitsPerEm }
func (f *Font) Ascent() int16     { return f.ascent }
func (f *Font) Descent() int16    { return f.descent }
func (f *Font) NumGlyphs() uint16 { return f.numGlyphs }
func (f *Font) Family() string    { return f.family }
func (f *Font) Subfamily() string { return f.subfamily }
func (f *Font) BBox() [4]int16    { return f.bbox }

// ErrLimit reports a font whose conversion exceeds one of the documented
// bounds. The engine turns it into a resource/limit diagnostic, so a dense
// outline stops the conversion instead of allocating without bound.
var ErrLimit = fmt.Errorf("font: conversion limit exceeded")

// Load reads a font from a file's bytes and returns it ready for layout. A
// TrueType font is parsed directly; an OpenType font with CFF outlines is
// converted to TrueType in memory first, so that the rest of the engine — the
// subsetter, the CIDs and the PDF writer — sees the one format it embeds.
//
// WOFF, WOFF2, CFF2, variable fonts and font collections are refused with a
// diagnostic naming the format: each needs a decoder the engine does not carry.
func Load(data []byte) (*Font, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("font: file is too short")
	}
	version := binary.BigEndian.Uint32(data[0:4])
	switch version {
	case 0x00010000, 0x74727565, 0x4F54544F: // 1.0, 'true', 'OTTO'
	case 0x774F4646: // 'wOFF'
		return nil, fmt.Errorf("font: WOFF web fonts are not supported, use the original TrueType or OpenType file")
	case 0x774F4632: // 'wOF2'
		return nil, fmt.Errorf("font: WOFF2 web fonts are not supported, use the original TrueType or OpenType file")
	case 0x74746366: // 'ttcf'
		return nil, fmt.Errorf("font: font collections are not supported, name a single font file")
	default:
		return nil, fmt.Errorf("font: unknown sfnt version %#x", version)
	}
	tables, err := tableDirectory(data)
	if err != nil {
		return nil, err
	}
	if _, ok := tables["fvar"]; ok {
		return nil, fmt.Errorf("font: variable fonts are not supported, use a static instance")
	}
	if _, ok := tables["CFF2"]; ok {
		return nil, fmt.Errorf("font: CFF2 outlines are not supported, use a CFF or TrueType font")
	}
	if version != 0x4F54544F {
		return Parse(data)
	}
	converted, err := convertCFF(data, tables)
	if err != nil {
		return nil, err
	}
	return Parse(converted)
}

// tableDirectory returns the tables of one sfnt, keyed by tag.
func tableDirectory(data []byte) (map[string][]byte, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("font: file is too short")
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+numTables*16 {
		return nil, fmt.Errorf("font: truncated table directory")
	}
	tables := make(map[string][]byte, numTables)
	for i := 0; i < numTables; i++ {
		rec := data[12+i*16 : 12+i*16+16]
		tag := string(rec[0:4])
		offset := int64(binary.BigEndian.Uint32(rec[8:12]))
		length := int64(binary.BigEndian.Uint32(rec[12:16]))
		if offset+length > int64(len(data)) {
			return nil, fmt.Errorf("font: table %q is out of bounds", tag)
		}
		tables[tag] = data[offset : offset+length]
	}
	return tables, nil
}

// Parse reads a TrueType font. CFF based OpenType fonts are rejected because
// their outlines need a separate reader; Load converts them first.
func Parse(data []byte) (*Font, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("font: file is too short")
	}
	version := binary.BigEndian.Uint32(data[0:4])
	switch version {
	case 0x00010000, 0x74727565: // 1.0, 'true'
	case 0x4F54544F: // 'OTTO'
		return nil, fmt.Errorf("font: CFF/OpenType outlines are not supported, use a TrueType font")
	default:
		return nil, fmt.Errorf("font: unknown sfnt version %#x", version)
	}
	tables, err := tableDirectory(data)
	if err != nil {
		return nil, err
	}
	f := &Font{data: data, tables: tables}

	head, ok := f.tables["head"]
	if !ok || len(head) < 54 {
		return nil, fmt.Errorf("font: missing head table")
	}
	f.unitsPerEm = int32(binary.BigEndian.Uint16(head[18:20]))
	if f.unitsPerEm == 0 {
		return nil, fmt.Errorf("font: unitsPerEm is zero")
	}
	indexToLoc := int16(binary.BigEndian.Uint16(head[50:52]))
	f.bbox = [4]int16{
		int16(binary.BigEndian.Uint16(head[36:38])),
		int16(binary.BigEndian.Uint16(head[38:40])),
		int16(binary.BigEndian.Uint16(head[40:42])),
		int16(binary.BigEndian.Uint16(head[42:44])),
	}

	hhea, ok := f.tables["hhea"]
	if !ok || len(hhea) < 36 {
		return nil, fmt.Errorf("font: missing hhea table")
	}
	f.ascent = int16(binary.BigEndian.Uint16(hhea[4:6]))
	f.descent = int16(binary.BigEndian.Uint16(hhea[6:8]))
	f.numHMetric = binary.BigEndian.Uint16(hhea[34:36])

	maxp, ok := f.tables["maxp"]
	if !ok || len(maxp) < 6 {
		return nil, fmt.Errorf("font: missing maxp table")
	}
	f.numGlyphs = binary.BigEndian.Uint16(maxp[4:6])

	glyf, ok := f.tables["glyf"]
	if !ok {
		// The profile supports TrueType outlines only. A font without glyf
		// used to parse and then silently produce glyphs with no contours.
		return nil, fmt.Errorf("font: missing glyf table")
	}
	f.glyf = glyf
	loca, ok := f.tables["loca"]
	if !ok {
		return nil, fmt.Errorf("font: missing loca table")
	}
	count := int(f.numGlyphs) + 1
	if indexToLoc == 0 {
		if len(loca) < count*2 {
			return nil, fmt.Errorf("font: loca table is truncated")
		}
		f.loca = make([]uint32, count)
		for i := 0; i < count; i++ {
			f.loca[i] = uint32(binary.BigEndian.Uint16(loca[i*2:i*2+2])) * 2
		}
	} else {
		if len(loca) < count*4 {
			return nil, fmt.Errorf("font: loca table is truncated")
		}
		f.loca = make([]uint32, count)
		for i := 0; i < count; i++ {
			f.loca[i] = binary.BigEndian.Uint32(loca[i*4 : i*4+4])
		}
	}

	// loca must be monotonic and stay inside glyf: every glyph. A corrupt
	// table must be rejected instead of yielding an empty subset.
	prev := uint32(0)
	for i, off := range f.loca {
		if off < prev {
			return nil, fmt.Errorf("font: loca offsets are not monotonic at glyph %d", i)
		}
		if int64(off) > int64(len(f.glyf)) {
			return nil, fmt.Errorf("font: loca offset %d for glyph %d is outside the glyf table", off, i)
		}
		prev = off
	}

	if err := f.parseHmtx(); err != nil {
		return nil, err
	}
	// cmap is optional: a subset embedded in a PDF is addressed by glyph id and
	// needs no character mapping.
	if _, ok := f.tables["cmap"]; ok {
		if err := f.parseCmap(); err != nil {
			return nil, err
		}
	}
	f.parseName()
	f.parseGPOS()
	return f, nil
}

func (f *Font) parseHmtx() error {
	hmtx, ok := f.tables["hmtx"]
	if !ok {
		return fmt.Errorf("font: missing hmtx table")
	}
	n := int(f.numHMetric)
	if n == 0 {
		return fmt.Errorf("font: numberOfHMetrics is zero")
	}
	if len(hmtx) < n*4 {
		return fmt.Errorf("font: hmtx table is truncated")
	}
	f.advances = make([]uint16, f.numGlyphs)
	last := uint16(0)
	for i := 0; i < int(f.numGlyphs); i++ {
		if i < n {
			last = binary.BigEndian.Uint16(hmtx[i*4 : i*4+2])
		}
		f.advances[i] = last
	}
	return nil
}

// Advance returns the advance width of a glyph in font units.
func (f *Font) Advance(gid uint16) int32 {
	if int(gid) >= len(f.advances) {
		return 0
	}
	return int32(f.advances[gid])
}

// GlyphIndex maps a rune to a glyph index.
func (f *Font) GlyphIndex(r rune) (uint16, bool) {
	if f.cmap == nil {
		return 0, false
	}
	gid, ok := f.cmap.lookup(r)
	if !ok || gid == 0 || !f.HasGlyph(gid) {
		// Glyph 0 is .notdef and an id outside numGlyphs cannot be drawn; both
		// mean the character has no glyph.
		return 0, false
	}
	return gid, true
}

// HasGlyph reports whether the glyph index exists in the font.
func (f *Font) HasGlyph(gid uint16) bool { return gid < f.numGlyphs }

func (f *Font) glyphData(gid uint16) []byte {
	if int(gid)+1 >= len(f.loca) {
		return nil
	}
	start, end := f.loca[gid], f.loca[gid+1]
	if end <= start || int(end) > len(f.glyf) {
		return nil
	}
	return f.glyf[start:end]
}

func (f *Font) parseName() {
	name, ok := f.tables["name"]
	if !ok || len(name) < 6 {
		return
	}
	count := int(binary.BigEndian.Uint16(name[2:4]))
	stringOffset := int(binary.BigEndian.Uint16(name[4:6]))
	for i := 0; i < count; i++ {
		rec := 6 + i*12
		if rec+12 > len(name) {
			return
		}
		platform := binary.BigEndian.Uint16(name[rec : rec+2])
		nameID := binary.BigEndian.Uint16(name[rec+6 : rec+8])
		length := int(binary.BigEndian.Uint16(name[rec+8 : rec+10]))
		offset := int(binary.BigEndian.Uint16(name[rec+10 : rec+12]))
		start := stringOffset + offset
		if start < 0 || start+length > len(name) {
			continue
		}
		raw := name[start : start+length]
		if platform != 3 && platform != 0 && platform != 1 {
			continue
		}
		var s string
		if platform == 3 || platform == 0 {
			// UTF-16BE
			if length%2 != 0 {
				continue
			}
			runes := make([]rune, 0, length/2)
			for j := 0; j+1 < len(raw); j += 2 {
				runes = append(runes, rune(binary.BigEndian.Uint16(raw[j:j+2])))
			}
			s = string(runes)
		} else {
			s = string(raw)
		}
		s = strings.TrimSpace(s)
		switch nameID {
		case 1:
			if f.family == "" {
				f.family = s
			}
		case 2:
			if f.subfamily == "" {
				f.subfamily = s
			}
		}
	}
}

// cmapSubtable is a supported character to glyph mapping.
type cmapSubtable struct {
	format uint16
	// format 4
	endCodes   []uint16
	startCodes []uint16
	idDeltas   []int16
	idRangeOff []uint16
	segCount   int
	// format 12
	groups     []cmapGroup
	tableStart int
	table      []byte
}

type cmapGroup struct {
	startChar uint32
	endChar   uint32
	startGID  uint32
}

func (f *Font) parseCmap() error {
	cmap, ok := f.tables["cmap"]
	if !ok || len(cmap) < 4 {
		return fmt.Errorf("font: missing cmap table")
	}
	numTables := int(binary.BigEndian.Uint16(cmap[2:4]))
	type candidate struct {
		score int
		off   int
	}
	best := candidate{score: -1}
	for i := 0; i < numTables; i++ {
		rec := 4 + i*8
		if rec+8 > len(cmap) {
			break
		}
		platform := binary.BigEndian.Uint16(cmap[rec : rec+2])
		encoding := binary.BigEndian.Uint16(cmap[rec+2 : rec+4])
		offset := int(binary.BigEndian.Uint32(cmap[rec+4 : rec+8]))
		if offset+2 > len(cmap) {
			continue
		}
		format := binary.BigEndian.Uint16(cmap[offset : offset+2])
		score := -1
		switch {
		case format == 12 && platform == 3 && encoding == 10:
			score = 100
		case format == 12 && platform == 0:
			score = 90
		case format == 4 && platform == 3 && encoding == 1:
			score = 80
		case format == 4 && platform == 0:
			score = 70
		}
		if score > best.score {
			best = candidate{score: score, off: offset}
		}
	}
	if best.score < 0 {
		return fmt.Errorf("font: no supported cmap subtable (formats 4 and 12 are supported)")
	}
	sub, err := parseCmapSubtable(cmap, best.off)
	if err != nil {
		return err
	}
	f.cmap = sub
	return nil
}

func parseCmapSubtable(cmap []byte, off int) (*cmapSubtable, error) {
	format := binary.BigEndian.Uint16(cmap[off : off+2])
	switch format {
	case 4:
		if off+14 > len(cmap) {
			return nil, fmt.Errorf("font: truncated cmap format 4")
		}
		segCount := int(binary.BigEndian.Uint16(cmap[off+6:off+8])) / 2
		sub := &cmapSubtable{format: 4, segCount: segCount, table: cmap, tableStart: off}
		endBase := off + 14
		startBase := endBase + segCount*2 + 2
		deltaBase := startBase + segCount*2
		rangeBase := deltaBase + segCount*2
		if rangeBase+segCount*2 > len(cmap) {
			return nil, fmt.Errorf("font: truncated cmap format 4 segments")
		}
		sub.endCodes = make([]uint16, segCount)
		sub.startCodes = make([]uint16, segCount)
		sub.idDeltas = make([]int16, segCount)
		sub.idRangeOff = make([]uint16, segCount)
		for i := 0; i < segCount; i++ {
			sub.endCodes[i] = binary.BigEndian.Uint16(cmap[endBase+i*2 : endBase+i*2+2])
			sub.startCodes[i] = binary.BigEndian.Uint16(cmap[startBase+i*2 : startBase+i*2+2])
			sub.idDeltas[i] = int16(binary.BigEndian.Uint16(cmap[deltaBase+i*2 : deltaBase+i*2+2]))
			sub.idRangeOff[i] = binary.BigEndian.Uint16(cmap[rangeBase+i*2 : rangeBase+i*2+2])
			if sub.startCodes[i] > sub.endCodes[i] || (i > 0 && sub.startCodes[i] <= sub.endCodes[i-1]) {
				return nil, fmt.Errorf("font: invalid cmap format 4 segment %d", i)
			}
		}
		return sub, nil
	case 12:
		if off+16 > len(cmap) {
			return nil, fmt.Errorf("font: truncated cmap format 12")
		}
		nGroups := int(binary.BigEndian.Uint32(cmap[off+12 : off+16]))
		if off+16+nGroups*12 > len(cmap) {
			return nil, fmt.Errorf("font: truncated cmap format 12 groups")
		}
		sub := &cmapSubtable{format: 12, table: cmap, tableStart: off}
		sub.groups = make([]cmapGroup, nGroups)
		prevEnd := uint32(0)
		for i := 0; i < nGroups; i++ {
			base := off + 16 + i*12
			g := cmapGroup{
				startChar: binary.BigEndian.Uint32(cmap[base : base+4]),
				endChar:   binary.BigEndian.Uint32(cmap[base+4 : base+8]),
				startGID:  binary.BigEndian.Uint32(cmap[base+8 : base+12]),
			}
			if g.endChar < g.startChar || (i > 0 && g.startChar <= prevEnd) {
				return nil, fmt.Errorf("font: invalid cmap format 12 group %d", i)
			}
			if g.startGID+uint32(g.endChar-g.startChar) > 0xFFFF {
				return nil, fmt.Errorf("font: cmap format 12 group %d maps past the glyph id space", i)
			}
			sub.groups[i] = g
			prevEnd = g.endChar
		}
		return sub, nil
	default:
		return nil, fmt.Errorf("font: unsupported cmap format %d", format)
	}
}

func (s *cmapSubtable) lookup(r rune) (uint16, bool) {
	if r < 0 || r > 0x10FFFF {
		return 0, false
	}
	c := uint32(r)
	if s.format == 12 {
		lo, hi := 0, len(s.groups)-1
		for lo <= hi {
			mid := (lo + hi) / 2
			g := s.groups[mid]
			switch {
			case c < g.startChar:
				hi = mid - 1
			case c > g.endChar:
				lo = mid + 1
			default:
				gid := g.startGID + (c - g.startChar)
				if gid == 0 || gid > 0xFFFF {
					return 0, false
				}
				return uint16(gid), true
			}
		}
		return 0, false
	}
	// format 4: binary search over segments.
	lo, hi := 0, s.segCount-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case c > uint32(s.endCodes[mid]):
			lo = mid + 1
		case c < uint32(s.startCodes[mid]):
			hi = mid - 1
		default:
			if s.idRangeOff[mid] == 0 {
				gid := uint32(int32(c) + int32(s.idDeltas[mid]))
				return uint16(gid & 0xFFFF), gid != 0
			}
			addr := s.tableStart + 14 + s.segCount*2 + 2 + s.segCount*2 + s.segCount*2 + mid*2
			addr += int(s.idRangeOff[mid])
			addr += int(c-uint32(s.startCodes[mid])) * 2
			if addr+2 > len(s.table) || addr+2 > s.tableStart+int(binary.BigEndian.Uint16(s.table[s.tableStart+2:s.tableStart+4])) {
				return 0, false
			}
			gid := binary.BigEndian.Uint16(s.table[addr : addr+2])
			if gid == 0 {
				return 0, false
			}
			return uint16((int(gid) + int(s.idDeltas[mid])) & 0xFFFF), true
		}
	}
	return 0, false
}
