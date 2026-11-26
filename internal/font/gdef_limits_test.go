package font

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// gdefClassDef assembles a GDEF table whose glyph class definition is a
// format-2 ClassDef with the given ranges. The mark-set offset stays zero, so
// the parser reads only the class definition.
func gdefClassDef(ranges []classRange) []byte {
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], 12) // GlyphClassDef follows the header
	classDef := make([]byte, 4+6*len(ranges))
	binary.BigEndian.PutUint16(classDef[0:2], 2)
	binary.BigEndian.PutUint16(classDef[2:4], uint16(len(ranges)))
	for i, r := range ranges {
		base := 4 + i*6
		binary.BigEndian.PutUint16(classDef[base:base+2], r.start)
		binary.BigEndian.PutUint16(classDef[base+2:base+4], r.end)
		binary.BigEndian.PutUint16(classDef[base+4:base+6], r.class)
	}
	return append(out, classDef...)
}

// gdefMarkSets assembles a GDEF table whose mark-set table points every set at
// one shared Coverage, so a test can grow the expanded glyph count without
// growing the file.
func gdefMarkSets(coverage []byte, sets int) []byte {
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[10:12], 12) // mark-set table follows the header
	table := make([]byte, 4+4*sets)
	binary.BigEndian.PutUint16(table[0:2], 1)
	binary.BigEndian.PutUint16(table[2:4], uint16(sets))
	for i := 0; i < sets; i++ {
		binary.BigEndian.PutUint32(table[4+4*i:], uint32(len(table)))
	}
	table = append(table, coverage...)
	return append(out, table...)
}

// minimalGPOS is a GPOS table with an empty LookupList. A font needs a GPOS
// table for parseGPOS to read GDEF, and the empty list keeps the test's charge
// limited to the GDEF work.
func minimalGPOS() []byte {
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[8:10], 10) // LookupList offset
	// [10:12] stays zero: the list declares no lookups.
	return out
}

// tinyFontWithGDEF builds the smallest complete TrueType font carrying gdef
// and the empty GPOS table, so the GDEF parser is reached through the normal
// load path.
func tinyFontWithGDEF(gdef []byte) []byte {
	const numGlyphs = 2
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm
	binary.BigEndian.PutUint16(head[50:52], 1)    // long loca offsets
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:6], 800) // ascent
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics
	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], numGlyphs)
	hmtx := []byte{0, 100, 0, 0}
	loca := make([]byte, (numGlyphs+1)*4) // both glyphs are empty

	b := &fontBuilder{}
	b.add("head", head)
	b.add("hhea", hhea)
	b.add("maxp", maxp)
	b.add("hmtx", hmtx)
	b.add("loca", loca)
	b.add("glyf", nil)
	b.add("GPOS", minimalGPOS())
	b.add("GDEF", gdef)
	return b.bytes()
}

// stepCancelContext reports context.Canceled on its Nth Err call. Counting the
// calls gives a deterministic mid-parse checkpoint without a timer, and a test
// that removes a cancellation check parses past the expected work.
type stepCancelContext struct {
	context.Context
	calls    int
	cancelAt int
}

func (c *stepCancelContext) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

// TestGDEFClassExpansionIsChargedByWrites pins the independent cost of narrow,
// wide and overlapping class-3 ranges: one unit per range plus one per entry
// written, before the write. Overlapping ranges write the same key twice, and
// both writes are charged.
func TestGDEFClassExpansionIsChargedByWrites(t *testing.T) {
	ranges := []classRange{
		{start: 10, end: 10, class: 3},     // narrow: one entry
		{start: 100, end: 60099, class: 3}, // wide: 60,000 entries
		{start: 0, end: 4, class: 3},       // five entries
		{start: 2, end: 6, class: 3},       // five entries, two rewrites
		{start: 0, end: 10, class: 1},      // not a mark class: no expansion
	}
	data := gdefClassDef(ranges)

	// The expected cost is computed from the fixture, not from the production
	// charge helper: every class-3 range costs one unit plus its entry count.
	var wantUsed uint64
	writes := 0
	wantMarks := map[uint16]bool{}
	for _, r := range ranges {
		if r.class != 3 {
			continue
		}
		entries := int(r.end-r.start) + 1
		writes += entries
		wantUsed += uint64(entries) + 1
		for g := uint32(r.start); g <= uint32(r.end); g++ {
			wantMarks[uint16(g)] = true
		}
	}

	g := budget.New(context.Background(), 0)
	marks, sets, err := (&kerning{guard: g}).parseGDEF(data)
	if err != nil {
		t.Fatalf("parseGDEF: %v", err)
	}
	if used := g.Used(); used != wantUsed {
		t.Fatalf("%d entries over %d ranges cost %d units, want %d", writes, len(ranges), used, wantUsed)
	}
	if len(marks) != len(wantMarks) {
		t.Fatalf("marks = %d glyphs, want %d", len(marks), len(wantMarks))
	}
	for gid := range wantMarks {
		if !marks[gid] {
			t.Fatalf("glyph %d is missing from the mark map", gid)
		}
	}
	if sets != nil {
		t.Fatalf("the fixture declares no mark sets, got %v", sets)
	}
	// The two overlapping ranges write ten entries but only seven keys: the
	// charge counts writes, not distinct keys.
	if len(wantMarks) >= writes {
		t.Fatalf("the fixture has %d distinct marks and %d writes; the rewrite case was lost", len(wantMarks), writes)
	}
}

