package font_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/vyachin/go-pdf/internal/font"
)

// subsetText is the text whose glyphs the subset keeps: Latin and Cyrillic, so
// both the ASCII and the Cyrillic halves of the font are exercised.
const subsetText = "Счёт ABCabc 0123"

// TestSubsetOracle hands the subsetter's output to the independent decoder.
//
// sfnt.Parse requires a `cmap` and a `post` table, and Subset drops both on
// purpose (it keeps the glyph numbering instead, so the original cmap stays
// valid). The test therefore restores those two tables from the original font
// and lets sfnt read the subsetter's rewritten `head`, `hhea`, `maxp`, `hmtx`,
// `loca` and `glyf` tables as they were written. That the restored cmap still
// resolves every rune to the same glyph is itself the proof that the numbering
// the subsetter relies on survived.
func TestSubsetOracle(t *testing.T) {
	for _, name := range []string{"DejaVuSans.ttf", "DejaVuSans-Bold.ttf"} {
		name := name
		t.Run(name, func(t *testing.T) {
			eng, data := bundled(t, name)
			gids, runes := usedGlyphs(t, eng)
			subset, err := eng.Subset(toSet(gids))
			if err != nil {
				t.Fatalf("Subset: %v", err)
			}
			if len(subset) >= len(data) {
				t.Fatalf("the subset (%d bytes) is not smaller than the font (%d)", len(subset), len(data))
			}
			fixture, err := restoreDroppedTables(subset, data)
			if err != nil {
				t.Fatal(err)
			}
			sf, err := sfnt.Parse(fixture)
			if err != nil {
				t.Fatalf("sfnt rejects the subset: %v", err)
			}
			if err := compareSubset(eng, sf, gids); err != nil {
				t.Fatal(err)
			}
			// The cmap of the original resolves through the subset's glyphs.
			var buf sfnt.Buffer
			for r, want := range runes {
				got, err := sf.GlyphIndex(&buf, r)
				if err != nil || uint16(got) != want {
					t.Fatalf("U+%04X resolves to %d (err %v) in the subset, %d in the original", r, got, err, want)
				}
			}
			// A glyph the subset drops has no outline but keeps its slot.
			unused := firstUnused(eng, toSet(gids))
			segs, err := sf.LoadGlyph(&buf, sfnt.GlyphIndex(unused), fixed.Int26_6(sf.UnitsPerEm()), nil)
			if err != nil {
				t.Fatalf("glyph %d: %v", unused, err)
			}
			if len(segs) != 0 {
				t.Fatalf("glyph %d should be empty in the subset", unused)
			}
		})
	}
}

// compareSubset checks the glyph count, the em size, and the advance and outline
// of every retained glyph of a subset against the font it came from.
func compareSubset(eng *font.Font, sf *sfnt.Font, gids []uint16) error {
	if int(eng.UnitsPerEm()) != int(sf.UnitsPerEm()) {
		return fmt.Errorf("units per em: original %d, subset %d", eng.UnitsPerEm(), sf.UnitsPerEm())
	}
	if int(eng.NumGlyphs()) != sf.NumGlyphs() {
		return fmt.Errorf("glyph count: original %d, subset %d", eng.NumGlyphs(), sf.NumGlyphs())
	}
	var buf sfnt.Buffer
	ppem := fixed.Int26_6(sf.UnitsPerEm())
	for _, gid := range gids {
		adv, err := sf.GlyphAdvance(&buf, sfnt.GlyphIndex(gid), ppem, xfont.HintingNone)
		if err != nil {
			return fmt.Errorf("glyph %d: advance: %w", gid, err)
		}
		if diff(int(adv), int(eng.Advance(gid))) > tolerance {
			return fmt.Errorf("glyph %d: advance %d in the subset, %d in the original", gid, adv, eng.Advance(gid))
		}
		want, err := sf.LoadGlyph(&buf, sfnt.GlyphIndex(gid), ppem, nil)
		if err != nil {
			return fmt.Errorf("glyph %d: outline: %w", gid, err)
		}
		contours, err := eng.GlyphPath(gid)
		if err != nil {
			return fmt.Errorf("glyph %d: original outline: %w", gid, err)
		}
		if err := compareSegments(int(gid), contourSegments(contours), want); err != nil {
			return err
		}
	}
	return nil
}

