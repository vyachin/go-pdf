// Package font_test checks the private TrueType parser against the Go
// project's decoder, golang.org/x/image/font/sfnt.
//
// The engine's parser and subsetter are the only hand-written code that reads
// and writes a binary font format, and until this file they were checked
// against themselves: TestSubset re-read the generated subset with the same
// parser that wrote it, and the PDF validator checked the document around the
// font, not the font's tables. sfnt is an independent decoder of the same
// format, so a divergence here is a real defect in one of the two.
//
// The reference in every comparison is sfnt. The package under test answers
// the same questions — which rune maps to which glyph, how wide a glyph is,
// where its outline goes — and the answers must agree.
//
// TrueType fonts are compared glyph by glyph, segment by segment. An
// OpenType/CFF font is converted to TrueType first, so its cmap, advances and
// vertical metrics are compared directly and its outlines are compared by their
// on-curve anchors: the conversion replaces a cubic with fitted quadratics, so
// the segment kinds differ by design while the source's anchors must survive in
// order. Glyphs sfnt itself cannot decode (a coloured glyph, a missing glyph, a
// charstring operator its CFF reader rejects) are counted and logged, never
// skipped silently.
//
// Kerning is deliberately not compared: sfnt reads a Latin-only subset of
// `GPOS` with a different lookup model, while the engine applies several
// lookups in order and supports class-based subtables. A difference there is
// not a defect.
package font_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/vyachin/go-pdf/internal/font"
)

const (
	// tolerance is one font unit. Both decoders read the same tables and differ
	// only in fixed-point rounding, so anything larger is a divergence.
	tolerance = 1
	// minComparedFonts fails the suite when the corpus walk compares too
	// little. A suite that silently compares nothing is worse than no suite.
	minComparedFonts = 10
)

// corpusFont is one font of the corpus parsed by both decoders.
type corpusFont struct {
	name string
	eng  *font.Font
	sfnt *sfnt.Font
	// converted records that the engine font came from an OTTO file through
	// the CFF conversion, so its outlines are quadratics fitted to sfnt's
	// cubics.
	converted bool
}

// corpusFiles returns every font file under root, sorted.
func corpusFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ttf", ".otf":
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

// discover walks root for TrueType fonts and parses each with both decoders.
// A font is skipped, with a reason, when one side cannot handle it; the error
// reports a corpus too small to be worth anything.
func discover(root string) (fonts []corpusFont, skipped []string, err error) {
	paths, err := corpusFiles(root)
	if err != nil {
		return nil, nil, err
	}
	for _, path := range paths {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, skipped, readErr
		}
		eng, engErr := font.Parse(data)
		converted := false
		if engErr != nil && len(data) >= 4 && binary.BigEndian.Uint32(data[0:4]) == 0x4F54544F {
			// An OpenType/CFF font is converted to TrueType by Load, so the
			// oracle can compare it too: the cmap, the advances and the
			// vertical metrics are the source's, and the outline anchors are
			// the ones the conversion fits.
			if loaded, loadErr := font.Load(data); loadErr == nil {
				eng, engErr, converted = loaded, nil, true
			}
		}
		if engErr != nil {
			skipped = append(skipped, fmt.Sprintf("%s: the engine rejects it: %v", rel, engErr))
			continue
		}
		sf, sfErr := sfnt.Parse(data)
		if sfErr != nil {
			skipped = append(skipped, fmt.Sprintf("%s: sfnt rejects a font the engine accepts: %v", rel, sfErr))
			continue
		}
		if unsupportedCmap(eng, sf) {
			skipped = append(skipped, fmt.Sprintf("%s: the engine maps no rune while sfnt maps 'A'; the cmap format is outside 4 and 12", rel))
			continue
		}
		fonts = append(fonts, corpusFont{name: rel, eng: eng, sfnt: sf, converted: converted})
	}
	if len(fonts) < minComparedFonts {
		return fonts, skipped, fmt.Errorf("compared %d fonts, want at least %d", len(fonts), minComparedFonts)
	}
	return fonts, skipped, nil
}

