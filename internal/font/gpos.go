package font

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/vyachin/go-pdf/internal/budget"
)

// kerning holds horizontal pair adjustments from the GPOS table. Adjustments
// are kept per lookup: every lookup of the feature applies in order and their
// values add up, while the subtables inside one lookup are alternatives and
// only the first matching one is used. Keeping a single subtable (as an earlier
// version did) dropped every pair of the first lookup as soon as a later lookup
// with an empty class table followed it.
type kerning struct {
	lookups []*pairLookup
	// coverGlyphs counts the glyph ids expanded from every Coverage of the
	// table, so repeated lookups and subtables share one budget.
	coverGlyphs int
	// scannedRanges counts the coverage entries examined across the whole
	// table, so a corrupt font cannot make one file buy unbounded work.
	scannedRanges int
	// declaredPairs counts the explicit pairs the format 1 subtables of the
	// whole table declare, so repeated PairSet offsets cannot expand one file
	// into millions of map entries.
	declaredPairs int
	// markGlyphs holds the glyphs GDEF classes as marks, so IgnoreMarks and a
	// mark filtering set can skip them. Nil when the font has no GDEF.
	markGlyphs map[uint16]bool
	// markSets holds the GDEF mark filtering sets, indexed by the lookup's
	// markFilteringSet. Nil when the font declares none.
	markSets [][]uint16
	// flagIgnoreMarks and flagFilter are the flags of the lookup being parsed,
	// copied into each pairLookup and markLookup as it is built.
	flagIgnoreMarks bool
	flagFilter      map[uint16]bool
	// guard bounds the work and carries cancellation. Nil means no limit.
	guard *budget.Guard
}

// Parsing bounds that are independent of the counts a font declares. Real
// fonts are far below them: DejaVu Sans has two lookups.
const (
	maxGPOSLookups   = 4096
	maxGPOSSubtables = 4096
	maxScannedRanges = 1_000_000
)

// GPOS lookup flags the engine honours.
const (
	flagIgnoreMarks         = 0x0008
	flagUseMarkFilteringSet = 0x0010
)

// maxKerningPairs bounds the explicit pairs a font may declare. A PairSet costs
// four bytes per pair, but several PairSet offsets may point at the same data,
// so the file size alone bounds nothing: a 264 KB font can otherwise declare
// 16,380 covered glyphs times 65,535 pairs. Real fonts are far below the cap;
// DejaVu Sans declares a few thousand.
const maxKerningPairs = 200_000

// limitf reports a font that crossed one of the parsing bounds above.
func limitf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrLimit, fmt.Sprintf(format, args...))
}

// charge records parsing work. A nil guard is unlimited work.
func (k *kerning) charge(n uint64) error {
	if k.guard == nil {
		return nil
	}
	if err := k.guard.Add(n); err != nil {
		if errors.Is(err, budget.ErrExceeded) {
			return limitf("GPOS parsing exceeded the work budget")
		}
		return err
	}
	return nil
}

// check reports a cancelled conversion promptly between lookups.
func (k *kerning) check() error {
	if k.guard == nil {
		return nil
	}
	if err := k.guard.Err(); err != nil {
		if errors.Is(err, budget.ErrExceeded) {
			return limitf("GPOS parsing exceeded the work budget")
		}
		return err
	}
	return nil
}

// pairLookup holds the positioning data of one PairPos lookup.
type pairLookup struct {
	// pairs are the explicit pairs of PairPos format 1 subtables.
	pairs map[uint32]int16
	// classes are the class-based PairPos format 2 subtables in file order.
	classes []*classSubtable
	// ignoreMarks is the lookup's IgnoreMarks flag: a mark glyph does not
	// participate in the pair.
	ignoreMarks bool
	// filter is the lookup's mark filtering set: a mark outside it does not
	// participate in the pair.
	filter map[uint16]bool
}

// classSubtable is one class-based (format 2) PairPos subtable.
type classSubtable struct {
	class1 map[uint16]uint16
	// covered is the first-glyph Coverage: glyphs outside it must not use the
	// class records.
	covered map[uint16]bool
	records [][]int16
	class2  []classRange
}

// maxKerningGlyphs bounds the glyph ids a corrupt GPOS table may expand into
// slices and maps. A range pair costs six bytes in the file but may cover the
// whole glyph space, so the count must be checked before the allocation.
const maxKerningGlyphs = 200000

