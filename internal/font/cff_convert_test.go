package font

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// TestLoadAcceptsOpenType checks the entry point on the corpus fonts: the
// converted font has the same numbering, metrics and family as the source, and
// the outlines of the glyphs sfnt cannot read are present.
func TestLoadAcceptsOpenType(t *testing.T) {
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			f, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			sf, err := sfnt.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if int(f.NumGlyphs()) != sf.NumGlyphs() {
				t.Fatalf("%d glyphs, the source has %d", f.NumGlyphs(), sf.NumGlyphs())
			}
			if int(f.UnitsPerEm()) != int(sf.UnitsPerEm()) {
				t.Fatalf("unitsPerEm %d, the source has %d", f.UnitsPerEm(), sf.UnitsPerEm())
			}
			var buf sfnt.Buffer
			ppem := fixed.Int26_6(sf.UnitsPerEm())
			for r := rune(0x20); r < 0x7f; r++ {
				want, err := sf.GlyphIndex(&buf, r)
				if err != nil {
					t.Fatalf("%q: %v", r, err)
				}
				got, ok := f.GlyphIndex(r)
				if !ok || uint16(want) != got {
					t.Fatalf("%q maps to %d, the source says %d", r, got, want)
				}
				adv, err := sf.GlyphAdvance(&buf, want, ppem, 0)
				if err != nil {
					t.Fatalf("%q advance: %v", r, err)
				}
				if int32(adv) != f.Advance(got) {
					t.Fatalf("%q advance %d, the source says %d", r, f.Advance(got), adv)
				}
				if _, err := f.GlyphPath(got); err != nil {
					t.Fatalf("%q outline: %v", r, err)
				}
			}
			if f.Family() == "" {
				t.Fatal("the converted font has no family name")
			}
		})
	}
}

// TestConvertedFontKeepsTables proves the numbering is preserved by copying the
// tables that depend on it: cmap, GPOS, GDEF, hmtx and post come out of the
// converter byte for byte, so a glyph id, an advance, a kerning pair or a mark
// class cannot shift. The table set is exact: GDEF appears only when the source
// declares it.
func TestConvertedFontKeepsTables(t *testing.T) {
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			tables, err := tableDirectory(data)
			if err != nil {
				t.Fatal(err)
			}
			converted, err := convertCFF(data, tables, nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := tableDirectory(converted)
			if err != nil {
				t.Fatal(err)
			}
			for _, tag := range []string{"cmap", "GPOS", "GDEF", "hmtx", "post", "name", "OS/2", "hhea"} {
				if !bytes.Equal(out[tag], tables[tag]) {
					t.Fatalf("the %q table changed during the conversion", tag)
				}
			}
			// The corpus fonts declare GDEF, so the exact converted set
			// includes it plus the tables this converter writes.
			want := []string{
				"GDEF", "GPOS", "OS/2", "cmap", "glyf", "head", "hhea",
				"hmtx", "loca", "maxp", "name", "post",
			}
			got := make([]string, 0, len(out))
			for tag := range out {
				got = append(got, tag)
			}
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("converted tables = %v, want %v", got, want)
			}
			f, err := Parse(converted)
			if err != nil {
				t.Fatal(err)
			}
			// Kerning still answers: the family has a GPOS pair for "AV" or at
			// least for a pair of glyphs, and a dropped table would return 0
			// for every pair.
			nonzero := 0
			for left := uint16(0); left < f.NumGlyphs(); left++ {
				for right := uint16('A'); right < 'Z'; right++ {
					if f.Kerning(left, right) != 0 {
						nonzero++
					}
				}
			}
			if nonzero == 0 {
				t.Fatal("no kerning pair survived the conversion")
			}
		})
	}
}

// TestConvertedCFFKeepsGDEFMarkClasses guards RV5-32 on the corpus: the GDEF
// table comes out byte for byte, so the mark classes that IgnoreMarks and the
// mark filtering sets read survive the conversion.
func TestConvertedCFFKeepsGDEFMarkClasses(t *testing.T) {
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			tables, err := tableDirectory(data)
			if err != nil {
				t.Fatal(err)
			}
			sourceMarks, _ := parseGDEF(tables["GDEF"])
			if len(sourceMarks) == 0 {
				t.Fatal("the fixture declares GDEF but no mark glyphs")
			}
			converted, err := convertCFF(data, tables, nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := tableDirectory(converted)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out["GDEF"], tables["GDEF"]) {
				t.Fatal("the GDEF table changed during the conversion")
			}
			f, err := Parse(converted)
			if err != nil {
				t.Fatal(err)
			}
			if f.gpos == nil {
				t.Fatal("the converted font has no GPOS")
			}
			for gid := range sourceMarks {
				if !f.gpos.markGlyphs[gid] {
					t.Fatalf("glyph %d lost its mark class in the conversion", gid)
				}
			}
		})
	}
}