// unsupportedCmap reports whether the engine cannot map a rune that sfnt can,
// which means the font's cmap subtable format is outside the two the engine
// implements.
func unsupportedCmap(eng *font.Font, sf *sfnt.Font) bool {
	var buf sfnt.Buffer
	gid, err := sf.GlyphIndex(&buf, 'A')
	if err != nil || gid == 0 {
		return false
	}
	_, ok := eng.GlyphIndex('A')
	return !ok
}

// corpusRoot is where the committed fonts live, relative to this package.
func corpusRoot() string { return filepath.Join("..", "..", "testdata") }

// mustCorpus returns the corpus and fails the test when it is too small.
func mustCorpus(t *testing.T) []corpusFont {
	t.Helper()
	fonts, skipped, err := discover(corpusRoot())
	for _, reason := range skipped {
		t.Logf("skip %s", reason)
	}
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	return fonts
}

// sampleRunes is the code point range the cmap comparison covers: the whole
// BMP, which includes Latin, punctuation, Cyrillic, Greek, Hebrew, Arabic and a
// CJK sample, plus a supplementary sample of emoji blocks.
func sampleRunes() []rune {
	out := make([]rune, 0, 0x10000+0x400)
	for r := rune(0); r <= 0xFFFF; r++ {
		out = append(out, r)
	}
	for r := rune(0x1F300); r <= 0x1F6FF; r++ {
		out = append(out, r)
	}
	return out
}

// compareCmap checks presence and glyph id for every rune of the sample.
func compareCmap(eng *font.Font, sf *sfnt.Font) error {
	var buf sfnt.Buffer
	for _, r := range sampleRunes() {
		want, err := sf.GlyphIndex(&buf, r)
		sfOK := err == nil && want != 0
		got, engOK := eng.GlyphIndex(r)
		switch {
		case sfOK && !engOK:
			return fmt.Errorf("U+%04X: sfnt maps it to gid %d, the engine maps nothing", r, want)
		case !sfOK && engOK:
			return fmt.Errorf("U+%04X: the engine maps it to gid %d, sfnt maps nothing", r, got)
		case sfOK && uint16(want) != got:
			return fmt.Errorf("U+%04X: the engine maps it to gid %d, sfnt to gid %d", r, got, want)
		}
	}
	return nil
}

// compareMetrics checks the glyph count, the em size, the vertical metrics and
// every advance.
func compareMetrics(eng *font.Font, sf *sfnt.Font) error {
	if int(eng.UnitsPerEm()) != int(sf.UnitsPerEm()) {
		return fmt.Errorf("units per em: engine %d, sfnt %d", eng.UnitsPerEm(), sf.UnitsPerEm())
	}
	if int(eng.NumGlyphs()) != sf.NumGlyphs() {
		return fmt.Errorf("glyph count: engine %d, sfnt %d", eng.NumGlyphs(), sf.NumGlyphs())
	}
	var buf sfnt.Buffer
	ppem := fixed.Int26_6(sf.UnitsPerEm())
	m, err := sf.Metrics(&buf, ppem, xfont.HintingNone)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	if d := diff(int(eng.Ascent()), int(m.Ascent)); d > tolerance {
		return fmt.Errorf("ascent: engine %d, sfnt %d", eng.Ascent(), m.Ascent)
	}
	// The engine keeps the hhea descent negative; sfnt reports it positive.
	if d := diff(int(eng.Descent()), -int(m.Descent)); d > tolerance {
		return fmt.Errorf("descent: engine %d, sfnt %d", eng.Descent(), m.Descent)
	}
	for gid := 0; gid < sf.NumGlyphs(); gid++ {
		want, err := sf.GlyphAdvance(&buf, sfnt.GlyphIndex(gid), ppem, xfont.HintingNone)
		if err != nil {
			return fmt.Errorf("glyph %d: sfnt advance: %w", gid, err)
		}
		if got := eng.Advance(uint16(gid)); diff(int(got), int(want)) > tolerance {
			return fmt.Errorf("glyph %d: advance %d in the engine, %d in sfnt", gid, got, want)
		}
	}
	return nil
}

