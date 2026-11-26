package font

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// glyphFont builds the smallest complete TrueType font that carries the given
// glyph data. The loca offsets are not padded, so a test can describe a glyph
// whose range ends exactly where the test wants it to end.
func glyphFont(glyphs [][]byte) []byte {
	numGlyphs := len(glyphs)
	var glyf []byte
	loca := make([]uint32, numGlyphs+1)
	for i, data := range glyphs {
		loca[i] = uint32(len(glyf))
		glyf = append(glyf, data...)
	}
	loca[numGlyphs] = uint32(len(glyf))

	locaBytes := make([]byte, len(loca)*4)
	for i, v := range loca {
		binary.BigEndian.PutUint32(locaBytes[i*4:i*4+4], v)
	}
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm
	binary.BigEndian.PutUint16(head[50:52], 1)    // long loca offsets
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:6], 800) // ascent
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], uint16(numGlyphs))
	hmtx := []byte{0, 100, 0, 0}

	b := &fontBuilder{}
	b.add("head", head)
	b.add("hhea", hhea)
	b.add("maxp", maxp)
	b.add("hmtx", hmtx)
	b.add("loca", locaBytes)
	b.add("glyf", glyf)
	return b.bytes()
}

// compositeRecord writes one composite component record.
func compositeRecord(flags, component uint16, args int16) []byte {
	out := make([]byte, 0, 8)
	out = appendU16(out, flags)
	out = appendU16(out, component)
	if flags&0x0001 != 0 {
		out = appendU16(out, uint16(args))
		out = appendU16(out, 0)
	} else {
		out = append(out, byte(args), 0)
	}
	return out
}

// compositeGlyphData writes the header and the components of a composite glyph.
func compositeGlyphData(components ...[]byte) []byte {
	out := make([]byte, 10)
	binary.BigEndian.PutUint16(out[0:2], 0xFFFF) // composite
	for _, c := range components {
		out = append(out, c...)
	}
	return out
}

// TestOnlyForeignScriptEnablesNoLookups guards RV5-28: a usable ScriptList that
// names only another script selects nothing, so its pair never reaches Latin
// text, while the same pair under DFLT still does.
func TestOnlyForeignScriptEnablesNoLookups(t *testing.T) {
	foreign := buildGPOS(
		[]gposScript{{tag: "arab", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}},
		[][]byte{pairLookupTable(pairPosFormat1Test(65, 86, -300), 0, 0)},
	)
	if enabled := enabledLookups(foreign); enabled == nil || len(enabled) != 0 {
		t.Fatalf("an only-arab ScriptList selected %v, want an empty non-nil set", enabled)
	}
	f, err := Parse(tinyFontWithGPOS(foreign))
	if err != nil {
		t.Fatalf("a font with an only-arab ScriptList was rejected: %v", err)
	}
	if got := f.Kerning(65, 86); got != 0 {
		t.Fatalf("Latin AV kerned by %d from an arab-only script", got)
	}

	ordinary := buildGPOS(
		[]gposScript{{tag: "arab", features: []int{0}}, {tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}},
		[][]byte{pairLookupTable(pairPosFormat1Test(65, 86, -300), 0, 0)},
	)
	f, err = Parse(tinyFontWithGPOS(ordinary))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Kerning(65, 86); got != -300 {
		t.Fatalf("a DFLT kern pair returned %d, want -300", got)
	}
}

// TestNoUsableScriptListKeepsTheFallback guards the other half of RV5-28: a
// table whose ScriptList offset is missing or out of bounds still enables every
// lookup.
func TestNoUsableScriptListKeepsTheFallback(t *testing.T) {
	base := buildGPOS(
		[]gposScript{{tag: "arab", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}},
		[][]byte{pairLookupTable(pairPosFormat1Test(65, 86, -100), 0, 0)},
	)
	withScriptList := func(off uint16) []byte {
		out := append([]byte(nil), base...)
		binary.BigEndian.PutUint16(out[4:6], off)
		return out
	}
	cases := map[string][]byte{
		"missing":       withScriptList(0),
		"out of bounds": withScriptList(0xFFFF),
	}
	for name, gpos := range cases {
		t.Run(name, func(t *testing.T) {
			if enabled := enabledLookups(gpos); enabled != nil {
				t.Fatalf("enabledLookups = %v, want nil for a missing ScriptList", enabled)
			}
			f, err := Parse(tinyFontWithGPOS(gpos))
			if err != nil {
				t.Fatal(err)
			}
			if got := f.Kerning(65, 86); got != -100 {
				t.Fatalf("the fallback kerning = %d, want -100", got)
			}
		})
	}
}