// TestCFFConversionKeepsIgnoreMarksBehavior guards the behaviour half of
// RV5-32 with a crafted CFF font: with GDEF, IgnoreMarks skips the mark, and
// without GDEF the same pair would apply.
func TestCFFConversionKeepsIgnoreMarksBehavior(t *testing.T) {
	charstring := new(charstringBuilder).num(0, 0).op(21).op(14).bytes()
	gpos := buildGPOS(
		[]gposScript{{tag: "DFLT", features: []int{0}}},
		[]gposFeature{{tag: "kern", lookups: []int{0}}},
		[][]byte{pairLookupTable(pairPosFormat1Test(0, 0, -100), 0x0008, 0)},
	)
	gdef := buildGDEF([]uint16{0}, nil)
	tables := map[string][]byte{
		"CFF ": testCFFTable(charstring),
		"maxp": []byte{0, 0, 0x50, 0x00, 0, 1}, // version 0.5, one glyph
		"head": testHead(),
		"hhea": testHhea(),
		"hmtx": []byte{0x01, 0xf4, 0, 0},
		"GDEF": gdef,
		"GPOS": gpos,
	}
	data := assembleSFNT(0x4F54544F, tables)
	converted, err := convertCFF(data, tables, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tableDirectory(converted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out["GDEF"], gdef) {
		t.Fatal("the converted font does not carry the source GDEF")
	}
	f, err := Parse(converted)
	if err != nil {
		t.Fatal(err)
	}
	if !f.gpos.markGlyphs[0] {
		t.Fatal("glyph 0 is not a mark after the conversion")
	}
	if got := f.Kerning(0, 0); got != 0 {
		t.Fatalf("IgnoreMarks applied the pair to a mark: %d", got)
	}

	// The same source without GDEF cannot know the glyph is a mark, so the
	// pair applies. This is the behaviour the copied table prevents.
	delete(tables, "GDEF")
	withoutGDEF, err := convertCFF(data, tables, nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(withoutGDEF)
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Kerning(0, 0); got != -100 {
		t.Fatalf("without GDEF the pair = %d, want -100", got)
	}
}

// TestConvertedCFFWithoutGDEFKeepsTheOldTableSet guards the conditional half of
// RV5-32: a source without GDEF converts with the table set the converter wrote
// before the change.
func TestConvertedCFFWithoutGDEFKeepsTheOldTableSet(t *testing.T) {
	charstring := new(charstringBuilder).num(0, 0).op(21).op(14).bytes()
	data := testCFFFont(charstring)
	tables, err := tableDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tables["GDEF"]; ok {
		t.Fatal("the crafted font declares GDEF")
	}
	converted, err := convertCFF(data, tables, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tableDirectory(converted)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(out))
	for tag := range out {
		got = append(got, tag)
	}
	slices.Sort(got)
	want := []string{"glyf", "head", "hhea", "hmtx", "loca", "maxp"}
	if !slices.Equal(got, want) {
		t.Fatalf("converted tables = %v, want %v", got, want)
	}
}

// TestConvertCFFDeterministic proves that the same bytes convert to the same
// font: no map iteration order, address or random value may reach the output.
func TestConvertCFFDeterministic(t *testing.T) {
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			tables, err := tableDirectory(data)
			if err != nil {
				t.Fatal(err)
			}
			first, err := convertCFF(data, tables, nil)
			if err != nil {
				t.Fatal(err)
			}
			second, err := convertCFF(data, tables, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Fatal("two conversions of the same font differ")
			}
		})
	}
}

// TestCFFSubdivisionTolerance checks the conversion's promise where it is made:
// every quadratic fitted to a cubic stays within outlineFitTolerance of it. The
// comparison is parametric, which is what the fit bounds: the quadratic for
// piece j of n is compared with the cubic at the same fraction of the interval.
func TestCFFSubdivisionTolerance(t *testing.T) {
	// A straight line and a quarter circle of radius 500 in 26.6 units, with the
	// usual control-point ratio, plus two corpus glyphs.
	const k = 27614 // 0.5522847498 * 500 in 26.6 units
	straight := []cffPoint{{}, {10 << 6, 10 << 6}, {20 << 6, 20 << 6}, {30 << 6, 30 << 6}}
	arc := []cffPoint{{500 << 6, 0}, {500 << 6, k}, {k, 500 << 6}, {0, 500 << 6}}

	tests := []struct {
		name  string
		curve []cffPoint
	}{
		{name: "straight", curve: straight},
		{name: "circular arc", curve: arc},
	}
	for _, name := range cffCorpusFonts {
		data := readCorpusFont(t, name)
		cff := cffOf(t, data)
		for _, r := range []rune{'А', 'g', '8'} {
			glyph, ok := glyphForRune(t, data, r)
			if !ok {
				continue
			}
			segments, err := cff.glyphOutline(glyph)
			if err != nil {
				t.Fatalf("%s %q: %v", name, r, err)
			}
			for i, segment := range segments {
				if segment.Op != cffCubeTo {
					continue
				}
				start := previousPoint(segments, i)
				tests = append(tests, struct {
					name  string
					curve []cffPoint
				}{name: fmt.Sprintf("%s %q cubic %d", name, r, i), curve: []cffPoint{start, segment.Args[0], segment.Args[1], segment.Args[2]}})
			}
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			quads := cubicQuadratics(tc.curve[0], tc.curve[1], tc.curve[2], tc.curve[3])
			if d := quadraticDeviation(tc.curve, quads); d > float64(outlineFitTolerance)/64 {
				t.Fatalf("the fitted quadratics are %.4f font units from the cubic, the tolerance is %.4f",
					d, float64(outlineFitTolerance)/64)
			}
			if len(quads) == 0 {
				t.Fatal("no quadratics were fitted")
			}
		})
	}
}

