package text

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/testfont"
)

// repeatRune returns r repeated n times.
func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}

// TestAdvanceWidthSaturates guards RV5-09: the run width was an int32 sum, so a
// word of 350,000 Cyrillic characters at 12 pt wrapped to a negative width. The
// sum is 64-bit now and saturates at MaxInt32 while reporting the clamp; a
// width below the range keeps its exact value and does not report saturation.
func TestAdvanceWidthSaturates(t *testing.T) {
	f := testFont(t)

	big, err := Shape(f, 12000, repeatRune('ж', 350000))
	if err != nil {
		t.Fatal(err)
	}
	if big.Width != math.MaxInt32 {
		t.Fatalf("350,000 characters shaped to width %d, want MaxInt32", big.Width)
	}
	if !big.Saturated {
		t.Fatal("the clamped run did not report saturation")
	}

	small, err := Shape(f, 12000, repeatRune('ж', 1000))
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, g := range small.Glyphs {
		sum += int64(g.Advance)
	}
	if sum >= math.MaxInt32 {
		t.Fatalf("the in-range fixture no longer fits the range: %d", sum)
	}
	if int64(small.Width) != sum {
		t.Fatalf("in-range width %d, want the exact sum %d", small.Width, sum)
	}
	if small.Saturated {
		t.Fatal("an in-range run reported saturation")
	}
}

// TestAdvanceWidthBoundary guards the exact boundary: a run whose true sum is
// MaxInt32 keeps the value without reporting saturation, one above it reports
// the clamp, and a hostile or malformed font cannot wrap the sum into a
// negative advance.
func TestAdvanceWidthBoundary(t *testing.T) {
	exact := []Glyph{{Advance: math.MaxInt32}}
	if w, saturated := AdvanceWidth(exact); w != math.MaxInt32 || saturated {
		t.Fatalf("exactly MaxInt32: width %d, saturated %v", w, saturated)
	}

	over := []Glyph{{Advance: math.MaxInt32}, {Advance: 1}}
	if w, saturated := AdvanceWidth(over); w != math.MaxInt32 || !saturated {
		t.Fatalf("MaxInt32+1: width %d, saturated %v", w, saturated)
	}

	hostile := make([]Glyph, 100000)
	for i := range hostile {
		hostile[i].Advance = math.MaxInt32
	}
	if w, saturated := AdvanceWidth(hostile); w != math.MaxInt32 || !saturated {
		t.Fatalf("a hostile run: width %d, saturated %v", w, saturated)
	}

	// A negative sum can only come from a malformed font; a run width is never
	// negative and the clamp is reported.
	negative := []Glyph{{Advance: -1}, {Advance: -1}}
	if w, saturated := AdvanceWidth(negative); w != 0 || !saturated {
		t.Fatalf("a negative run: width %d, saturated %v", w, saturated)
	}
}

// TestScaleClampsBeforeNarrowing guards RV5-09: Scale narrowed the float64
// product straight to int32, which is implementation-defined when out of range.
// Every case now rounds once and clamps to the int32 range on both
// architectures.
func TestScaleClampsBeforeNarrowing(t *testing.T) {
	cases := []struct {
		name          string
		v, size, upem int32
		want          int32
	}{
		{"in range", 1000, 12000, 1000, 12000},
		{"rounds once", 1, 1, 2, 1},
		{"positive overflow", 60000, 2000000000, 1000, math.MaxInt32},
		{"negative overflow", -60000, 2000000000, 1000, math.MinInt32},
		{"zero units per em", 1000, 12000, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Scale(tc.v, tc.size, tc.upem); got != tc.want {
				t.Fatalf("Scale(%d, %d, %d) = %d, want %d", tc.v, tc.size, tc.upem, got, tc.want)
			}
		})
	}
	// scaleFontUnits applies the same rule to a 64-bit font-unit value.
	if got := scaleFontUnits(1<<40, 2000000000, 1000); got != math.MaxInt32 {
		t.Fatalf("scaleFontUnits overflowed to %d, want MaxInt32", got)
	}
	if got := scaleFontUnits(100, 12000, 0); got != 0 {
		t.Fatalf("scaleFontUnits with a zero em = %d, want 0", got)
	}
}

// TestMetricsClampOutOfRange guards the face metrics: an extreme size must
// clamp ascent and descent explicitly and report that clamping, so layout can
// diagnose it instead of painting wrapped geometry.
func TestMetricsClampOutOfRange(t *testing.T) {
	// A small em lets a size inside the CSS length range overflow the metric
	// product; the real face is the bundled test font.
	f := fontWithUnitsPerEm(t, 16)
	m := MetricsOf(f, 2000000000)
	if m.Ascent != math.MaxInt32 || m.Descent != math.MaxInt32 {
		t.Fatalf("clamped metrics = %+v, want both MaxInt32", m)
	}
	if !MetricsClamped(f, 2000000000) {
		t.Fatal("the clamped metrics were not reported as clamped")
	}

	normal := MetricsOf(f, 12000)
	if normal.Ascent <= 0 || normal.Descent <= 0 {
		t.Fatalf("in-range metrics = %+v", normal)
	}
	if MetricsClamped(f, 12000) {
		t.Fatal("in-range metrics reported clamping")
	}

	// A face without a usable em never divides by zero.
	if m := MetricsOf(nil, 12000); m.Ascent != 0 || m.Descent != 0 {
		t.Fatalf("nil font metrics = %+v, want zero", m)
	}
	if MetricsClamped(nil, 12000) {
		t.Fatal("a nil font reported clamped metrics")
	}
}

// fontWithUnitsPerEm loads the test font with a replaced unitsPerEm, so a size
// within the document's length range can push a metric past the int32 range.
func fontWithUnitsPerEm(t *testing.T, upem uint16) *font.Font {
	t.Helper()
	data, err := os.ReadFile(testfont.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := setHeadUnitsPerEm(data, upem); err != nil {
		t.Fatal(err)
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// setHeadUnitsPerEm edits the head table of an sfnt in place. The parser does
// not verify table checksums, so the edit needs no directory rewrite.
func setHeadUnitsPerEm(data []byte, upem uint16) error {
	if len(data) < 12 {
		return fmt.Errorf("font is too short")
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	for i := 0; i < numTables; i++ {
		rec := 12 + i*16
		if rec+16 > len(data) {
			return fmt.Errorf("truncated table directory")
		}
		if string(data[rec:rec+4]) != "head" {
			continue
		}
		off := int(binary.BigEndian.Uint32(data[rec+8 : rec+12]))
		if off+20 > len(data) {
			return fmt.Errorf("head table is truncated")
		}
		binary.BigEndian.PutUint16(data[off+18:off+20], upem)
		return nil
	}
	return fmt.Errorf("font has no head table")
}