// TestSubsetOracleCanFail proves the subset oracle is not a rubber stamp: one
// rewritten outline byte, decoded by sfnt, must be reported and the glyph named.
func TestSubsetOracleCanFail(t *testing.T) {
	eng, data := bundled(t, "DejaVuSans.ttf")
	gids, _ := usedGlyphs(t, eng)
	subset, err := eng.Subset(toSet(gids))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := restoreDroppedTables(subset, data)
	if err != nil {
		t.Fatal(err)
	}
	honest, err := sfnt.Parse(fixture)
	if err != nil {
		t.Fatalf("sfnt rejects the honest subset: %v", err)
	}
	if err := compareSubset(eng, honest, gids); err != nil {
		t.Fatalf("the honest subset already disagrees: %v", err)
	}

	// Corrupt one byte of the first retained glyph's outline in the subset.
	corrupted, err := corruptGlyph(fixture, gids[0])
	if err != nil {
		t.Fatal(err)
	}
	other, err := sfnt.Parse(corrupted)
	if err != nil {
		// Rejecting the file outright is the other accepted failure mode.
		t.Logf("sfnt rejects the corrupted subset: %v", err)
		return
	}
	err = compareSubset(eng, other, gids)
	if err == nil {
		t.Fatal("the subset oracle missed a corrupted outline")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("glyph %d", gids[0])) {
		t.Fatalf("the error does not name the glyph: %v", err)
	}
}

// TestCmapDivergenceIsCaught renames the cmap table of the font the engine
// reads: the engine still parses it but maps nothing, while sfnt maps the
// document's characters, and the comparison must name the code point.
//
// The mutation is applied to the engine's copy because sfnt refuses to parse a
// font without a cmap at all, so a renamed cmap on that side would fail before
// the comparison.
func TestCmapDivergenceIsCaught(t *testing.T) {
	eng, data := bundled(t, "DejaVuSans.ttf")
	mutated, err := renameTable(data, "cmap", "bmap")
	if err != nil {
		t.Fatal(err)
	}
	blind, err := font.Parse(mutated)
	if err != nil {
		t.Fatalf("the engine rejects a font with a renamed cmap: %v", err)
	}
	sf, err := sfnt.Parse(data)
	if err != nil {
		t.Fatalf("sfnt: %v", err)
	}
	err = compareCmap(blind, sf)
	if err == nil {
		t.Fatal("the cmap comparison missed a missing cmap")
	}
	first := firstMapped(t, eng)
	if !strings.Contains(err.Error(), fmt.Sprintf("U+%04X", first)) {
		t.Fatalf("the error does not name U+%04X: %v", first, err)
	}
}

// TestAdvanceDivergenceIsCaught changes one hmtx record of the font sfnt reads:
// the comparison must name that glyph.
func TestAdvanceDivergenceIsCaught(t *testing.T) {
	eng, data := bundled(t, "DejaVuSans.ttf")
	const gid = 36 // 'A' in DejaVuSans
	mutated, err := setAdvance(data, gid, "+100")
	if err != nil {
		t.Fatal(err)
	}
	sf, err := sfnt.Parse(mutated)
	if err != nil {
		t.Fatalf("a font with a changed advance must still parse: %v", err)
	}
	err = compareMetrics(eng, sf)
	if err == nil {
		t.Fatal("the metrics comparison missed a changed advance")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("glyph %d", gid)) {
		t.Fatalf("the error does not name glyph %d: %v", gid, err)
	}
}

// TestOutlineDivergenceIsCaught changes one outline byte of the font sfnt reads:
// the comparison must name that glyph.
func TestOutlineDivergenceIsCaught(t *testing.T) {
	eng, data := bundled(t, "DejaVuSans.ttf")
	const gid = 36 // 'A' in DejaVuSans
	mutated, err := corruptGlyph(data, gid)
	if err != nil {
		t.Fatal(err)
	}
	sf, err := sfnt.Parse(mutated)
	if err != nil {
		t.Fatalf("a font with one changed outline byte must still parse: %v", err)
	}
	err = compareOutlines(eng, sf)
	if err == nil {
		t.Fatal("the outline comparison missed a changed byte")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("glyph %d", gid)) {
		t.Fatalf("the error does not name glyph %d: %v", gid, err)
	}
}