// previousPoint returns the point the segment at index i starts at.
func previousPoint(segments []cffSegment, i int) cffPoint {
	for j := i - 1; j >= 0; j-- {
		switch segments[j].Op {
		case cffMoveTo, cffLineTo:
			return segments[j].Args[0]
		case cffCubeTo:
			return segments[j].Args[2]
		}
	}
	return cffPoint{}
}

// quadraticDeviation is the greatest distance between a cubic and the
// quadratics fitted to it, in font units, measured with both curves at the same
// parameter.
func quadraticDeviation(curve []cffPoint, quads []quadratic) float64 {
	p0, p1, p2, p3 := curve[0], curve[1], curve[2], curve[3]
	worst := 0.0
	n := float64(len(quads))
	const samples = 16
	for j, q := range quads {
		start := float64(j) / n
		end := float64(j+1) / n
		pieceStart := cubicPointF(p0, p1, p2, p3, start)
		control, quadEnd := toPoint26(q.ctrl), toPoint26(q.end)
		for k := 0; k <= samples; k++ {
			s := float64(k) / samples
			qp := quadraticPoint(pieceStart, control, quadEnd, s)
			cp := cubicPointF(p0, p1, p2, p3, start+s*(end-start))
			d := math.Hypot(qp.x-cp.x, qp.y-cp.y)
			if d > worst {
				worst = d
			}
		}
	}
	return worst
}

// quadraticPoint is the point at t of the quadratic through a, control and b.
func quadraticPoint(a, control, b point26, t float64) point26 {
	u := 1 - t
	return point26{
		x: u*u*a.x + 2*u*t*control.x + t*t*b.x,
		y: u*u*a.y + 2*u*t*control.y + t*t*b.y,
	}
}

// toPoint26 converts a 26.6 point into font units.
func toPoint26(p cffPoint) point26 {
	return point26{float64(p.X) / 64, float64(p.Y) / 64}
}

// cubicPointF is cubicPoint over 26.6 control points.
func cubicPointF(p0, p1, p2, p3 cffPoint, t float64) point26 {
	return cubicPoint(toPoint26(p0), toPoint26(p1), toPoint26(p2), toPoint26(p3), t)
}

// TestConvertedPointsAreRoundedOnce checks the encoder: every stored point is
// the converter's own pre-quantization point rounded to the nearest font unit,
// in the same order and in the same contours. It is what keeps the stored
// outline within one unit of a curve that is itself within a quarter unit.
func TestConvertedPointsAreRoundedOnce(t *testing.T) {
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			tables, err := tableDirectory(data)
			if err != nil {
				t.Fatal(err)
			}
			converted, err := convertCFF(data, tables, nil)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(converted)
			if err != nil {
				t.Fatal(err)
			}
			cff := cffOf(t, data)
			for gid := uint16(0); gid < f.NumGlyphs(); gid++ {
				segments, err := cff.glyphOutline(gid)
				if err != nil {
					t.Fatalf("glyph %d: %v", gid, err)
				}
				ideal, err := cffContours(segments, gid)
				if err != nil {
					t.Fatalf("glyph %d: %v", gid, err)
				}
				stored, err := f.GlyphPath(gid)
				if err != nil {
					t.Fatalf("glyph %d: %v", gid, err)
				}
				if len(stored) != len(ideal) {
					t.Fatalf("glyph %d: %d contours stored, the converter produced %d", gid, len(stored), len(ideal))
				}
				for ci := range ideal {
					if len(stored[ci]) != len(ideal[ci]) {
						t.Fatalf("glyph %d contour %d: %d points stored, the converter produced %d", gid, ci, len(stored[ci]), len(ideal[ci]))
					}
					for pi := range ideal[ci] {
						want := ideal[ci][pi]
						got := stored[ci][pi]
						wx := divRound(int64(want.x), 64)
						wy := divRound(int64(want.y), 64)
						if int32(got.X) != wx || int32(got.Y) != wy {
							t.Fatalf("glyph %d contour %d point %d: stored (%v,%v), the converter says (%v,%v)",
								gid, ci, pi, got.X, got.Y, wx, wy)
						}
						if got.OnCurve != want.onCurve {
							t.Fatalf("glyph %d contour %d point %d: on-curve flag changed", gid, ci, pi)
						}
					}
				}
			}
		})
	}
}

