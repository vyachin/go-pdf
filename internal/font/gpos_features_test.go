package font

import (
	"encoding/binary"
	"testing"
)

// gposFeature is one feature record: a tag and the lookups it references.
type gposFeature struct {
	tag     string
	lookups []int
}

// gposScript is one script record: a tag and its default LangSys features.
type gposScript struct {
	tag      string
	features []int
}

func appendU16(b []byte, v uint16) []byte { return append(b, byte(v>>8), byte(v)) }

// buildGPOS assembles a GPOS table from scripts, features and lookups.
func buildGPOS(scripts []gposScript, features []gposFeature, lookups [][]byte) []byte {
	lookupList := make([]byte, 2)
	binary.BigEndian.PutUint16(lookupList, uint16(len(lookups)))
	lookupList = append(lookupList, make([]byte, 2*len(lookups))...)
	for i, lookup := range lookups {
		binary.BigEndian.PutUint16(lookupList[2+2*i:], uint16(len(lookupList)))
		lookupList = append(lookupList, lookup...)
	}

	featureList := make([]byte, 2)
	binary.BigEndian.PutUint16(featureList, uint16(len(features)))
	featureList = append(featureList, make([]byte, 6*len(features))...)
	for i, f := range features {
		copy(featureList[2+6*i:2+6*i+4], f.tag)
		binary.BigEndian.PutUint16(featureList[2+6*i+4:], uint16(len(featureList)))
		table := make([]byte, 4)
		binary.BigEndian.PutUint16(table[2:], uint16(len(f.lookups)))
		for _, l := range f.lookups {
			table = appendU16(table, uint16(l))
		}
		featureList = append(featureList, table...)
	}

	scriptList := make([]byte, 2)
	binary.BigEndian.PutUint16(scriptList, uint16(len(scripts)))
	scriptList = append(scriptList, make([]byte, 6*len(scripts))...)
	for i, sc := range scripts {
		copy(scriptList[2+6*i:2+6*i+4], sc.tag)
		binary.BigEndian.PutUint16(scriptList[2+6*i+4:], uint16(len(scriptList)))
		table := make([]byte, 2)
		binary.BigEndian.PutUint16(table, 4) // DefaultLangSys follows
		table = appendU16(table, 0)          // langSysCount
		table = appendU16(table, 0)          // lookupOrder
		table = appendU16(table, 0xFFFF)     // requiredFeatureIndex
		table = appendU16(table, uint16(len(sc.features)))
		for _, f := range sc.features {
			table = appendU16(table, uint16(f))
		}
		scriptList = append(scriptList, table...)
	}

	header := make([]byte, 10)
	binary.BigEndian.PutUint32(header[0:4], 0x00010000)
	out := header
	binary.BigEndian.PutUint16(out[4:6], uint16(len(out)))
	out = append(out, scriptList...)
	binary.BigEndian.PutUint16(out[6:8], uint16(len(out)))
	out = append(out, featureList...)
	binary.BigEndian.PutUint16(out[8:10], uint16(len(out)))
	out = append(out, lookupList...)
	return out
}

// pairLookupTable wraps one PairPos subtable in a type 2 lookup with flags.
func pairLookupTable(sub []byte, flags uint16, markSet uint16) []byte {
	headerLen := 6 + 2
	if flags&0x0010 != 0 {
		headerLen += 2
	}
	lookup := make([]byte, 6)
	binary.BigEndian.PutUint16(lookup[0:2], 2) // PairPos
	binary.BigEndian.PutUint16(lookup[2:4], flags)
	binary.BigEndian.PutUint16(lookup[4:6], 1) // one subtable
	lookup = appendU16(lookup, uint16(headerLen))
	if flags&0x0010 != 0 {
		lookup = appendU16(lookup, markSet)
	}
	return append(lookup, sub...)
}

// buildGDEF assembles a GDEF table with a glyph class definition and mark
// filtering sets.
func buildGDEF(markGlyphs []uint16, sets [][]uint16) []byte {
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	if len(markGlyphs) > 0 {
		classes := make([]uint16, len(markGlyphs))
		for i := range classes {
			classes[i] = 3
		}
		binary.BigEndian.PutUint16(out[4:6], uint16(len(out)))
		out = append(out, classDefFormat1Test(markGlyphs[0], classes)...)
	}
	if len(sets) > 0 {
		binary.BigEndian.PutUint16(out[10:12], uint16(len(out)))
		setsTable := make([]byte, 4)
		binary.BigEndian.PutUint16(setsTable[0:2], 1)
		binary.BigEndian.PutUint16(setsTable[2:4], uint16(len(sets)))
		setsTable = append(setsTable, make([]byte, 4*len(sets))...)
		for i, set := range sets {
			binary.BigEndian.PutUint32(setsTable[4+4*i:], uint32(len(setsTable)))
			setsTable = append(setsTable, coverageFormat1Test(set)...)
		}
		out = append(out, setsTable...)
	}
	return out
}