// TestGDEFClassExpansionBudgetBoundary stops one unit below the next
// range-plus-entry charge and admits it exactly, without returning a partial
// mark map after the refusal.
func TestGDEFClassExpansionBudgetBoundary(t *testing.T) {
	data := gdefClassDef([]classRange{
		{start: 0, end: 0, class: 3},   // costs 2
		{start: 10, end: 12, class: 3}, // costs 4
	})
	const firstCost, secondCost = 2, 4

	g := budget.New(context.Background(), firstCost+secondCost-1)
	marks, sets, err := (&kerning{guard: g}).parseGDEF(data)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("a budget one unit short returned %v, want ErrLimit", err)
	}
	if marks != nil || sets != nil {
		t.Fatalf("a refused expansion returned a partial mark set: %v", marks)
	}
	if used := g.Used(); used != firstCost+secondCost {
		t.Fatalf("the refused charge recorded %d units, want %d", used, firstCost+secondCost)
	}

	g = budget.New(context.Background(), firstCost+secondCost)
	marks, sets, err = (&kerning{guard: g}).parseGDEF(data)
	if err != nil {
		t.Fatalf("the exact budget returned %v", err)
	}
	if sets != nil {
		t.Fatalf("the fixture declares no mark sets, got %v", sets)
	}
	if len(marks) != 4 {
		t.Fatalf("the exact budget expanded %d marks, want 4", len(marks))
	}
	if used := g.Used(); used != firstCost+secondCost {
		t.Fatalf("the accepted expansion recorded %d units, want %d", used, firstCost+secondCost)
	}
}

// TestGDEFMarkExpansionIsCapped guards the expanded-entry cap: exactly the cap
// loads, one entry more is refused, and a refused table returns no mark map.
// The fixture crosses the cap by writing overlapping keys, so the bound counts
// writes rather than distinct glyphs.
func TestGDEFMarkExpansionIsCapped(t *testing.T) {
	atCap := []classRange{
		{start: 0, end: 65535, class: 3},
		{start: 0, end: 65535, class: 3},
		{start: 0, end: 65535, class: 3},
		{start: 0, end: 3391, class: 3},
	}
	marks, _, err := (&kerning{}).parseGDEF(gdefClassDef(atCap))
	if err != nil {
		t.Fatalf("a table at the cap returned %v", err)
	}
	if len(marks) != 65536 {
		t.Fatalf("the cap fixture marked %d glyphs, want 65536", len(marks))
	}

	overCap := append(append([]classRange{}, atCap...), classRange{start: 0, end: 3392, class: 3})
	marks, sets, err := (&kerning{}).parseGDEF(gdefClassDef(overCap))
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("a table one entry over the cap returned %v, want ErrLimit", err)
	}
	if marks != nil || sets != nil {
		t.Fatalf("the refused table returned a partial mark set: %d marks", len(marks))
	}
	// The refusal aborts the font load, not only the direct parse.
	if _, err := Load(tinyFontWithGDEF(gdefClassDef(overCap))); !errors.Is(err, ErrLimit) {
		t.Fatalf("a font over the mark cap returned %v, want ErrLimit", err)
	}

	// The format maximum: 65,535 overlapping full-space ranges stop at the cap
	// instead of allocating more than it.
	maxRanges := make([]classRange, 65535)
	for i := range maxRanges {
		maxRanges[i] = classRange{start: 0, end: 65535, class: 3}
	}
	marks, sets, err = (&kerning{}).parseGDEF(gdefClassDef(maxRanges))
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("the format-maximum table returned %v, want ErrLimit", err)
	}
	if marks != nil || sets != nil {
		t.Fatalf("the format-maximum table returned a mark map of %d glyphs", len(marks))
	}
}