// TestConvertedOutlinesMatchSourceGeometrically measures the stored outline
// against the source curve for a handful of glyphs per face, including the two
// whose source uses flex1. The source is sampled densely, so the polyline error
// is far below the one-unit promise.
func TestConvertedOutlinesMatchSourceGeometrically(t *testing.T) {
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			tables, err := tableDirectory(data)
			if err != nil {
				t.Fatal(err)
			}
			converted, err := convertCFF(data, tables, nil)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(converted)
			if err != nil {
				t.Fatal(err)
			}
			cff := cffOf(t, data)
			worst := 0.0
			for _, r := range []rune{'A', 'g', '8', 'r', 'O'} {
				gid, ok := glyphForRune(t, data, r)
				if !ok {
					continue
				}
				source, err := cff.glyphOutline(gid)
				if err != nil {
					t.Fatalf("%q: %v", r, err)
				}
				stored, err := f.GlyphPath(gid)
				if err != nil {
					t.Fatalf("%q: %v", r, err)
				}
				want := segmentsToContours(source)
				if len(stored) != len(want) {
					t.Fatalf("%q: %d contours, the source has %d", r, len(stored), len(want))
				}
				for ci := range want {
					d := contourDistance(stored[ci], want[ci])
					if d > worst {
						worst = d
					}
					if d > 1 {
						t.Fatalf("%q contour %d: a stored point is %.3f font units from the source curve", r, ci, d)
					}
				}
			}
			t.Logf("%s: the worst stored point is %.3f font units from the source", name, worst)
		})
	}
}

// glyphForRune maps a rune to its glyph through sfnt, which reads the cmap of
// the source font.
func glyphForRune(t *testing.T, data []byte, r rune) (uint16, bool) {
	t.Helper()
	sf, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	gid, err := sf.GlyphIndex(&buf, r)
	if err != nil || gid == 0 {
		return 0, false
	}
	return uint16(gid), true
}

// point26 is a point in font units, used by the geometric comparison.
type point26 struct{ x, y float64 }

// contourDistance is the greatest distance from a stored contour to the source
// contour, both in font units.
func contourDistance(stored Contour, source []outlinePoint) float64 {
	const samples = 24
	sourcePoints := sampleSource(source, samples)
	worst := 0.0
	for _, p := range sampleConverted(stored, samples) {
		if d := distanceToPath(sourcePoints, p); d > worst {
			worst = d
		}
	}
	return worst
}