// parseGPOSTable parses one crafted GPOS (and GDEF) table into a font.
func parseGPOSTable(t *testing.T, gpos, gdef []byte) *Font {
	t.Helper()
	tables := map[string][]byte{"GPOS": gpos}
	if gdef != nil {
		tables["GDEF"] = gdef
	}
	f := &Font{tables: tables}
	if err := f.parseGPOS(); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestLookupSelectionUsesTheOrdinaryScripts guards RV4-42: a PairPos lookup
// declared only under a non-ordinary script must not apply, while the default
// script's kern lookup still does.
func TestLookupSelectionUsesTheOrdinaryScripts(t *testing.T) {
	gpos := buildGPOS(
		[]gposScript{{tag: "arab", features: []int{0}}, {tag: "DFLT", features: []int{1}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}, {tag: "kern", lookups: []int{1}}},
		[][]byte{
			pairLookupTable(pairPosFormat1Test(65, 86, -100), 0, 0),
			pairLookupTable(pairPosFormat1Test(65, 86, -200), 0, 0),
		},
	)
	f := parseGPOSTable(t, gpos, nil)
	if got := f.Kerning(65, 86); got != -200 {
		t.Fatalf("kerning = %d, want the DFLT lookup's -200", got)
	}
}

// TestLookupSelectionIgnoresOtherFeatures guards the feature half of RV4-42: a
// lookup referenced only by another feature (here a cursive-attachment feature)
// does not apply.
func TestLookupSelectionIgnoresOtherFeatures(t *testing.T) {
	gpos := buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0, 1}}},
		[]gposFeature{{tag: "curs", lookups: []int{0}}, {tag: "kern", lookups: []int{1}}},
		[][]byte{
			pairLookupTable(pairPosFormat1Test(65, 86, -100), 0, 0),
			pairLookupTable(pairPosFormat1Test(65, 86, -200), 0, 0),
		},
	)
	f := parseGPOSTable(t, gpos, nil)
	if got := f.Kerning(65, 86); got != -200 {
		t.Fatalf("kerning = %d, want only the kern feature's -200", got)
	}
}

// TestIgnoreMarksSkipsMarkGlyphs guards the lookupFlag half of RV4-42: a pair
// whose left glyph is a GDEF mark does not participate in a lookup that sets
// IgnoreMarks, while the same pair of non-marks still does.
func TestIgnoreMarksSkipsMarkGlyphs(t *testing.T) {
	const ignoreMarks = 0x0008
	gpos := buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0, 1}}},
		[][]byte{
			pairLookupTable(pairPosFormat1Test(86, 65, -100), ignoreMarks, 0),
			pairLookupTable(pairPosFormat1Test(66, 65, -100), ignoreMarks, 0),
		},
	)
	f := parseGPOSTable(t, gpos, buildGDEF([]uint16{86}, nil))
	if got := f.Kerning(86, 65); got != 0 {
		t.Fatalf("a mark glyph kerned by %d under IgnoreMarks", got)
	}
	if got := f.Kerning(66, 65); got != -100 {
		t.Fatalf("a non-mark glyph lost its kerning: %d", got)
	}
}

// TestMarkFilteringSetExcludesMarks guards the mark filtering set half: a mark
// outside the lookup's set does not participate, and a mark inside it does.
func TestMarkFilteringSetExcludesMarks(t *testing.T) {
	const useSet = 0x0010
	gpos := buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0, 1}}},
		[][]byte{
			pairLookupTable(pairPosFormat1Test(86, 65, -100), useSet, 0),
			pairLookupTable(pairPosFormat1Test(87, 65, -100), useSet, 0),
		},
	)
	// Glyphs 86 and 87 are marks; the set contains only 87.
	f := parseGPOSTable(t, gpos, buildGDEF([]uint16{86, 87}, [][]uint16{{87}}))
	if got := f.Kerning(86, 65); got != 0 {
		t.Fatalf("a mark outside the filtering set kerned by %d", got)
	}
	if got := f.Kerning(87, 65); got != -100 {
		t.Fatalf("a mark inside the filtering set lost its kerning: %d", got)
	}
}

// TestFeatureListPastTableDisablesLookups guards RV5-01: an out-of-bounds
// FeatureList header is an unusable feature list, not a read outside the table.
// It disables every lookup instead of panicking or enabling all of them.
func TestFeatureListPastTableDisablesLookups(t *testing.T) {
	gpos := buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}},
		[][]byte{pairLookupTable(pairPosFormat1Test(65, 86, -100), 0, 0)},
	)
	binary.BigEndian.PutUint16(gpos[6:8], 0xFFFF)
	got := enabledLookups(gpos)
	if got == nil || len(got) != 0 {
		t.Fatalf("enabledLookups = %v, want an empty non-nil set", got)
	}
	f := parseGPOSTable(t, gpos, nil)
	if kerning := f.Kerning(65, 86); kerning != 0 {
		t.Fatalf("kerning = %d, want 0", kerning)
	}
}

// TestFeatureLookupsRejectsHeaderPastTable guards the local check in
// featureLookups: the header read is bounds-checked before it slices.
func TestFeatureLookupsRejectsHeaderPastTable(t *testing.T) {
	gpos := buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}},
		[][]byte{pairLookupTable(pairPosFormat1Test(65, 86, -100), 0, 0)},
	)
	for _, off := range []int{len(gpos), 0xFFFF} {
		if got := featureLookups(gpos, off, 0); got != nil {
			t.Fatalf("featureLookups(%d) = %v, want nil", off, got)
		}
	}
}

// TestFeatureLookupsRejectsRecordPastTable covers a FeatureList header that fits
// but whose feature-record offset does not.
func TestFeatureLookupsRejectsRecordPastTable(t *testing.T) {
	gpos := buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}},
		[][]byte{pairLookupTable(pairPosFormat1Test(65, 86, -100), 0, 0)},
	)
	featureList := int(binary.BigEndian.Uint16(gpos[6:8]))
	binary.BigEndian.PutUint16(gpos[featureList+2+4:featureList+2+6], 0xFFFF)
	if got := featureLookups(gpos, featureList, 0); got != nil {
		t.Fatalf("featureLookups = %v, want nil", got)
	}
}