// bundled returns one of the two bundled fonts parsed by the engine, with its
// bytes.
func bundled(t *testing.T, name string) (*font.Font, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", name))
	if err != nil {
		t.Skipf("the bundled font is missing: %v", err)
	}
	eng, err := font.Parse(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return eng, data
}

// usedGlyphs returns the sorted glyphs of subsetText and the rune-to-glyph map
// that produced them.
func usedGlyphs(t *testing.T, eng *font.Font) ([]uint16, map[rune]uint16) {
	t.Helper()
	runes := map[rune]uint16{}
	set := map[uint16]bool{}
	for _, r := range subsetText {
		if r == ' ' {
			continue
		}
		gid, ok := eng.GlyphIndex(r)
		if !ok {
			t.Fatalf("the font has no glyph for %q", r)
		}
		runes[r] = gid
		set[gid] = true
	}
	gids := make([]uint16, 0, len(set))
	for gid := range set {
		gids = append(gids, gid)
	}
	sort.Slice(gids, func(i, j int) bool { return gids[i] < gids[j] })
	return gids, runes
}

func toSet(gids []uint16) map[uint16]bool {
	set := make(map[uint16]bool, len(gids)+1)
	set[0] = true
	for _, gid := range gids {
		set[gid] = true
	}
	return set
}

// firstUnused returns a glyph the subset does not keep.
func firstUnused(eng *font.Font, used map[uint16]bool) uint16 {
	for gid := 1; gid < int(eng.NumGlyphs()); gid++ {
		if !used[uint16(gid)] {
			return uint16(gid)
		}
	}
	return 0
}

// firstMapped returns the first rune of the sample the engine maps, which is
// the first one a divergence report can name.
func firstMapped(t *testing.T, eng *font.Font) rune {
	t.Helper()
	for _, r := range sampleRunes() {
		if _, ok := eng.GlyphIndex(r); ok {
			return r
		}
	}
	t.Fatal("the engine maps no rune of the sample")
	return 0
}

// --- Fixture assembly -------------------------------------------------------
//
// The helpers below read and write an SFNT file's table directory. They are not
// decoders: they exist so a test can hand sfnt a subset that carries the two
// tables sfnt requires, and so a test can corrupt exactly one table and keep
// the file parseable. Nothing here is used as a reference.

// sfntTable is one table of an SFNT file.
type sfntTable struct {
	tag  string
	data []byte
}

// tablesOf splits a font into its tables.
func tablesOf(data []byte) ([]sfntTable, error) {
	if len(data) < 12 {
		return nil, errors.New("font is too short")
	}
	n := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+n*16 {
		return nil, errors.New("table directory is truncated")
	}
	out := make([]sfntTable, 0, n)
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		off := int(binary.BigEndian.Uint32(data[rec+8 : rec+12]))
		length := int(binary.BigEndian.Uint32(data[rec+12 : rec+16]))
		if off < 0 || length < 0 || off+length > len(data) {
			return nil, fmt.Errorf("table %q is out of bounds", data[rec:rec+4])
		}
		out = append(out, sfntTable{tag: string(data[rec : rec+4]), data: data[off : off+length]})
	}
	return out, nil
}

// assemble writes the tables back as an SFNT file.
func assemble(tables []sfntTable) []byte {
	slices.SortFunc(tables, func(a, b sfntTable) int { return strings.Compare(a.tag, b.tag) })
	n := len(tables)
	pow2 := 1
	for pow2*2 <= n {
		pow2 *= 2
	}
	log2 := 0
	for 1<<log2 <= pow2 {
		log2++
	}
	out := make([]byte, 12+n*16)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(n))
	binary.BigEndian.PutUint16(out[6:8], uint16(pow2*16))
	binary.BigEndian.PutUint16(out[8:10], uint16(log2-1))
	binary.BigEndian.PutUint16(out[10:12], uint16(n*16-pow2*16))
	for i, t := range tables {
		rec := 12 + i*16
		copy(out[rec:rec+4], t.tag)
		binary.BigEndian.PutUint32(out[rec+4:rec+8], tableChecksum(t.data))
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(len(out)))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(t.data)))
		out = append(out, t.data...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	return out
}