type classRange struct {
	start, end uint16
	class      uint16
}

func (f *Font) parseGPOS() error {
	gpos, ok := f.tables["GPOS"]
	if !ok || len(gpos) < 10 {
		return nil
	}
	lookupList := int(binary.BigEndian.Uint16(gpos[8:10]))
	if lookupList == 0 || !fits(lookupList, 0, 2, len(gpos)) {
		return nil
	}
	k := &kerning{guard: f.guard}
	k.markGlyphs, k.markSets = parseGDEF(f.tables["GDEF"])
	// A lookup that belongs to another script or feature must not apply to
	// ordinary text: only the default script's kern, mark and mkmk features
	// are enabled. A font without a usable ScriptList keeps every lookup, as
	// the engine did before the selection existed.
	enabled := enabledLookups(gpos)
	count := int(binary.BigEndian.Uint16(gpos[lookupList : lookupList+2]))
	if count > maxGPOSLookups {
		return limitf("GPOS declares %d lookups, the limit is %d", count, maxGPOSLookups)
	}
	for i := 0; i < count; i++ {
		if enabled != nil && !enabled[i] {
			continue
		}
		if err := k.check(); err != nil {
			return err
		}
		if err := k.charge(1); err != nil {
			return err
		}
		offPos := lookupList + 2 + i*2
		if !fits(offPos, 0, 2, len(gpos)) {
			break
		}
		off := lookupList + int(binary.BigEndian.Uint16(gpos[offPos:offPos+2]))
		if !fits(off, 0, 6, len(gpos)) {
			continue
		}
		lookupType := binary.BigEndian.Uint16(gpos[off : off+2])
		if lookupType != 2 && lookupType != 4 && lookupType != 5 && lookupType != 6 && lookupType != 9 {
			continue
		}
		lookupFlags := binary.BigEndian.Uint16(gpos[off+2 : off+4])
		k.flagIgnoreMarks = lookupFlags&flagIgnoreMarks != 0
		k.flagFilter = nil
		if lookupFlags&flagUseMarkFilteringSet != 0 {
			idxPos := off + 6 + int(binary.BigEndian.Uint16(gpos[off+4:off+6]))*2
			if fits(idxPos, 0, 2, len(gpos)) {
				idx := int(binary.BigEndian.Uint16(gpos[idxPos : idxPos+2]))
				if idx < len(k.markSets) {
					k.flagFilter = make(map[uint16]bool, len(k.markSets[idx]))
					for _, gid := range k.markSets[idx] {
						k.flagFilter[gid] = true
					}
				}
			}
		}
		subCount := int(binary.BigEndian.Uint16(gpos[off+4 : off+6]))
		if subCount > maxGPOSSubtables {
			return limitf("GPOS lookup %d declares %d subtables, the limit is %d", i, subCount, maxGPOSSubtables)
		}
		lookup := &pairLookup{pairs: map[uint32]int16{}, ignoreMarks: k.flagIgnoreMarks, filter: k.flagFilter}
		for j := 0; j < subCount; j++ {
			if err := k.charge(1); err != nil {
				return err
			}
			subOffPos := off + 6 + j*2
			if !fits(subOffPos, 0, 2, len(gpos)) {
				break
			}
			subOff := off + int(binary.BigEndian.Uint16(gpos[subOffPos:subOffPos+2]))
			subType := int(lookupType)
			if lookupType == 9 {
				// Extension positioning wraps the real subtable behind a
				// 32-bit offset. Several system fonts (SFNS, SFCompact, Kefa)
				// store every PairPos subtable this way, and DejaVu stores its
				// mark lookups that way.
				var ok bool
				subOff, subType, ok = extensionSubtable(gpos, subOff)
				if !ok {
					continue
				}
			}
			switch subType {
			case 2:
				if err := lookup.parsePairPos(k, gpos, subOff); err != nil {
					return err
				}
			case 4, 5, 6:
				mark, err := k.parseMarkLookup(gpos, subOff, subType)
				if err != nil {
					return err
				}
				if mark == nil {
					continue
				}
				mark.ignoreMarks = k.flagIgnoreMarks
				mark.filter = k.flagFilter
				if f.marks == nil {
					f.marks = &markAttachment{}
				}
				f.marks.lookups = append(f.marks.lookups, mark)
			}
		}
		if len(lookup.pairs) > 0 || len(lookup.classes) > 0 {
			k.lookups = append(k.lookups, lookup)
		}
	}
	if len(k.lookups) > 0 {
		f.gpos = k
	}
	return nil
}