// gposWithCoverageLookups builds a font whose GPOS declares n kern lookups,
// each covering the first 60,000 glyphs.
func gposWithCoverageLookups(n int) []byte {
	coverage := coverageFormat2Test([][2]uint16{{0, 59999}})
	subtable := pairPosFormat2Test(coverage)
	lookups := make([][]byte, n)
	indices := make([]int, n)
	for i := range lookups {
		lookups[i] = pairLookupTable(subtable, 0, 0)
		indices[i] = i
	}
	return buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: indices}},
		lookups,
	)
}

// TestCoverageCapIsResourceLimit guards RV5-29: four lookups of 60,000 glyphs
// exceed the shared 200,000-glyph cap, so the load fails with resource/limit
// instead of silently keeping three lookups.
func TestCoverageCapIsResourceLimit(t *testing.T) {
	data := tinyFontWithGPOS(gposWithCoverageLookups(4))
	_, err := Parse(data)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("a font over the coverage cap returned %v, want ErrLimit", err)
	}
	if !strings.Contains(err.Error(), "coverage") {
		t.Fatalf("the limit error does not name the coverage cap: %v", err)
	}
}

// TestCoverageWithinCapLoadsEveryLookup guards the other side of RV5-29: three
// lookups of 60,000 glyphs fit, and every one is enabled.
func TestCoverageWithinCapLoadsEveryLookup(t *testing.T) {
	f, err := Parse(tinyFontWithGPOS(gposWithCoverageLookups(3)))
	if err != nil {
		t.Fatalf("a font within the coverage cap was rejected: %v", err)
	}
	if f.gpos == nil || len(f.gpos.lookups) != 3 {
		t.Fatalf("the font loaded %d lookups, want 3", len(f.gpos.lookups))
	}
}

// TestTruncatedLookupIsNotACapError guards the "malformed is not the cap" half
// of RV5-29: a truncated Coverage returns no lookups and the font still loads.
func TestTruncatedLookupIsNotACapError(t *testing.T) {
	coverage := []byte{0, 2, 0, 10} // format 2 declares ten ranges, none present
	data := tinyFontWithGPOS(gposWithPairPos(pairPosFormat2Test(coverage)))
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("a truncated optional GPOS rejected the font: %v", err)
	}
	if errors.Is(err, ErrLimit) {
		t.Fatal("a truncated lookup was reported as resource/limit")
	}
	if f.gpos != nil {
		t.Fatal("a truncated lookup was kept")
	}
}

// TestCompositeComponentBeyondNumGlyphsIsRejected guards RV5-30: a component
// id equal to numGlyphs is an error in the outline reader and in the subsetter,
// which therefore writes no font.
func TestCompositeComponentBeyondNumGlyphsIsRejected(t *testing.T) {
	composite := compositeGlyphData(compositeRecord(0x0002, 2, 0))
	data := glyphFont([][]byte{nil, composite}) // numGlyphs is 2
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("an unused dangling component rejected the font: %v", err)
	}
	_, err = f.GlyphPath(1)
	if err == nil {
		t.Fatal("a dangling component produced an outline")
	}
	if !strings.Contains(err.Error(), "glyph 1") || !strings.Contains(err.Error(), "component 2") {
		t.Fatalf("the outline error does not name the glyph and component: %v", err)
	}
	subset, err := f.Subset(map[uint16]bool{1: true})
	if err == nil {
		t.Fatal("the subsetter accepted a dangling component")
	}
	if subset != nil {
		t.Fatal("a subset with a dangling reference was written")
	}
	if !strings.Contains(err.Error(), "component 2") {
		t.Fatalf("the subset error does not name the component: %v", err)
	}
}