// sampleConverted returns points along a TrueType contour, applying the implied
// on-curve points the format allows between two off-curve points.
func sampleConverted(c Contour, samples int) []point26 {
	if len(c) == 0 {
		return nil
	}
	type mark struct {
		p  point26
		on bool
	}
	points := make([]mark, 0, len(c)*2)
	for i, p := range c {
		next := c[(i+1)%len(c)]
		points = append(points, mark{point26{float64(p.X), float64(p.Y)}, p.OnCurve})
		if !p.OnCurve && !next.OnCurve {
			points = append(points, mark{point26{(float64(p.X) + float64(next.X)) / 2, (float64(p.Y) + float64(next.Y)) / 2}, true})
		}
	}
	start := -1
	for i := range points {
		if points[i].on {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	out := []point26{points[start].p}
	current, control := points[start].p, points[start].p
	haveControl := false
	for i, step := (start+1)%len(points), 0; i != start; step, i = step+1, (i+1)%len(points) {
		if step > len(points) {
			break
		}
		p := points[i]
		if !p.on {
			control, haveControl = p.p, true
			continue
		}
		for k := 1; k <= samples; k++ {
			t := float64(k) / float64(samples)
			if haveControl {
				u := 1 - t
				out = append(out, point26{
					x: u*u*current.x + 2*u*t*control.x + t*t*p.p.x,
					y: u*u*current.y + 2*u*t*control.y + t*t*p.p.y,
				})
				continue
			}
			out = append(out, point26{current.x + (p.p.x-current.x)*t, current.y + (p.p.y-current.y)*t})
		}
		current, control, haveControl = p.p, p.p, false
	}
	return out
}

// sampleSource flattens the source contour into a polyline in font units.
func sampleSource(contour []outlinePoint, samples int) []point26 {
	var out []point26
	at := func(p outlinePoint) point26 { return point26{float64(p.x) / 64, float64(p.y) / 64} }
	var current point26
	for i := 0; i < len(contour); i++ {
		p := contour[i]
		switch p.op {
		case cffMoveTo:
			current = at(p)
			out = append(out, current)
		case cffLineTo:
			end := at(p)
			for k := 1; k <= samples; k++ {
				t := float64(k) / float64(samples)
				out = append(out, point26{current.x + (end.x-current.x)*t, current.y + (end.y-current.y)*t})
			}
			current = end
		case cffCubeTo:
			p1, p2, p3 := at(p), at(contour[i+1]), at(contour[i+2])
			i += 2
			for k := 1; k <= samples; k++ {
				out = append(out, cubicPoint(current, p1, p2, p3, float64(k)/float64(samples)))
			}
			current = p3
		}
	}
	return out
}

func cubicPoint(p0, p1, p2, p3 point26, t float64) point26 {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return point26{a*p0.x + b*p1.x + c*p2.x + d*p3.x, a*p0.y + b*p1.y + c*p2.y + d*p3.y}
}

// distanceToPath is the distance from a point to the polyline through points.
func distanceToPath(points []point26, p point26) float64 {
	best := math.Inf(1)
	if len(points) == 1 {
		return math.Hypot(points[0].x-p.x, points[0].y-p.y)
	}
	for i := 1; i < len(points); i++ {
		if d := distanceToSegment(points[i-1], points[i], p); d < best {
			best = d
		}
	}
	return best
}

// distanceToSegment is the distance from p to the segment a-b.
func distanceToSegment(a, b, p point26) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	if dx == 0 && dy == 0 {
		return math.Hypot(p.x-a.x, p.y-a.y)
	}
	t := ((p.x-a.x)*dx + (p.y-a.y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p.x-(a.x+t*dx), p.y-(a.y+t*dy))
}

// TestSubsetOfConvertedFont checks the subsetter on a converted font: the used
// glyphs keep their outlines and their ids, and every unused slot is empty. The
// text includes "8", whose source charstring uses the flex1 operator, so the
// outline being subset is the one this change made possible.
func TestSubsetOfConvertedFont(t *testing.T) {
	data := readCorpusFont(t, "barlowcondensed-bold.otf")
	tables, err := tableDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	converted, err := convertCFF(data, tables, nil)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(converted)
	if err != nil {
		t.Fatal(err)
	}
	used := map[uint16]bool{}
	var gids []uint16
	for _, r := range "B8 r128" {
		if r == ' ' {
			continue
		}
		gid, ok := f.GlyphIndex(r)
		if !ok {
			t.Fatalf("%q is not mapped", r)
		}
		if !used[gid] {
			gids = append(gids, gid)
		}
		used[gid] = true
	}
	// Subset always keeps glyph 0, the .notdef that an embedded font needs.
	used[0] = true
	subset, err := f.Subset(used)
	if err != nil {
		t.Fatal(err)
	}
	if len(subset) >= len(converted) {
		t.Fatalf("the subset (%d bytes) is not smaller than the converted font (%d)", len(subset), len(converted))
	}
	sub, err := Parse(subset)
	if err != nil {
		t.Fatalf("the subset does not parse: %v", err)
	}
	if sub.NumGlyphs() != f.NumGlyphs() {
		t.Fatalf("the subset has %d glyphs, the converted font %d", sub.NumGlyphs(), f.NumGlyphs())
	}
	for _, gid := range gids {
		want, err := f.GlyphPath(gid)
		if err != nil {
			t.Fatalf("glyph %d: %v", gid, err)
		}
		got, err := sub.GlyphPath(gid)
		if err != nil {
			t.Fatalf("glyph %d: %v", gid, err)
		}
		if len(got) == 0 {
			t.Fatalf("glyph %d has no outline in the subset", gid)
		}
		if len(got) != len(want) {
			t.Fatalf("glyph %d: %d contours in the subset, %d in the converted font", gid, len(got), len(want))
		}
		for ci := range want {
			if len(got[ci]) != len(want[ci]) {
				t.Fatalf("glyph %d contour %d: %d points, want %d", gid, ci, len(got[ci]), len(want[ci]))
			}
			for pi := range want[ci] {
				if got[ci][pi] != want[ci][pi] {
					t.Fatalf("glyph %d contour %d point %d: %v, want %v", gid, ci, pi, got[ci][pi], want[ci][pi])
				}
			}
		}
		if sub.Advance(gid) != f.Advance(gid) {
			t.Fatalf("glyph %d advance %d, want %d", gid, sub.Advance(gid), f.Advance(gid))
		}
	}
	for gid := uint16(0); gid < f.NumGlyphs(); gid++ {
		if used[gid] {
			continue
		}
		got, err := sub.GlyphPath(gid)
		if err != nil {
			t.Fatalf("glyph %d: %v", gid, err)
		}
		if len(got) != 0 {
			t.Fatalf("unused glyph %d kept %d contours in the subset", gid, len(got))
		}
	}
}

// TestConvertedGlyfLimitFourTimes checks the first output bound: a converted
// glyf that grows beyond four times the input is refused with ErrLimit.
func TestConvertedGlyfLimitFourTimes(t *testing.T) {
	// A cubic whose control points are far apart subdivides until each piece is
	// within the fit tolerance, so a few hundred bytes of charstring can produce
	// thousands of points.
	fontData, charstring := testCFFFontWithHugeCurves(t, 8)
	tables, err := tableDirectory(fontData)
	if err != nil {
		t.Fatal(err)
	}
	_, err = convertCFF(fontData, tables, nil)
	if err == nil {
		t.Fatalf("the conversion of a %d-byte font with %d charstring bytes was accepted", len(fontData), len(charstring))
	}
	if !isLimitError(err) {
		t.Fatalf("got %v, want a limit error", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("glyf")) {
		t.Fatalf("the diagnostic does not name the bound: %v", err)
	}
}

// TestConvertedGlyfLimitSixtyFourMiB checks the absolute half of the bound: the
// limit is the smaller of four times the input and 64 MiB.
func TestConvertedGlyfLimitSixtyFourMiB(t *testing.T) {
	if got := convertedGlyfLimit(1024); got != convertedGlyfFactor*1024 {
		t.Fatalf("the limit for a small font is %d, want %d", got, convertedGlyfFactor*1024)
	}
	if got := convertedGlyfLimit(1 << 30); got != maxConvertedGlyf {
		t.Fatalf("the limit for a large font is %d, want %d", got, maxConvertedGlyf)
	}
}

// TestConvertedGlyphPointCap checks the per-glyph bound: one glyph may not grow
// past maxGlyphPoints.
func TestConvertedGlyphPointCap(t *testing.T) {
	fontData, _ := testCFFFontWithHugeCurves(t, 40)
	tables, err := tableDirectory(fontData)
	if err != nil {
		t.Fatal(err)
	}
	_, err = convertCFF(fontData, tables, nil)
	if err == nil {
		t.Fatal("a glyph beyond the point cap was accepted")
	}
	if !isLimitError(err) {
		t.Fatalf("got %v, want a limit error", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("glyph 0")) {
		t.Fatalf("the diagnostic does not name the glyph: %v", err)
	}
}

// isLimitError reports whether err is the documented conversion limit.
func isLimitError(err error) bool {
	return err != nil && bytes.Contains([]byte(err.Error()), []byte("conversion limit exceeded"))
}

// TestLoadRejectsFormats checks the formats the loader names instead of
// decoding.
func TestLoadRejectsFormats(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "WOFF", data: withVersion(0x774F4646), want: "WOFF"},
		{name: "WOFF2", data: withVersion(0x774F4632), want: "WOFF2"},
		{name: "collection", data: withVersion(0x74746366), want: "collection"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(tc.data)
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte(tc.want)) {
				t.Fatalf("got %v, want a diagnostic naming %s", err, tc.want)
			}
		})
	}
}