// tableChecksum is the sum the SFNT directory records for a table. sfnt ignores
// it; it is written so the fixture is a well-formed font.
func tableChecksum(b []byte) uint32 {
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

// restoreDroppedTables adds the `cmap` and `post` tables of the original font
// to a subset that dropped them, because sfnt.Parse requires both.
func restoreDroppedTables(subset, original []byte) ([]byte, error) {
	tables, err := tablesOf(subset)
	if err != nil {
		return nil, err
	}
	source, err := tablesOf(original)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, t := range tables {
		have[t.tag] = true
	}
	for _, tag := range []string{"cmap", "post"} {
		if have[tag] {
			continue
		}
		found := false
		for _, t := range source {
			if t.tag == tag {
				tables = append(tables, t)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("the original font has no %q table", tag)
		}
	}
	return assemble(tables), nil
}

// tableRange returns the offset and length of a table in a font's directory.
func tableRange(data []byte, tag string) (off, length int, ok bool) {
	if len(data) < 12 {
		return 0, 0, false
	}
	n := int(binary.BigEndian.Uint16(data[4:6]))
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if string(data[rec:rec+4]) == tag {
			return int(binary.BigEndian.Uint32(data[rec+8 : rec+12])), int(binary.BigEndian.Uint32(data[rec+12 : rec+16])), true
		}
	}
	return 0, 0, false
}

// renameTable changes a table's tag in place, keeping the directory sorted.
func renameTable(data []byte, from, to string) ([]byte, error) {
	out := slices.Clone(data)
	n := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if string(out[rec:rec+4]) == from {
			copy(out[rec:rec+4], to)
			return out, nil
		}
	}
	return nil, fmt.Errorf("no %q table", from)
}

// setAdvance rewrites one hmtx record, so the advance disagrees with the table
// the engine read.
func setAdvance(data []byte, gid uint16, delta string) ([]byte, error) {
	out := slices.Clone(data)
	hhea, hheaLen, ok := tableRange(out, "hhea")
	if !ok || hheaLen < 36 {
		return nil, errors.New("no hhea table")
	}
	hmtx, hmtxLen, ok := tableRange(out, "hmtx")
	if !ok || hmtxLen < 4 {
		return nil, errors.New("no hmtx table")
	}
	numMetrics := int(binary.BigEndian.Uint16(out[hhea+34 : hhea+36]))
	idx := int(gid)
	if idx >= numMetrics {
		idx = numMetrics - 1
	}
	pos := hmtx + 4*idx
	if pos+2 > hmtx+hmtxLen {
		return nil, fmt.Errorf("hmtx record for glyph %d is out of bounds", gid)
	}
	value := int(binary.BigEndian.Uint16(out[pos : pos+2]))
	// delta is a string only so a test reads as its intent; keep it explicit.
	switch delta {
	case "+100":
		value += 100
	default:
		return nil, fmt.Errorf("unknown advance change %q", delta)
	}
	binary.BigEndian.PutUint16(out[pos:pos+2], uint16(value))
	return out, nil
}

// corruptGlyph changes one byte of a glyph's data: the high byte of its contour
// count, so the glyph describes a number of contours the data does not hold.
// The font still parses; the glyph does not, which is a failure the oracle
// must report.
func corruptGlyph(data []byte, gid uint16) ([]byte, error) {
	out := slices.Clone(data)
	start, end, err := glyphRange(out, gid)
	if err != nil {
		return nil, err
	}
	if end-start < 2 {
		return nil, fmt.Errorf("glyph %d is empty", gid)
	}
	out[start] ^= 0x40
	return out, nil
}

// glyphRange returns the offset of one glyph's data in a font, using only the
// head and loca tables.
func glyphRange(data []byte, gid uint16) (start, end int, err error) {
	head, headLen, ok := tableRange(data, "head")
	if !ok || headLen < 52 {
		return 0, 0, errors.New("no head table")
	}
	loca, locaLen, ok := tableRange(data, "loca")
	if !ok {
		return 0, 0, errors.New("no loca table")
	}
	glyf, glyfLen, ok := tableRange(data, "glyf")
	if !ok {
		return 0, 0, errors.New("no glyf table")
	}
	format := int16(binary.BigEndian.Uint16(data[head+50 : head+52]))
	idx := int(gid)
	if format == 0 {
		if locaLen < (idx+2)*2 {
			return 0, 0, fmt.Errorf("loca is too short for glyph %d", gid)
		}
		start = int(binary.BigEndian.Uint16(data[loca+idx*2:])) * 2
		end = int(binary.BigEndian.Uint16(data[loca+(idx+1)*2:])) * 2
	} else {
		if locaLen < (idx+2)*4 {
			return 0, 0, fmt.Errorf("loca is too short for glyph %d", gid)
		}
		start = int(binary.BigEndian.Uint32(data[loca+idx*4:]))
		end = int(binary.BigEndian.Uint32(data[loca+(idx+1)*4:]))
	}
	if start < 0 || end < start || end > glyfLen {
		return 0, 0, fmt.Errorf("glyph %d is out of bounds", gid)
	}
	return glyf + start, glyf + end, nil
}