// TestValidCompositeKeepsEveryComponent guards the valid half of RV5-30: a
// subset of a valid composite still contains every component.
func TestValidCompositeKeepsEveryComponent(t *testing.T) {
	f, err := Parse(testCompositeFont(false))
	if err != nil {
		t.Fatal(err)
	}
	subset, err := f.Subset(map[uint16]bool{2: true})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := Parse(subset)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}
	full, err := f.GlyphPath(2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := sub.GlyphPath(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(full) || len(got) == 0 {
		t.Fatalf("the subset composite has %d contours, want %d", len(got), len(full))
	}
	if got[0][0] != full[0][0] {
		t.Fatalf("the subset component moved: %v, want %v", got[0][0], full[0][0])
	}
}

// TestEmptyGlyphIsNotAnError guards RV5-31's legal case: equal loca offsets are
// an empty glyph, not a truncation.
func TestEmptyGlyphIsNotAnError(t *testing.T) {
	f, err := Parse(glyphFont([][]byte{nil, nil}))
	if err != nil {
		t.Fatal(err)
	}
	contours, err := f.GlyphPath(1)
	if err != nil {
		t.Fatalf("an empty glyph returned an error: %v", err)
	}
	if len(contours) != 0 {
		t.Fatalf("an empty glyph returned %d contours", len(contours))
	}
}

// TestTruncatedSimpleGlyphIsAnError guards RV5-31: a non-zero loca range that
// ends inside the contour data names the glyph instead of returning an empty
// outline.
func TestTruncatedSimpleGlyphIsAnError(t *testing.T) {
	header := make([]byte, 10)
	binary.BigEndian.PutUint16(header[0:2], 1) // one contour, endpoints missing
	f, err := Parse(glyphFont([][]byte{nil, header}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.GlyphPath(1); err == nil {
		t.Fatal("a truncated simple glyph returned an empty outline")
	} else if !strings.Contains(err.Error(), "glyph 1") {
		t.Fatalf("the error does not name the glyph: %v", err)
	}
}

// TestTruncatedCompositeGlyphIsAnError guards RV5-31's composite case: the
// component list says another record follows, but the data ends.
func TestTruncatedCompositeGlyphIsAnError(t *testing.T) {
	composite := compositeGlyphData(compositeRecord(0x0022, 0, 0)) // MORE_COMPONENTS
	f, err := Parse(glyphFont([][]byte{nil, composite}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.GlyphPath(1); err == nil {
		t.Fatal("a truncated composite returned a partial outline")
	} else if !strings.Contains(err.Error(), "glyph 1") {
		t.Fatalf("the error does not name the glyph: %v", err)
	}
	if _, err := f.Subset(map[uint16]bool{1: true}); err == nil {
		t.Fatal("the subsetter accepted a truncated component list")
	}
}

// TestMalformedGlyphsNeverPanic is the adversarial sweep: every crafted glyph
// goes through the outline reader and the subsetter without a panic, and each
// error names the glyph it came from.
func TestMalformedGlyphsNeverPanic(t *testing.T) {
	simple := make([]byte, 10)
	binary.BigEndian.PutUint16(simple[0:2], 1)
	composite := compositeGlyphData(compositeRecord(0x0022, 0, 0))
	dangling := compositeGlyphData(compositeRecord(0x0002, 9, 0))
	for name, glyph := range map[string][]byte{
		"truncated simple":    simple,
		"truncated composite": composite,
		"dangling component":  dangling,
	} {
		t.Run(name, func(t *testing.T) {
			f, err := Parse(glyphFont([][]byte{nil, glyph}))
			if err != nil {
				t.Fatal(err)
			}
			_, _ = f.GlyphPath(1)
			_, _ = f.Subset(map[uint16]bool{1: true})
		})
	}
}