// withVersion returns 64 zero bytes with the given sfnt version, which is
// enough for the loader to identify the container.
func withVersion(version uint32) []byte {
	data := make([]byte, 64)
	binary.BigEndian.PutUint32(data[0:4], version)
	return data
}

// TestLoadRejectsCFF2 checks that a CFF2 font is refused rather than
// half-converted.
func TestLoadRejectsCFF2(t *testing.T) {
	data := testCFFFont([]byte{14})
	tables, err := tableDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	tables["CFF2"] = []byte{0, 0, 0, 0}
	_, err = Load(assembleSFNT(0x4F54544F, tables))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("CFF2")) {
		t.Fatalf("got %v, want a CFF2 diagnostic", err)
	}
}

// TestLoadRejectsVariableFont checks that a font with an fvar table is refused.
func TestLoadRejectsVariableFont(t *testing.T) {
	data := testCFFFont([]byte{14})
	tables, err := tableDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	tables["fvar"] = []byte{0, 0, 0, 0}
	_, err = Load(assembleSFNT(0x4F54544F, tables))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("variable")) {
		t.Fatalf("got %v, want a variable font diagnostic", err)
	}
}

// TestTruncatedCFFIsAnError checks that damaged CFF data is reported, not
// panicked on: the loader is fed every prefix of a real font.
func TestTruncatedCFFIsAnError(t *testing.T) {
	data := readCorpusFont(t, "barlowcondensed-regular.otf")
	for _, n := range []int{12, 64, 200, 1000, 12_000, 13_600, 20_000, len(data) / 2, len(data) - 1} {
		if _, err := Load(data[:n]); err == nil {
			t.Fatalf("%d bytes of the font were accepted", n)
		}
	}
}

// TestConvertCFFMalformedCharstring checks that a charstring that ends inside a
// number is reported rather than read past its end.
func TestConvertCFFMalformedCharstring(t *testing.T) {
	data := testCFFFont([]byte{28})
	f, err := Load(data)
	if err == nil {
		t.Fatalf("a truncated charstring was accepted: %+v", f)
	}
}

// testCFFFontWithHugeCurves builds a font whose single glyph is n cubics far too
// large for one quadratic, and returns the font and its charstring.
func testCFFFontWithHugeCurves(t *testing.T, n int) ([]byte, []byte) {
	t.Helper()
	b := new(charstringBuilder).num(0, 0).op(21)
	for i := 0; i < n; i++ {
		// This cubic stays inside the int16 coordinate range and still
		// subdivides into roughly 128 quadratics.
		b.num(8000, 8000, -16000, 0, 8000, -8000)
		b.op(8) // rrcurveto
	}
	b.op(14)
	code := b.bytes()
	return testCFFFont(code), code
}

// testCFFFont builds a minimal OpenType/CFF font whose only glyph is charstring.
func testCFFFont(charstring []byte) []byte {
	tables := map[string][]byte{
		"CFF ": testCFFTable(charstring),
		"maxp": []byte{0, 0, 0x50, 0x00, 0, 1}, // version 0.5, one glyph
		"head": testHead(),
		"hhea": testHhea(),
		"hmtx": []byte{0x01, 0xf4, 0, 0}, // one advance of 500 units
	}
	return assembleSFNT(0x4F54544F, tables)
}

// testHead is a head table for the synthetic fonts: 1000 units per em, short
// loca offsets.
func testHead() []byte {
	head := make([]byte, 54)
	binary.BigEndian.PutUint32(head[0:4], 0x00010000)
	binary.BigEndian.PutUint32(head[12:16], 0x5F0F3CF5) // magicNumber
	binary.BigEndian.PutUint16(head[18:20], 1000)       // unitsPerEm
	return head
}

