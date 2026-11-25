package font

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// gposTable builds a GPOS table whose LookupList declares `lookups` PairPos
// lookups, each declaring `subtables` subtables. Every subtable offset points
// at its own lookup: the data is not a valid PairPos, which is exactly the
// point — the parser must stop on the declared counts and the budget before it
// reads them.
func gposTable(lookups, subtables int) []byte {
	const lookupList = 10
	offsets := lookups * 2
	lookupArea := lookupList + 2 + offsets
	lookupSize := 6 + subtables*2
	out := make([]byte, lookupArea+lookups*lookupSize)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[8:10], lookupList)
	binary.BigEndian.PutUint16(out[lookupList:lookupList+2], uint16(lookups))
	for i := 0; i < lookups; i++ {
		off := lookupArea + i*lookupSize
		binary.BigEndian.PutUint16(out[lookupList+2+i*2:], uint16(off-lookupList))
		binary.BigEndian.PutUint16(out[off:off+2], 2) // PairPos
		binary.BigEndian.PutUint16(out[off+4:off+6], uint16(subtables))
	}
	return out
}

// tinyFontWithGPOS builds the smallest complete TrueType font that carries the
// given GPOS table, so the table is reached through the normal parse path.
func tinyFontWithGPOS(gpos []byte) []byte {
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
	b.add("GPOS", gpos)
	return b.bytes()
}

// TestGPOSDeclaredLookupCountIsCapped guards the lookup cap: the declared count
// is checked before the LookupList is walked, so the maximum value is rejected
// cheaply instead of being parsed.
func TestGPOSDeclaredLookupCountIsCapped(t *testing.T) {
	data := tinyFontWithGPOS(gposTable(maxGPOSLookups+1, 1))
	if _, err := Parse(data); !errors.Is(err, ErrLimit) {
		t.Fatalf("a font declaring %d lookups returned %v, want ErrLimit", maxGPOSLookups+1, err)
	}
}

// TestGPOSDeclaredSubtableCountIsCapped guards the subtable cap, the inner half
// of the lookup-by-subtable cross product.
func TestGPOSDeclaredSubtableCountIsCapped(t *testing.T) {
	data := tinyFontWithGPOS(gposTable(1, maxGPOSSubtables+1))
	if _, err := Parse(data); !errors.Is(err, ErrLimit) {
		t.Fatalf("a font declaring %d subtables returned %v, want ErrLimit", maxGPOSSubtables+1, err)
	}
}

// TestGPOSAmplifierStopsAtTheWorkBudget is the amplifier fixture: many
// subtables are charged to the shared work guard, so a small budget stops the
// parse instead of letting a small file buy large work.
func TestGPOSAmplifierStopsAtTheWorkBudget(t *testing.T) {
	data := tinyFontWithGPOS(gposTable(1, 100))
	guard := budget.New(context.Background(), 8)
	_, err := ParseWithOptions(data, ParseOptions{Guard: guard})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("the amplifier returned %v under a small budget, want ErrLimit", err)
	}
	if !errors.Is(guard.Err(), budget.ErrExceeded) {
		t.Fatalf("the parse did not charge the guard: %v", guard.Err())
	}
}

// TestGPOSNormalTableIsUnaffected keeps the cap honest: a small GPOS parses
// with the default limits and no cancellation.
func TestGPOSNormalTableIsUnaffected(t *testing.T) {
	data := tinyFontWithGPOS(gposTable(1, 2))
	if _, err := Parse(data); err != nil {
		t.Fatalf("a small GPOS was rejected: %v", err)
	}
}

// TestGPOSParsingObservesCancellation covers the cancellation scenario: a
// cancelled conversion stops before the parse starts.
func TestGPOSParsingObservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := tinyFontWithGPOS(gposTable(1, 2))
	_, err := ParseWithOptions(data, ParseOptions{Guard: budget.New(ctx, 0)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled parse returned %v, want context.Canceled", err)
	}
}