// extensionSubtable resolves an ExtensionPos (lookup type 9) subtable to the
// real subtable it wraps and returns its type. Any positioning type can be
// wrapped; the caller ignores the ones it does not implement.
func extensionSubtable(data []byte, off int) (int, int, bool) {
	if off < 0 || off+8 > len(data) {
		return 0, 0, false
	}
	format := binary.BigEndian.Uint16(data[off : off+2])
	lookupType := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
	if format != 1 || lookupType == 0 {
		return 0, 0, false
	}
	delta, ok := readU32(data, off+4)
	if !ok || delta == 0 || delta > uint64(len(data))-uint64(off) {
		return 0, 0, false
	}
	target := uint64(off) + delta
	if target+8 > uint64(len(data)) {
		return 0, 0, false
	}
	return int(target), lookupType, true
}

// parsePairPos parses one subtable into a lookup of its own. It is the entry
// point for tests of a single subtable; the table parser groups the subtables
// of a lookup itself.
func (k *kerning) parsePairPos(data []byte, off int) error {
	lookup := &pairLookup{pairs: map[uint32]int16{}}
	if err := lookup.parsePairPos(k, data, off); err != nil {
		return err
	}
	if len(lookup.pairs) > 0 || len(lookup.classes) > 0 {
		k.lookups = append(k.lookups, lookup)
	}
	return nil
}