// testHhea is an hhea table with one horizontal metric.
func testHhea() []byte {
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint32(hhea[0:4], 0x00010000)
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics
	return hhea
}

// testCFFIndex writes one CFF INDEX with an offset size that fits the data.
func testCFFIndex(entries [][]byte) []byte {
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, uint16(len(entries)))
	if len(entries) == 0 {
		return out
	}
	total := 1
	for _, entry := range entries {
		total += len(entry)
	}
	offSize := 1
	switch {
	case total > 0xFFFFFF:
		offSize = 4
	case total > 0xFFFF:
		offSize = 3
	case total > 0xFF:
		offSize = 2
	}
	out = append(out, byte(offSize))
	put := func(v int) {
		for i := offSize - 1; i >= 0; i-- {
			out = append(out, byte(v>>(8*i)))
		}
	}
	put(1)
	offset := 1
	for _, entry := range entries {
		offset += len(entry)
		put(offset)
	}
	for _, entry := range entries {
		out = append(out, entry...)
	}
	return out
}

// testCFFTable builds a CFF table with one charstring. The Top DICT offsets use
// the fixed-width int32 encoding so the DICT length does not depend on the
// values patched in afterwards.
func testCFFTable(charstring []byte) []byte {
	// Header: version 1.0, header size 4.
	out := []byte{1, 0, 4, 4}
	out = append(out, testCFFIndex([][]byte{[]byte("Synthetic")})...)
	// The Top DICT is [29 offset 17] [29 size 29 offset 18]: 17 bytes, which
	// keeps the offsets that are patched in below at a fixed position.
	topDict := make([]byte, 17)
	for _, at := range []int{0, 6, 11} {
		topDict[at] = 29
	}
	topDict[5], topDict[16] = 17, 18 // CharStrings, Private
	topIndexStart := len(out)
	out = append(out, testCFFIndex([][]byte{topDict})...)
	topDictStart := topIndexStart + 5 // count, offSize and two one-byte offsets
	// String INDEX and Global Subr INDEX: none.
	out = append(out, 0, 0, 0, 0)
	charStringsOffset := len(out)
	out = append(out, testCFFIndex([][]byte{charstring})...)
	privateOffset := len(out)
	out = append(out, 0, 0, 0, 0) // an empty Private DICT
	binary.BigEndian.PutUint32(out[topDictStart+1:topDictStart+5], uint32(charStringsOffset))
	binary.BigEndian.PutUint32(out[topDictStart+7:topDictStart+11], 0) // Private size
	binary.BigEndian.PutUint32(out[topDictStart+12:topDictStart+16], uint32(privateOffset))
	return out
}

// assembleSFNT writes tables into one sfnt. The tests use it to build fonts the
// corpus does not contain.
func assembleSFNT(version uint32, tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	// The directory must be sorted by tag.
	for i := 1; i < len(tags); i++ {
		for j := i; j > 0 && tags[j] < tags[j-1]; j-- {
			tags[j], tags[j-1] = tags[j-1], tags[j]
		}
	}
	header := make([]byte, 12+16*len(tags))
	binary.BigEndian.PutUint32(header[0:4], version)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(tags)))
	pow2 := 1
	for pow2*2 <= len(tags) {
		pow2 *= 2
	}
	binary.BigEndian.PutUint16(header[6:8], uint16(pow2*16))
	log2 := 0
	for 1<<log2 <= pow2 {
		log2++
	}
	binary.BigEndian.PutUint16(header[8:10], uint16(log2-1))
	binary.BigEndian.PutUint16(header[10:12], uint16(len(tags)*16-pow2*16))
	out := header
	offset := len(header)
	for i, tag := range tags {
		data := tables[tag]
		rec := 12 + i*16
		copy(out[rec:rec+4], tag)
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(offset))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], uint32(len(data)))
		out = append(out, data...)
		offset += len(data)
		for len(out)%4 != 0 {
			out = append(out, 0)
			offset++
		}
	}
	return out
}

// TestConvertedFontRoundTripsThroughSFNT parses the converted bytes with the
// oracle: a font the engine embeds must also be a valid font to another decoder.
func TestConvertedFontRoundTripsThroughSFNT(t *testing.T) {
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			tables, err := tableDirectory(data)
			if err != nil {
				t.Fatal(err)
			}
			converted, err := convertCFF(data, tables, nil)
			if err != nil {
				t.Fatal(err)
			}
			sf, err := sfnt.Parse(converted)
			if err != nil {
				t.Fatalf("sfnt rejects the converted font: %v", err)
			}
			var buf sfnt.Buffer
			for gid := 0; gid < sf.NumGlyphs(); gid++ {
				if _, err := sf.LoadGlyph(&buf, sfnt.GlyphIndex(gid), fixed.Int26_6(sf.UnitsPerEm()), nil); err != nil {
					t.Fatalf("glyph %d: %v", gid, err)
				}
			}
		})
	}
}