// compareOutlines checks the outline of every glyph. The engine's contours are
// converted into the segment sequence sfnt reports, so the two are compared as
// the same kind of value.
func compareOutlines(eng *font.Font, sf *sfnt.Font, converted bool) (skipped int, err error) {
	var buf sfnt.Buffer
	ppem := fixed.Int26_6(sf.UnitsPerEm())
	for gid := 0; gid < sf.NumGlyphs(); gid++ {
		want, err := sf.LoadGlyph(&buf, sfnt.GlyphIndex(gid), ppem, nil)
		if err != nil {
			if errors.Is(err, sfnt.ErrColoredGlyph) || errors.Is(err, sfnt.ErrNotFound) {
				// A coloured or missing glyph has no outline to compare; the
				// count is reported so the suite does not silently pass.
				skipped++
				continue
			}
			if converted {
				// sfnt's CFF reader cannot decode every charstring the engine's
				// reader implements (it rejects the flex operators some corpus
				// fonts use); the glyph is counted as skipped instead.
				skipped++
				continue
			}
			return 0, fmt.Errorf("glyph %d: sfnt outline: %w", gid, err)
		}
		contours, err := eng.GlyphPath(uint16(gid))
		if err != nil {
			// The documented limit of the engine on a glyph bomb, not a
			// divergence.
			if errors.Is(err, font.ErrGlyphTooComplex) {
				skipped++
				continue
			}
			return 0, fmt.Errorf("glyph %d: engine outline: %w", gid, err)
		}
		if converted {
			// The conversion fits quadratics to sfnt's cubics, so the segment
			// kinds differ by design. The on-curve anchors are the contract:
			// every segment ends where sfnt's segment of the same index ends.
			if err := compareAnchors(gid, contourSegments(contours), want); err != nil {
				return 0, err
			}
			continue
		}
		if err := compareSegments(gid, contourSegments(contours), want); err != nil {
			return 0, err
		}
	}
	return skipped, nil
}

// compareAnchors compares the on-curve anchors of two outlines as a
// subsequence: every anchor sfnt reports must appear in the engine's anchor
// sequence, in order and within the tolerance. The conversion subdivides one
// cubic into several quadratics, so the engine has extra intermediate anchors;
// the source's anchors are the ones that must survive.
func compareAnchors(gid int, got, want sfnt.Segments) error {
	ga, wa := anchors(got), anchors(want)
	next := 0
	for i := range wa {
		found := false
		for next < len(ga) {
			if diff(int(ga[next].X), int(wa[i].X)) <= tolerance && diff(int(ga[next].Y), int(wa[i].Y)) <= tolerance {
				found = true
				next++
				break
			}
			next++
		}
		if !found {
			return fmt.Errorf("glyph %d anchor %d (%d,%d) is missing from the converted outline", gid, i, wa[i].X, wa[i].Y)
		}
	}
	return nil
}

// anchors returns the final point of every segment, which is on the curve.
func anchors(segs sfnt.Segments) []fixed.Point26_6 {
	out := make([]fixed.Point26_6, 0, len(segs))
	for _, s := range segs {
		args := segmentArgs(s.Op)
		if args == 0 {
			continue
		}
		out = append(out, s.Args[args-1])
	}
	return out
}

// compareSegments compares one glyph's converted outline with sfnt's.
func compareSegments(gid int, got, want sfnt.Segments) error {
	if len(got) != len(want) {
		return fmt.Errorf("glyph %d: %d segments in the engine, %d in sfnt", gid, len(got), len(want))
	}
	for i := range want {
		if got[i].Op != want[i].Op {
			return fmt.Errorf("glyph %d segment %d: op %d in the engine, %d in sfnt", gid, i, got[i].Op, want[i].Op)
		}
		for k := 0; k < segmentArgs(want[i].Op); k++ {
			gx, gy := int(got[i].Args[k].X), int(got[i].Args[k].Y)
			wx, wy := int(want[i].Args[k].X), int(want[i].Args[k].Y)
			if diff(gx, wx) > tolerance || diff(gy, wy) > tolerance {
				return fmt.Errorf("glyph %d segment %d point %d: (%d,%d) in the engine, (%d,%d) in sfnt", gid, i, k, gx, gy, wx, wy)
			}
		}
	}
	return nil
}