// TestGDEFMarkSetCoverageLimitIsPropagated guards the coverage half: mark sets
// share the font's expanded-coverage budget, so a fourth set that does not fit
// is a resource refusal instead of a set silently dropped.
func TestGDEFMarkSetCoverageLimitIsPropagated(t *testing.T) {
	// Every set points at the same one-range Coverage: 65,535 glyphs each.
	coverage := coverageFormat2Test([][2]uint16{{0, 65534}})
	k := &kerning{}
	marks, sets, err := k.parseGDEF(gdefMarkSets(coverage, 4))
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("a mark set past the coverage cap returned %v, want ErrLimit", err)
	}
	if marks != nil || sets != nil {
		t.Fatal("the refused table returned a partial mark set")
	}
	if want := 3 * 65535; k.coverGlyphs != want {
		t.Fatalf("the parser kept %d coverage glyphs, want %d", k.coverGlyphs, want)
	}
	// The refusal aborts the font load rather than dropping the fourth set.
	if _, err := Load(tinyFontWithGDEF(gdefMarkSets(coverage, 4))); !errors.Is(err, ErrLimit) {
		t.Fatalf("a font with the oversized mark set returned %v, want ErrLimit", err)
	}
}

// TestMalformedGDEFStillDegrades keeps the optional-table rule: a truncated
// table returns no mark data and no error, so the font stays usable.
func TestMalformedGDEFStillDegrades(t *testing.T) {
	valid := gdefClassDef([]classRange{{start: 0, end: 3, class: 3}})
	truncated := valid[:len(valid)-2]
	marks, sets, err := (&kerning{}).parseGDEF(truncated)
	if err != nil || marks != nil || sets != nil {
		t.Fatalf("a truncated GDEF returned marks=%v sets=%v err=%v, want nil, nil, nil", marks, sets, err)
	}
	// The same table through the load path must not reject the font.
	f, err := Load(tinyFontWithGDEF(truncated))
	if err != nil {
		t.Fatalf("a font with a malformed optional GDEF was rejected: %v", err)
	}
	if f == nil {
		t.Fatal("the font was not returned")
	}
}

// TestGDEFLoadIsChargedByRangeEntries guards the plumbing: LoadWithOptions
// charges the shared guard for the two thousand ranges and their entries.
func TestGDEFLoadIsChargedByRangeEntries(t *testing.T) {
	ranges := make([]classRange, 2000)
	for i := range ranges {
		ranges[i] = classRange{start: uint16(i), end: uint16(i), class: 3}
	}
	data := tinyFontWithGDEF(gdefClassDef(ranges))
	g := budget.New(context.Background(), 0)
	f, err := LoadWithOptions(data, ParseOptions{Guard: g})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if f == nil {
		t.Fatal("no font was returned")
	}
	if want := uint64(2 * len(ranges)); g.Used() != want {
		t.Fatalf("two thousand one-entry ranges cost %d units, want %d", g.Used(), want)
	}
}

// TestGDEFAmplifierStopsAtTheWorkBudget is the 12 KB amplifier from the
// proposal: two thousand overlapping ranges expanding 131 million entries stop
// at a small budget instead of running the whole expansion.
func TestGDEFAmplifierStopsAtTheWorkBudget(t *testing.T) {
	ranges := make([]classRange, 2000)
	for i := range ranges {
		ranges[i] = classRange{start: 0, end: 65535, class: 3}
	}
	data := tinyFontWithGDEF(gdefClassDef(ranges))
	if len(data) < 12_000 || len(data) > 14_000 {
		t.Fatalf("the amplifier is %d bytes, want the 12 KB fixture", len(data))
	}
	g := budget.New(context.Background(), 250)
	f, err := LoadWithOptions(data, ParseOptions{Guard: g})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("the amplifier returned %v under a small budget, want ErrLimit", err)
	}
	if f != nil {
		t.Fatal("the amplifier returned a font despite the resource refusal")
	}
	if used := g.Used(); used == 0 || used > 1_000_000 {
		t.Fatalf("the amplifier charged %d units before it was refused", used)
	}
}