func (l *pairLookup) parsePairPos(k *kerning, data []byte, off int) error {
	if !fits(off, 0, 10, len(data)) {
		return nil
	}
	format := binary.BigEndian.Uint16(data[off : off+2])
	coverageOff := off + int(binary.BigEndian.Uint16(data[off+2:off+4]))
	valueFormat1 := binary.BigEndian.Uint16(data[off+4 : off+6])
	valueFormat2 := binary.BigEndian.Uint16(data[off+6 : off+8])
	remaining := maxKerningGlyphs - k.coverGlyphs
	if remaining <= 0 {
		return nil
	}
	cover, ok, err := k.parseCoverage(data, coverageOff, remaining)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	k.coverGlyphs += len(cover)
	switch format {
	case 1:
		pairSetCount := int(binary.BigEndian.Uint16(data[off+8 : off+10]))
		recordSize := valueRecordSize(valueFormat1) + valueRecordSize(valueFormat2)
		// Count the declared pairs before building the map: PairSet offsets
		// are independent of the pair data, so a small file can point every
		// covered glyph at one huge PairSet and expand it thousands of times.
		declared := 0
		for i := 0; i < pairSetCount && i < len(cover); i++ {
			setPos := off + 10 + i*2
			if !fits(setPos, 0, 2, len(data)) {
				return nil
			}
			setOff := off + int(binary.BigEndian.Uint16(data[setPos:setPos+2]))
			if !fits(setOff, 0, 2, len(data)) {
				continue
			}
			n := int(binary.BigEndian.Uint16(data[setOff : setOff+2]))
			// One unit per pair, charged before the loop that expands them.
			if err := k.charge(uint64(n) + 1); err != nil {
				return err
			}
			if k.declaredPairs > maxKerningPairs-n || declared > maxKerningPairs-k.declaredPairs-n {
				return limitf("GPOS declares more than %d pairs", maxKerningPairs)
			}
			declared += n
		}
		k.declaredPairs += declared
		for i := 0; i < pairSetCount && i < len(cover); i++ {
			setPos := off + 10 + i*2
			if !fits(setPos, 0, 2, len(data)) {
				return nil
			}
			setOff := off + int(binary.BigEndian.Uint16(data[setPos:setPos+2]))
			if !fits(setOff, 0, 2, len(data)) {
				continue
			}
			n := int(binary.BigEndian.Uint16(data[setOff : setOff+2]))
			cursor := setOff + 2
			for j := 0; j < n; j++ {
				if !fits(cursor, 0, uint64(2+recordSize), len(data)) {
					return nil
				}
				second := binary.BigEndian.Uint16(data[cursor : cursor+2])
				cursor += 2
				adv, _ := xAdvance(data[cursor:cursor+valueRecordSize(valueFormat1)], valueFormat1)
				cursor += recordSize
				if adv != 0 {
					l.pairs[uint32(cover[i])<<16|uint32(second)] = adv
				}
			}
		}
	case 2:
		if !fits(off, 0, 16, len(data)) {
			return nil
		}
		classDef1Off := off + int(binary.BigEndian.Uint16(data[off+8:off+10]))
		classDef2Off := off + int(binary.BigEndian.Uint16(data[off+10:off+12]))
		class1Count := int(binary.BigEndian.Uint16(data[off+12 : off+14]))
		class2Count := int(binary.BigEndian.Uint16(data[off+14 : off+16]))
		classes1, ok := parseClassDef(data, classDef1Off)
		if !ok {
			return nil
		}
		sub := &classSubtable{class1: map[uint16]uint16{}, covered: make(map[uint16]bool, len(cover))}
		if c2, ok := parseClassDef(data, classDef2Off); ok {
			sub.class2 = c2
		}
		recordSize := valueRecordSize(valueFormat1) + valueRecordSize(valueFormat2)
		if recordSize == 0 {
			return nil
		}
		// Three font-declared counts multiply here; the product is compared in
		// 64-bit so a 32-bit build cannot wrap it into a negative bound.
		if !fits(off+16, 0, uint64(class1Count)*uint64(class2Count)*uint64(recordSize), len(data)) {
			return nil
		}
		// A corrupt table can describe ranges covering the whole glyph space;
		// expanding them into a map is bounded instead.
		expanded := 0
		for _, r := range classes1 {
			if r.end < r.start {
				return nil
			}
			expanded += int(r.end-r.start) + 1
		}
		if expanded > maxKerningGlyphs {
			return nil
		}
		sub.records = make([][]int16, class1Count)
		for _, gid := range cover {
			sub.covered[gid] = true
		}
		for _, r := range classes1 {
			for g := uint32(r.start); g <= uint32(r.end); g++ {
				sub.class1[uint16(g)] = r.class
			}
		}
		cursor := off + 16
		for c1 := 0; c1 < class1Count; c1++ {
			row := make([]int16, class2Count)
			for c2 := 0; c2 < class2Count; c2++ {
				if !fits(cursor, 0, uint64(recordSize), len(data)) {
					return nil
				}
				adv, _ := xAdvance(data[cursor:cursor+valueRecordSize(valueFormat1)], valueFormat1)
				row[c2] = adv
				cursor += recordSize
			}
			sub.records[c1] = row
		}
		l.classes = append(l.classes, sub)
	}
	return nil
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
// Every examined range is charged to the work guard, including a range that
// fails its bounds check.
func (k *kerning) parseCoverage(data []byte, off, limit int) ([]uint16, bool, error) {
	if !fits(off, 0, 4, len(data)) || limit <= 0 {
		return nil, false, nil
	}
	if err := k.charge(1); err != nil {
		return nil, false, err
	}
	format := binary.BigEndian.Uint16(data[off : off+2])
	switch format {
	case 1:
		n := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		if n > limit || !fits(off, 4, uint64(n)*2, len(data)) {
			return nil, false, nil
		}
		if err := k.charge(uint64(n)); err != nil {
			return nil, false, err
		}
		k.scannedRanges += n
		if k.scannedRanges > maxScannedRanges {
			return nil, false, limitf("a font scans more than %d coverage entries", maxScannedRanges)
		}
		out := make([]uint16, n)
		for i := 0; i < n; i++ {
			out[i] = binary.BigEndian.Uint16(data[off+4+i*2 : off+6+i*2])
			// The format requires strictly increasing glyph ids.
			if i > 0 && out[i] <= out[i-1] {
				return nil, false, nil
			}
		}
		return out, true, nil
	case 2:
		n := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		if !fits(off, 4, uint64(n)*6, len(data)) {
			return nil, false, nil
		}
		// Count the expansion first: a repeated 0..65535 range would otherwise
		// append 65536 entries per six bytes.
		total := 0
		prevEnd := -1
		for i := 0; i < n; i++ {
			if err := k.charge(1); err != nil {
				return nil, false, err
			}
			k.scannedRanges++
			if k.scannedRanges > maxScannedRanges {
				return nil, false, limitf("a font scans more than %d coverage ranges", maxScannedRanges)
			}
			base := off + 4 + i*6
			start := int(binary.BigEndian.Uint16(data[base : base+2]))
			end := int(binary.BigEndian.Uint16(data[base+2 : base+4]))
			if end < start || start <= prevEnd {
				return nil, false, nil
			}
			total += end - start + 1
			if total > limit {
				return nil, false, nil
			}
			prevEnd = end
		}
		// Charge the expansion, not only the ranges: two ranges can describe
		// the whole glyph space while costing twelve bytes.
		if err := k.charge(uint64(total) + 1); err != nil {
			return nil, false, err
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
		return out, true, nil
	}
	return nil, false, nil
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
// All lookups of the feature apply, so their adjustments add up.
func (f *Font) Kerning(left, right uint16) int32 {
	if f.gpos == nil {
		return 0
	}
	var total int32
	for _, lookup := range f.gpos.lookups {
		if !f.gpos.participates(lookup, left) || !f.gpos.participates(lookup, right) {
			continue
		}
		if v, ok := lookup.adjust(left, right); ok {
			total += int32(v)
		}
	}
	return total
}

// adjust returns the adjustment of the first subtable that applies, which is
// the subtable this lookup contributes.
func (l *pairLookup) adjust(left, right uint16) (int16, bool) {
	if v, ok := l.pairs[uint32(left)<<16|uint32(right)]; ok {
		return v, true
	}
	for _, sub := range l.classes {
		if v, ok := sub.adjust(left, right); ok {
			return v, true
		}
	}
	return 0, false
}

func (s *classSubtable) adjust(left, right uint16) (int16, bool) {
	if s.covered != nil && !s.covered[left] {
		return 0, false
	}
	// A glyph absent from a ClassDef belongs to class 0, not to "no kerning".
	c1 := uint16(0)
	if v, ok := s.class1[left]; ok {
		c1 = v
	}
	if int(c1) >= len(s.records) {
		return 0, false
	}
	c2 := uint16(0)
	if v, ok := classOf(s.class2, right); ok {
		c2 = v
	}
	if int(c2) >= len(s.records[c1]) {
		return 0, false
	}
	return s.records[c1][c2], true
}

// participates reports whether a glyph takes part in a lookup whose flags
// mention marks: IgnoreMarks skips every mark, and a mark filtering set skips
// the marks outside it.
func (k *kerning) participates(l *pairLookup, gid uint16) bool {
	if !k.markGlyphs[gid] {
		return true
	}
	if l.ignoreMarks {
		return false
	}
	if l.filter != nil && !l.filter[gid] {
		return false
	}
	return true
}

// ordinaryScripts are the scripts whose text the profile lays out as ordinary
// text: Latin, Cyrillic, Greek and Hebrew, plus the default script. The engine
// does not segment a run by script, but a feature under another script (Arabic
// joining, Indic reordering) belongs to shaping it does not perform, so those
// lookups stay disabled.
var ordinaryScripts = []string{"DFLT", "latn", "cyrl", "grek", "hebr"}

// enabledLookups returns the lookup indices the ordinary scripts' kern, mark and
// mkmk features reference. It returns nil when the table has no usable
// ScriptList or names none of the ordinary scripts, which means "every lookup":
// a crafted or single-script table without a script list still applies.
func enabledLookups(gpos []byte) map[int]bool {
	scriptList := int(binary.BigEndian.Uint16(gpos[4:6]))
	featureList := int(binary.BigEndian.Uint16(gpos[6:8]))
	if scriptList == 0 || featureList == 0 || !fits(scriptList, 0, 2, len(gpos)) {
		return nil
	}
	scripts := scriptTables(gpos, scriptList)
	var offs []int
	for _, tag := range ordinaryScripts {
		if off, ok := scripts[tag]; ok {
			offs = append(offs, off)
		}
	}
	if len(offs) == 0 {
		return nil
	}
	out := map[int]bool{}
	for _, off := range offs {
		langSys := defaultLangSys(gpos, off)
		if langSys < 0 {
			continue
		}
		for _, index := range langSysFeatures(gpos, langSys) {
			for _, lookup := range featureLookups(gpos, featureList, index) {
				out[lookup] = true
			}
		}
	}
	return out
}

// scriptTables maps each script tag to the offset of its Script table.
func scriptTables(gpos []byte, scriptList int) map[string]int {
	out := map[string]int{}
	count := int(binary.BigEndian.Uint16(gpos[scriptList : scriptList+2]))
	for i := 0; i < count; i++ {
		rec := scriptList + 2 + i*6
		if !fits(rec, 0, 6, len(gpos)) {
			break
		}
		tag := string(gpos[rec : rec+4])
		off := scriptList + int(binary.BigEndian.Uint16(gpos[rec+4:rec+6]))
		if !fits(off, 0, 4, len(gpos)) {
			continue
		}
		out[tag] = off
	}
	return out
}

// defaultLangSys returns the offset of the DefaultLangSys of one script, or -1.
func defaultLangSys(gpos []byte, script int) int {
	off := int(binary.BigEndian.Uint16(gpos[script : script+2]))
	if off == 0 || !fits(script+off, 0, 6, len(gpos)) {
		return -1
	}
	return script + off
}

// langSysFeatures returns the feature indices of one LangSys, required first.
func langSysFeatures(gpos []byte, langSys int) []int {
	required := int(binary.BigEndian.Uint16(gpos[langSys+2 : langSys+4]))
	count := int(binary.BigEndian.Uint16(gpos[langSys+4 : langSys+6]))
	out := make([]int, 0, count+1)
	if required != 0xFFFF {
		out = append(out, required)
	}
	for i := 0; i < count; i++ {
		if !fits(langSys, uint64(6+i*2), 2, len(gpos)) {
			break
		}
		out = append(out, int(binary.BigEndian.Uint16(gpos[langSys+6+i*2:langSys+8+i*2])))
	}
	return out
}

// featureLookups returns the lookups of one feature record when the feature is
// kern, mark or mkmk.
func featureLookups(gpos []byte, featureList, index int) []int {
	count := int(binary.BigEndian.Uint16(gpos[featureList : featureList+2]))
	if index < 0 || index >= count {
		return nil
	}
	rec := featureList + 2 + index*6
	if !fits(rec, 0, 6, len(gpos)) {
		return nil
	}
	tag := string(gpos[rec : rec+4])
	switch tag {
	case "kern", "mark", "mkmk":
	default:
		return nil
	}
	off := featureList + int(binary.BigEndian.Uint16(gpos[rec+4:rec+6]))
	if !fits(off, 0, 4, len(gpos)) {
		return nil
	}
	n := int(binary.BigEndian.Uint16(gpos[off+2 : off+4]))
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if !fits(off, uint64(4+i*2), 2, len(gpos)) {
			break
		}
		out = append(out, int(binary.BigEndian.Uint16(gpos[off+4+i*2:off+6+i*2])))
	}
	return out
}

// parseGDEF reads the glyph class definition and the mark filtering sets of a
// font's GDEF table. A missing or malformed table returns nil maps: the engine
// then has no marks to skip, which is what a font without GDEF declares.
func parseGDEF(data []byte) (map[uint16]bool, [][]uint16) {
	if len(data) < 12 {
		return nil, nil
	}
	var marks map[uint16]bool
	if classOff := int(binary.BigEndian.Uint16(data[4:6])); classOff != 0 {
		if ranges, ok := parseClassDef(data, classOff); ok {
			for _, r := range ranges {
				if r.class != 3 {
					continue
				}
				if marks == nil {
					marks = map[uint16]bool{}
				}
				for g := uint32(r.start); g <= uint32(r.end); g++ {
					marks[uint16(g)] = true
				}
			}
		}
	}
	var sets [][]uint16
	if setOff := int(binary.BigEndian.Uint16(data[10:12])); setOff != 0 && fits(setOff, 0, 4, len(data)) {
		if binary.BigEndian.Uint16(data[setOff:setOff+2]) == 1 {
			n := int(binary.BigEndian.Uint16(data[setOff+2 : setOff+4]))
			sets = make([][]uint16, n)
			k := &kerning{}
			for i := 0; i < n; i++ {
				pos := setOff + 4 + i*4
				raw, ok := readU32(data, pos)
				if !ok {
					break
				}
				off := setOff + int(raw)
				cover, ok, err := k.parseCoverage(data, off, maxKerningGlyphs)
				if err != nil || !ok {
					continue
				}
				sets[i] = cover
			}
		}
	}
	return marks, sets
}