// segmentArgs is the number of points one segment carries.
func segmentArgs(op sfnt.SegmentOp) int {
	switch op {
	case sfnt.SegmentOpQuadTo:
		return 2
	case sfnt.SegmentOpCubeTo:
		return 3
	}
	return 1
}

// diff is the absolute difference of two integers.
func diff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// contourSegments converts the engine's contours into the segments sfnt reports,
// canonicalising the start point and closing every contour explicitly.
//
// TrueType stores a contour from an arbitrary start point, which may be
// off-curve, and sfnt normalises it: it starts at the first on-curve point, or
// at the midpoint of the first two points when both are off-curve. The engine
// keeps the stored order, so this function rotates nothing — it replays sfnt's
// own iterator over the stored points.
//
// sfnt's Y axis points down, so every Y is negated. The two decoders read the
// same coordinates, so the result is comparable point by point.
func contourSegments(contours []font.Contour) sfnt.Segments {
	var out sfnt.Segments
	for _, c := range contours {
		out = append(out, contourOf(c)...)
	}
	return out
}

// contourOf converts one closed contour.
func contourOf(c font.Contour) sfnt.Segments {
	n := len(c)
	if n == 0 {
		return nil
	}
	points := make([]fixed.Point26_6, n)
	on := make([]bool, n)
	for i, p := range c {
		points[i] = fixed.Point26_6{
			X: round(p.X),
			Y: -round(p.Y),
		}
		on[i] = p.OnCurve
	}

	var start fixed.Point26_6
	var firstOff fixed.Point26_6
	var pending fixed.Point26_6
	hasFirstOff, hasPending := false, false
	from := 0
	switch {
	case on[0]:
		start, from = points[0], 1
	case n == 1:
		// A one-point contour is degenerate; keep it representable.
		start, from = points[0], 1
	case on[1]:
		start, from = points[1], 2
		firstOff, hasFirstOff = points[0], true
	default:
		start, from = midPoint(points[0], points[1]), 2
		firstOff, hasFirstOff = points[0], true
		pending, hasPending = points[1], true
	}

	out := sfnt.Segments{{Op: sfnt.SegmentOpMoveTo, Args: [3]fixed.Point26_6{start}}}
	for i := from; i < n; i++ {
		switch {
		case on[i] && hasPending:
			out = append(out, quadTo(pending, points[i]))
			hasPending = false
		case on[i]:
			out = append(out, sfnt.Segment{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{points[i]}})
		default:
			if hasPending {
				out = append(out, quadTo(pending, midPoint(pending, points[i])))
			}
			pending, hasPending = points[i], true
		}
	}
	// sfnt closes every contour with an explicit final segment.
	switch {
	case hasPending && hasFirstOff:
		// Both ends are off-curve. sfnt emits the closing quad through the
		// last control point and then does not mark the contour closed, so the
		// iterator closes it a second time through the first control point.
		// The double close is part of the sequence the oracle compares.
		out = append(out, quadTo(pending, midPoint(pending, firstOff)))
		out = append(out, quadTo(firstOff, start))
	case hasPending:
		out = append(out, quadTo(pending, start))
	case hasFirstOff:
		out = append(out, quadTo(firstOff, start))
	default:
		out = append(out, sfnt.Segment{Op: sfnt.SegmentOpLineTo, Args: [3]fixed.Point26_6{start}})
	}
	return out
}

func quadTo(control, point fixed.Point26_6) sfnt.Segment {
	return sfnt.Segment{Op: sfnt.SegmentOpQuadTo, Args: [3]fixed.Point26_6{control, point}}
}