// TestCFFDeltaOutsideInt16IsRejected guards the converter: a charstring line
// from -32768 to 0 has a delta of 32768, which must not wrap to int16.
func TestCFFDeltaOutsideInt16IsRejected(t *testing.T) {
	// rmoveto(-32767.99998, 0), rlineto(32767.99998, 0): the points are
	// -32768 and 0 font units, so their delta is 32768.
	code := []byte{
		255, 0x80, 0x00, 0x00, 0x01, 139, 21,
		255, 0x7F, 0xFF, 0xFF, 0xFF, 139, 5, 14,
	}
	if _, err := Load(testCFFFont(code)); err == nil {
		t.Fatal("a delta outside int16 was accepted")
	} else if !strings.Contains(err.Error(), "delta") {
		t.Fatalf("error = %v, want it to name the delta", err)
	}
}

// TestCFFWithoutPrivateLoads guards the legal font whose Top DICT has no
// Private operator: it has no local subroutines and must load.
func TestCFFWithoutPrivateLoads(t *testing.T) {
	charstring := new(charstringBuilder).num(0, 0).op(21).num(100, 0).op(5).num(0, 100).op(5).op(14).bytes()
	f, err := Load(testCFFFontNoPrivate(charstring))
	if err != nil {
		t.Fatalf("a CFF font without a Private DICT was rejected: %v", err)
	}
	path, err := f.GlyphPath(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) == 0 {
		t.Fatal("glyph 0 has no outline")
	}
}

// testCFFFontNoPrivate builds a one-glyph CFF font whose Top DICT omits the
// Private operator.
func testCFFFontNoPrivate(charstring []byte) []byte {
	tables := map[string][]byte{
		"CFF ": testCFFTableNoPrivate(charstring),
		"maxp": []byte{0, 0, 0x50, 0x00, 0, 1},
		"head": testHead(),
		"hhea": testHhea(),
		"hmtx": []byte{0x01, 0xf4, 0, 0},
	}
	return assembleSFNT(0x4F54544F, tables)
}

func testCFFTableNoPrivate(charstring []byte) []byte {
	out := []byte{1, 0, 4, 4}
	out = append(out, testCFFIndex([][]byte{[]byte("Synthetic")})...)
	topDict := make([]byte, 6)
	topDict[0] = 29
	topDict[5] = 17 // CharStrings
	topIndexStart := len(out)
	out = append(out, testCFFIndex([][]byte{topDict})...)
	topDictStart := topIndexStart + 5
	out = append(out, 0, 0, 0, 0) // String INDEX and Global Subr INDEX
	charStringsOffset := len(out)
	out = append(out, testCFFIndex([][]byte{charstring})...)
	binary.BigEndian.PutUint32(out[topDictStart+1:topDictStart+5], uint32(charStringsOffset))
	return out
}

// TestCFFConversionIsCharged guards RV4-05: converting a CFF font charges one
// unit per glyph and per charstring instruction, so a 394 KB bomb stops with a
// font limit long before it executes its millions of instructions.
func TestCFFConversionIsCharged(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "adversarial", "cff-bomb.otf"))
	if err != nil {
		t.Fatal(err)
	}
	g := budget.New(context.Background(), 100_000)
	if _, err := LoadWithOptions(data, ParseOptions{Guard: g}); !errors.Is(err, fontErrLimit()) {
		t.Fatalf("the CFF bomb was not refused: %v", err)
	}
	if used := g.Used(); used > 200_000 {
		t.Fatalf("the bomb spent %d units before it was refused", used)
	}
}

// fontErrLimit is the sentinel the conversion reports.
func fontErrLimit() error { return ErrLimit }

// cancelAfterContext reports a cancellation after a number of Err calls, so a
// test observes cancellation in the middle of a conversion without timing it.
type cancelAfterContext struct {
	context.Context
	mu    sync.Mutex
	calls int
	after int
}

func (c *cancelAfterContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls > c.after {
		return context.Canceled
	}
	return nil
}

// TestCFFConversionHonoursCancellation guards the cancellation half of RV4-05:
// a context cancelled mid-conversion returns the context error after a bounded
// number of instructions, not after the whole font is converted.
func TestCFFConversionHonoursCancellation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "adversarial", "cff-bomb.otf"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterContext{Context: context.Background(), after: 3}
	g := budget.New(ctx, 0)
	_, err = LoadWithOptions(data, ParseOptions{Guard: g})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled conversion returned %v", err)
	}
	if used := g.Used(); used > 100_000 {
		t.Fatalf("the cancelled conversion spent %d units", used)
	}
}

// TestCFFFontWideInstructionCap guards the backstop for a caller with no work
// guard: a font that executes more than cffMaxTotalInstructions between all of
// its glyphs is refused instead of converting for minutes.
func TestCFFFontWideInstructionCap(t *testing.T) {
	f := &cffTable{
		charStrings:  [][]byte{{139, 139, 21, 139, 139, 21, 14}},
		fds:          []cffFontDict{{}},
		instructions: cffMaxTotalInstructions,
	}
	if _, err := f.glyphOutline(0); !errors.Is(err, ErrLimit) {
		t.Fatalf("the font-wide instruction cap did not stop the glyph: %v", err)
	}
}
