package layout

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/testfont"
)

// assertNonNegativeRuns checks that no painted run carries a negative width or
// a negative advance: a wrapped int32 advance would move the pen backwards.
func assertNonNegativeRuns(t *testing.T, doc *Document) {
	t.Helper()
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			if op.Text.Run.Width < 0 {
				t.Fatalf("page %d paints a run with width %d", page.Number, op.Text.Run.Width)
			}
			for _, g := range op.Text.Run.Glyphs {
				if g.Advance < 0 {
					t.Fatalf("page %d paints a glyph with advance %d", page.Number, g.Advance)
				}
			}
			if op.Text.X < 0 {
				t.Fatalf("page %d paints a run at negative x=%d", page.Number, op.Text.X)
			}
		}
	}
}

// TestSaturatedWordReportsOnce guards RV5-09: a 350,000 character word at 12 pt
// wrapped the run and line widths negative, so the overflow check compared a
// negative natural width with the content box and stayed silent. The word is
// clamped now, reported once for the paragraph, and output stays non-negative.
func TestSaturatedWordReportsOnce(t *testing.T) {
	h := newHarness(t)
	doc, diags := h.runRaw(`<p>`+repeatRune('ж', 350000)+`</p>`, h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if len(doc.Pages) == 0 {
		t.Fatal("the document produced no page")
	}
	if n := countOverflow(diags); n != 1 {
		t.Fatalf("the saturated word reported %d layout/overflow diagnostics, want 1: %v", n, diags.Lines())
	}
	for _, d := range diags {
		if d.Code == "layout/overflow" && (d.Pos.File != "test.html" || d.Pos.Line != 1 || d.Pos.Col != 1) {
			t.Fatalf("the diagnostic position is %+v, want the paragraph at test.html:1:1", d.Pos)
		}
	}
	assertNonNegativeRuns(t, doc)
}

// TestSaturatedWordInTableCellReportsOnce guards the shared check for a table
// cell: the static cell painter compares the built lines with the cell width,
// so a saturated cell line reports once through the same path.
func TestSaturatedWordInTableCellReportsOnce(t *testing.T) {
	h := newHarness(t)
	_, diags := h.runRaw(`<table><tr><td>`+repeatRune('ж', 350000)+`</td></tr></table>`,
		h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if n := countOverflow(diags); n != 1 {
		t.Fatalf("the saturated cell reported %d layout/overflow diagnostics, want 1: %v", n, diags.Lines())
	}
	for _, d := range diags {
		if d.Code == "layout/overflow" && !strings.Contains(d.Element, "td") {
			t.Fatalf("the diagnostic does not name the cell: %s", d.String())
		}
	}
}

// TestLongWordWarningDoesNotRepeatAcrossPages guards the per-box latch: a
// paragraph that spans pages keeps one diagnostic, and its position names the
// paragraph instead of the fragment or the page that clipped it.
func TestLongWordWarningDoesNotRepeatAcrossPages(t *testing.T) {
	h := newHarness(t)
	var src strings.Builder
	src.WriteString(`<p>`)
	src.WriteString(repeatRune('ж', 350000))
	src.WriteString(" ")
	for i := 0; i < 1000; i++ {
		src.WriteString("слово ")
	}
	src.WriteString(`</p>`)
	doc, diags := h.runRaw(src.String(), h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if len(doc.Pages) < 2 {
		t.Fatalf("the paragraph produced %d pages, want a split paragraph", len(doc.Pages))
	}
	if n := countOverflow(diags); n != 1 {
		t.Fatalf("the split paragraph reported %d layout/overflow diagnostics, want 1: %v", n, diags.Lines())
	}
	for _, d := range diags {
		if d.Code != "layout/overflow" {
			continue
		}
		if d.Pos.File != "test.html" || d.Pos.Line != 1 || d.Pos.Col != 1 {
			t.Fatalf("the diagnostic position is %+v, want the paragraph at test.html:1:1", d.Pos)
		}
	}
}

// TestPreformattedLineSaturatesWithoutWrapping guards the line accumulation: a
// preformatted line of several runs keeps one line and saturates in int64
// instead of wrapping to a negative natural width that would place the later
// runs before the first.
func TestPreformattedLineSaturatesWithoutWrapping(t *testing.T) {
	h := newHarness(t)
	src := `<p style="white-space:pre;font-size:2000000pt;margin:0">A A A</p>`
	doc, diags := h.runRaw(src, h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if n := countOverflow(diags); n != 1 {
		t.Fatalf("the saturated line reported %d layout/overflow diagnostics, want 1: %v", n, diags.Lines())
	}
	assertNonNegativeRuns(t, doc)
	// Every item of the preformatted line is painted at one baseline, and their
	// widths together exceed the int32 range: the fixture still exercises the
	// saturation it guards.
	var total int64
	var baseline int32
	first := true
	for _, op := range doc.Pages[0].Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		if first {
			baseline = op.Text.Y
			first = false
		}
		if op.Text.Y != baseline {
			t.Fatalf("the preformatted line was split: baselines %d and %d", baseline, op.Text.Y)
		}
		total += int64(op.Text.Run.Width)
	}
	if total <= math.MaxInt32 {
		t.Fatalf("the fixture width %d no longer exceeds MaxInt32", total)
	}
}

// TestExtremeFontSizeIsDiagnosed guards RV5-09: a font size whose scaled
// advances leave the int32 range is reported and still renders a clamped,
// non-negative advance instead of wrapped geometry.
func TestExtremeFontSizeIsDiagnosed(t *testing.T) {
	h := newHarness(t)
	doc, diags := h.runRaw(`<p style="font-size:2000000pt;margin:0">AB</p>`,
		h.ctx.PageSize, h.ctx.Margin)
	if diags.HasErrors() {
		t.Fatalf("layout errors: %v", diags.Lines())
	}
	if n := countOverflow(diags); n != 1 {
		t.Fatalf("the extreme font size reported %d layout/overflow diagnostics, want 1: %v", n, diags.Lines())
	}
	assertNonNegativeRuns(t, doc)
}

// TestLineOverflowCondition guards the exact overflow predicate: a fitting line
// is quiet, a wider line is reported, a saturated line is reported even when
// its clamped width equals the content width, and a face whose metrics clamp is
// reported even when the line itself fits.
func TestLineOverflowCondition(t *testing.T) {
	const width = int32(1000)
	if lineOverflows(Line{Natural: width}, width) {
		t.Fatal("a fitting line reported overflow")
	}
	if !lineOverflows(Line{Natural: width + 1}, width) {
		t.Fatal("a line wider than the content box was not reported")
	}
	if !lineOverflows(Line{Natural: width, Saturated: true}, width) {
		t.Fatal("a saturated line equal to the content width was not reported")
	}

	clampedFace := &FontFace{Font: fontWithUnitsPerEm(t, 16), Size: 2000000000}
	if !lineOverflows(Line{Natural: 10, Items: []LineItem{{Kind: TextItem, Face: clampedFace}}}, width) {
		t.Fatal("a line using a face with clamped metrics was not reported")
	}
	normalFace := &FontFace{Font: loadTestFont(t), Size: 12000}
	if lineOverflows(Line{Natural: 10, Items: []LineItem{{Kind: TextItem, Face: normalFace}}}, width) {
		t.Fatal("an in-range face reported overflow")
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