// TestGDEFNormalFontIsUnaffected keeps the cap honest: DejaVu Sans loads in the
// default limits with no error and a charge far below the budget.
func TestGDEFNormalFontIsUnaffected(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Skipf("no TrueType font available: %v", err)
	}
	g := budget.New(context.Background(), 0)
	f, err := LoadWithOptions(data, ParseOptions{Guard: g})
	if err != nil {
		t.Fatalf("a normal font was rejected: %v", err)
	}
	if f.gpos == nil {
		t.Fatal("the normal font lost its GPOS data")
	}
	if len(f.gpos.markGlyphs) == 0 {
		t.Fatal("the normal font lost its mark glyphs")
	}
	if used := g.Used(); used > 10_000 {
		t.Fatalf("a normal font cost %d units, want a negligible charge", used)
	}
}

// TestGDEFPreCancelledContextStopsBeforeParsing covers the pre-cancelled
// scenario: the load returns the context error before any range is charged.
func TestGDEFPreCancelledContextStopsBeforeParsing(t *testing.T) {
	ranges := make([]classRange, 2000)
	for i := range ranges {
		ranges[i] = classRange{start: 0, end: 65535, class: 3}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := budget.New(ctx, 0)
	f, err := LoadWithOptions(tinyFontWithGDEF(gdefClassDef(ranges)), ParseOptions{Guard: g})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a pre-cancelled load returned %v, want context.Canceled", err)
	}
	if f != nil {
		t.Fatal("a pre-cancelled load returned a font")
	}
	if used := g.Used(); used != 0 {
		t.Fatalf("a pre-cancelled load charged %d units before the GDEF parse", used)
	}
}

// TestGDEFClassExpansionCancellationStopsRemainingWork cancels at a counted
// checkpoint in the class loop: the parse stops before the next range and no
// font is returned, with the context error rather than resource/limit.
func TestGDEFClassExpansionCancellationStopsRemainingWork(t *testing.T) {
	ranges := make([]classRange, 2000)
	for i := range ranges {
		ranges[i] = classRange{start: 0, end: 0, class: 3} // overlapping
	}
	data := tinyFontWithGDEF(gdefClassDef(ranges))
	const cancelAt = 251
	ctx := &stepCancelContext{Context: context.Background(), cancelAt: cancelAt}
	g := budget.New(ctx, 0)
	f, err := LoadWithOptions(data, ParseOptions{Guard: g})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled class expansion returned %v, want context.Canceled", err)
	}
	if errors.Is(err, ErrLimit) {
		t.Fatalf("the cancellation was reported as a resource limit: %v", err)
	}
	if f != nil {
		t.Fatal("a cancelled load returned a font")
	}
	// The whole fixture would cost 4,000 units. Stopping around the counted
	// checkpoint proves the remaining ranges were not expanded; without the
	// check the periodic guard check would spend roughly twice as much.
	if used := g.Used(); used < 400 || used > 600 {
		t.Fatalf("cancellation stopped after %d units, want a mid-parse stop near 500", used)
	}
}

// TestGDEFMarkSetCoverageCancellationStopsRemainingWork is the mark-set half of
// the cancellation checkpoint: the loop stops before the next set is read.
func TestGDEFMarkSetCoverageCancellationStopsRemainingWork(t *testing.T) {
	coverage := coverageFormat1Test([]uint16{0})
	data := tinyFontWithGDEF(gdefMarkSets(coverage, 2000))
	const cancelAt = 251
	ctx := &stepCancelContext{Context: context.Background(), cancelAt: cancelAt}
	g := budget.New(ctx, 0)
	f, err := LoadWithOptions(data, ParseOptions{Guard: g})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled mark-set coverage returned %v, want context.Canceled", err)
	}
	if errors.Is(err, ErrLimit) {
		t.Fatalf("the cancellation was reported as a resource limit: %v", err)
	}
	if f != nil {
		t.Fatal("a cancelled load returned a font")
	}
	if used := g.Used(); used < 400 || used > 600 {
		t.Fatalf("cancellation stopped after %d units, want a mid-parse stop near 500", used)
	}
}