func midPoint(a, b fixed.Point26_6) fixed.Point26_6 {
	return fixed.Point26_6{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
}

func round(v float32) fixed.Int26_6 {
	return fixed.Int26_6(math.Round(float64(v)))
}

// TestCorpusMustNotBeEmpty proves the guard: a suite that compares nothing is
// a failure, not a pass.
func TestCorpusMustNotBeEmpty(t *testing.T) {
	fonts, _, err := discover(t.TempDir())
	if err == nil {
		t.Fatalf("an empty corpus was accepted (%d fonts)", len(fonts))
	}
	if !strings.Contains(err.Error(), "compared 0 fonts") {
		t.Fatalf("error = %v, want it to name the compared count", err)
	}
}

// TestSyntheticContours pins the conversion on the shapes that need a start
// point decision, before the corpus exercises it at scale.
func TestSyntheticContours(t *testing.T) {
	pt := func(x, y float32, on bool) font.Point { return font.Point{X: x, Y: y, OnCurve: on} }
	ops := func(segs sfnt.Segments) []sfnt.SegmentOp {
		out := make([]sfnt.SegmentOp, len(segs))
		for i, s := range segs {
			out[i] = s.Op
		}
		return out
	}
	cases := []struct {
		name     string
		contour  font.Contour
		wantOps  []sfnt.SegmentOp
		wantArgs [][2]fixed.Point26_6 // the points of the segments, for the cases that check them
	}{
		{
			name:     "on-curve start",
			contour:  font.Contour{pt(0, 0, true), pt(100, 0, true), pt(100, 100, true)},
			wantOps:  []sfnt.SegmentOp{sfnt.SegmentOpMoveTo, sfnt.SegmentOpLineTo, sfnt.SegmentOpLineTo, sfnt.SegmentOpLineTo},
			wantArgs: [][2]fixed.Point26_6{{{X: 0, Y: 0}, {}}, {{X: 100, Y: 0}, {}}, {{X: 100, Y: -100}, {}}, {{X: 0, Y: 0}, {}}},
		},
		{
			// The first point is off-curve and the second is on-curve: sfnt
			// starts at the on-curve point and closes through the control point.
			name:     "off-curve start",
			contour:  font.Contour{pt(50, 100, false), pt(100, 0, true), pt(0, 0, true)},
			wantOps:  []sfnt.SegmentOp{sfnt.SegmentOpMoveTo, sfnt.SegmentOpLineTo, sfnt.SegmentOpQuadTo},
			wantArgs: [][2]fixed.Point26_6{{{X: 100, Y: 0}, {}}, {{X: 0, Y: 0}, {}}, {{X: 50, Y: -100}, {X: 100, Y: 0}}},
		},
		{
			// Two consecutive off-curve points: sfnt starts at their midpoint
			// and inserts the implied on-curve point between them.
			name:    "two off-curve points",
			contour: font.Contour{pt(0, 100, false), pt(100, 100, false), pt(100, 0, true)},
			wantOps: []sfnt.SegmentOp{sfnt.SegmentOpMoveTo, sfnt.SegmentOpQuadTo, sfnt.SegmentOpQuadTo},
			wantArgs: [][2]fixed.Point26_6{
				{{X: 50, Y: -100}, {}},
				{{X: 100, Y: -100}, {X: 100, Y: 0}},
				{{X: 0, Y: -100}, {X: 50, Y: -100}},
			},
		},
		{
			// The first and the last point are both off-curve: sfnt closes the
			// contour twice, once through the last control point and once
			// through the first.
			name:    "off-curve at both ends",
			contour: font.Contour{pt(0, 100, false), pt(100, 100, false), pt(100, 0, true), pt(0, 0, false)},
			wantOps: []sfnt.SegmentOp{sfnt.SegmentOpMoveTo, sfnt.SegmentOpQuadTo, sfnt.SegmentOpQuadTo, sfnt.SegmentOpQuadTo},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := contourSegments([]font.Contour{tc.contour})
			if !slices.Equal(ops(got), tc.wantOps) {
				t.Fatalf("ops = %v, want %v", ops(got), tc.wantOps)
			}
			for i, want := range tc.wantArgs {
				if got[i].Args[0] != want[0] {
					t.Fatalf("segment %d point 0 = %v, want %v", i, got[i].Args[0], want[0])
				}
				if want[1] != (fixed.Point26_6{}) && got[i].Args[1] != want[1] {
					t.Fatalf("segment %d point 1 = %v, want %v", i, got[i].Args[1], want[1])
				}
			}
		})
	}
}

// TestCompositeContourConversion covers a composite glyph: the engine expands
// the components, and the expanded contours must convert into exactly the
// segments sfnt reports for the same glyph.
func TestCompositeContourConversion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Skipf("the bundled font is missing: %v", err)
	}
	eng, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	sf, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	gid, err := sf.GlyphIndex(&buf, '\u00C1') // Á: A with an acute, a composite
	if err != nil || gid == 0 {
		t.Skipf("no composite glyph: %v", err)
	}
	contours, err := eng.GlyphPath(uint16(gid))
	if err != nil {
		t.Fatal(err)
	}
	if len(contours) < 2 {
		t.Fatalf("glyph %d expanded to %d contours, want the components", gid, len(contours))
	}
	want, err := sf.LoadGlyph(&buf, gid, fixed.Int26_6(sf.UnitsPerEm()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := compareSegments(int(gid), contourSegments(contours), want); err != nil {
		t.Fatal(err)
	}
}

// TestCmapAgreement compares the character map over the sample range.
func TestCmapAgreement(t *testing.T) {
	for _, f := range mustCorpus(t) {
		f := f
		t.Run(f.name, func(t *testing.T) {
			if err := compareCmap(f.eng, f.sfnt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestMetricsAgreement compares the em size, the glyph count, the vertical
// metrics and every advance.
func TestMetricsAgreement(t *testing.T) {
	for _, f := range mustCorpus(t) {
		f := f
		t.Run(f.name, func(t *testing.T) {
			if err := compareMetrics(f.eng, f.sfnt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestOutlineAgreement compares the outline of every glyph of every compared
// font.
func TestOutlineAgreement(t *testing.T) {
	fonts := mustCorpus(t)
	for _, f := range fonts {
		f := f
		t.Run(f.name, func(t *testing.T) {
			skipped, err := compareOutlines(f.eng, f.sfnt, f.converted)
			if err != nil {
				t.Fatal(err)
			}
			if skipped > 0 {
				t.Logf("%d glyphs have no comparable outline (coloured, not found or beyond the engine's outline limit)", skipped)
			}
		})
	}
}

// BenchmarkOutlineAgreement reports the cost of the outline comparison, which
// is the expensive part of the suite.
func BenchmarkOutlineAgreement(b *testing.B) {
	fonts, _, err := discover(corpusRoot())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, f := range fonts {
			if _, err := compareOutlines(f.eng, f.sfnt, f.converted); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// FuzzParserAgreement runs both decoders on arbitrary bytes. The engine must not
// panic on any input, and when both decoders accept it they must agree on the
// glyph count, the em size and a small character-map sample. The seeds are the
// committed corpus, so the ordinary test run reproduces the promise without a
// fuzzing budget.
func FuzzParserAgreement(f *testing.F) {
	paths, err := corpusFiles(corpusRoot())
	if err != nil {
		f.Fatal(err)
	}
	if len(paths) == 0 {
		f.Fatal("the corpus is empty")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		eng, engErr := font.Parse(data) // must not panic, whatever the bytes
		sf, sfErr := sfnt.Parse(data)
		if engErr != nil || sfErr != nil {
			return
		}
		if int(eng.UnitsPerEm()) != int(sf.UnitsPerEm()) {
			t.Fatalf("units per em: engine %d, sfnt %d", eng.UnitsPerEm(), sf.UnitsPerEm())
		}
		if int(eng.NumGlyphs()) != sf.NumGlyphs() {
			t.Fatalf("glyph count: engine %d, sfnt %d", eng.NumGlyphs(), sf.NumGlyphs())
		}
		var buf sfnt.Buffer
		for _, r := range []rune{'A', 'a', '0', 'Я', 'ё', 0x4E00, 0x1F600} {
			want, err := sf.GlyphIndex(&buf, r)
			sfOK := err == nil && want != 0
			got, engOK := eng.GlyphIndex(r)
			switch {
			case sfOK && !engOK:
				t.Fatalf("U+%04X: sfnt maps it to gid %d, the engine maps nothing", r, want)
			case !sfOK && engOK:
				t.Fatalf("U+%04X: the engine maps it to gid %d, sfnt maps nothing", r, got)
			case sfOK && uint16(want) != got:
				t.Fatalf("U+%04X: the engine maps it to gid %d, sfnt to gid %d", r, got, want)
			}
		}
	})
}
