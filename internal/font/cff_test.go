package font

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// cffCorpusFonts are the OpenType/CFF fonts of the corpus.
var cffCorpusFonts = []string{
	"barlowcondensed-regular.otf",
	"barlowcondensed-bold.otf",
	"barlowcondensed-light.otf",
}

// corpusPath is the path of one corpus font.
func corpusPath(name string) string {
	return filepath.Join("..", "..", "testdata", "weasyprint", "ticket", name)
}

// readCorpusFont reads one corpus font.
func readCorpusFont(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(corpusPath(name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// cffOf parses the CFF table of one sfnt.
func cffOf(t *testing.T, data []byte) *cffTable {
	t.Helper()
	tables, err := tableDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables["maxp"]) < 6 {
		t.Fatal("the fixture has no maxp table")
	}
	numGlyphs := int(readUint16(tables["maxp"][4:6]))
	cff, err := parseCFFTable(tables["CFF "], numGlyphs)
	if err != nil {
		t.Fatal(err)
	}
	return cff
}

func readUint16(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }

// TestCFFReaderMatchesSFNT compares the reader's cubic segments with sfnt's for
// every glyph the oracle can draw.
//
// sfnt is an independent Type 2 interpreter, so a divergence is a defect in one
// of the two. It cannot draw flex or flex1 (see the spike in the change), so the
// glyphs it refuses are counted rather than compared: a suite that silently
// compared less would be worse than no suite.
func TestCFFReaderMatchesSFNT(t *testing.T) {
	compared := 0
	for _, name := range cffCorpusFonts {
		t.Run(name, func(t *testing.T) {
			data := readCorpusFont(t, name)
			cff := cffOf(t, data)
			sf, err := sfnt.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			var buf sfnt.Buffer
			ppem := fixed.Int26_6(sf.UnitsPerEm())
			skipped := 0
			for gid := 0; gid < sf.NumGlyphs(); gid++ {
				want, err := sf.LoadGlyph(&buf, sfnt.GlyphIndex(gid), ppem, nil)
				if err != nil {
					skipped++
					continue
				}
				got, err := cff.glyphOutline(uint16(gid))
				if err != nil {
					t.Fatalf("glyph %d: %v", gid, err)
				}
				if err := compareCFFSegments(gid, got, want); err != nil {
					t.Fatal(err)
				}
				compared++
			}
			// sfnt fails on exactly the flex1 glyphs of each face.
			if skipped == 0 {
				t.Fatalf("sfnt refused no glyph: the flex1 gap this reader closes is gone, extend the test")
			}
			t.Logf("%s: %d glyphs compared, %d refused by sfnt", name, sf.NumGlyphs()-skipped, skipped)
		})
	}
	if compared < 3*600 {
		t.Fatalf("compared %d glyphs, want at least 1800", compared)
	}
}

// compareCFFSegments checks one glyph's segments against sfnt's. sfnt flips the
// y axis and appends an explicit closing line, and its coordinates are whole
// font units while ours are 26.6; the comparison normalises all three.
func compareCFFSegments(gid int, got []cffSegment, want sfnt.Segments) error {
	gotContours := segmentsToContours(got)
	wantContours := sfntContours(want)
	if len(gotContours) != len(wantContours) {
		return fmt.Errorf("glyph %d: %d contours in the reader, %d in sfnt", gid, len(gotContours), len(wantContours))
	}
	for ci := range wantContours {
		g, w := gotContours[ci], wantContours[ci]
		if len(g) != len(w) {
			return fmt.Errorf("glyph %d contour %d: %d points in the reader, %d in sfnt", gid, ci, len(g), len(w))
		}
		for i := range w {
			if g[i].op != w[i].op {
				return fmt.Errorf("glyph %d contour %d point %d: op %d in the reader, %d in sfnt", gid, ci, i, g[i].op, w[i].op)
			}
			for _, d := range [2]int{int(g[i].x - w[i].x), int(g[i].y - w[i].y)} {
				if d < -cffOracleTolerance || d > cffOracleTolerance {
					return fmt.Errorf("glyph %d contour %d point %d: (%d,%d) in the reader, (%d,%d) in sfnt",
						gid, ci, i, g[i].x, g[i].y, w[i].x, w[i].y)
				}
			}
		}
	}
	return nil
}

// cffOracleTolerance is how far the reader may differ from sfnt, in 26.6 font
// units. sfnt rounds every 16.16 operand to a whole font unit before adding it,
// so a chain of fractional operands drifts by up to a unit; the reader keeps the
// fraction until the segment is emitted.
const cffOracleTolerance = 0

// outlinePoint is one point of a normalised outline: a move, a line or a cubic
// control point.
type outlinePoint struct {
	op   cffOp
	x, y int32
}

// segmentsToContours splits the reader's segments at every move, in 26.6 units.
func segmentsToContours(segments []cffSegment) [][]outlinePoint {
	var contours [][]outlinePoint
	var cur []outlinePoint
	for _, s := range segments {
		switch s.Op {
		case cffMoveTo:
			if len(cur) > 0 {
				contours = append(contours, cur)
			}
			cur = []outlinePoint{{op: cffMoveTo, x: s.Args[0].X, y: s.Args[0].Y}}
		case cffLineTo:
			cur = append(cur, outlinePoint{op: cffLineTo, x: s.Args[0].X, y: s.Args[0].Y})
		case cffCubeTo:
			for i := 0; i < 3; i++ {
				cur = append(cur, outlinePoint{op: cffCubeTo, x: s.Args[i].X, y: s.Args[i].Y})
			}
		}
	}
	if len(cur) > 0 {
		contours = append(contours, cur)
	}
	return contours
}

// sfntContours normalises sfnt's segments the same way: whole font units become
// 26.6 units, the y axis is flipped back, and the explicit closing line sfnt
// appends is dropped.
func sfntContours(segments sfnt.Segments) [][]outlinePoint {
	var contours [][]outlinePoint
	var cur []outlinePoint
	for _, s := range segments {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if len(cur) > 0 {
				contours = append(contours, trimClosingLine(cur))
			}
			cur = []outlinePoint{{op: cffMoveTo, x: int32(s.Args[0].X) * 64, y: -int32(s.Args[0].Y) * 64}}
		case sfnt.SegmentOpLineTo:
			cur = append(cur, outlinePoint{op: cffLineTo, x: int32(s.Args[0].X) * 64, y: -int32(s.Args[0].Y) * 64})
		case sfnt.SegmentOpCubeTo:
			for i := 0; i < 3; i++ {
				cur = append(cur, outlinePoint{op: cffCubeTo, x: int32(s.Args[i].X) * 64, y: -int32(s.Args[i].Y) * 64})
			}
		}
	}
	if len(cur) > 0 {
		contours = append(contours, trimClosingLine(cur))
	}
	return contours
}

// trimClosingLine drops the line sfnt appends to close a contour: the reader
// closes implicitly and never emits it.
func trimClosingLine(contour []outlinePoint) []outlinePoint {
	if len(contour) < 2 {
		return contour
	}
	first, last := contour[0], contour[len(contour)-1]
	if last.op == cffLineTo && last.x == first.x && last.y == first.y {
		return contour[:len(contour)-1]
	}
	return contour
}

// charstringBuilder writes Type 2 charstrings for the tests: numbers are
// encoded the way a font encodes them and operators are copied as bytes.
type charstringBuilder struct{ b []byte }

// num appends number operands.
func (c *charstringBuilder) num(values ...int) *charstringBuilder {
	for _, v := range values {
		switch {
		case v >= -107 && v <= 107:
			c.b = append(c.b, byte(v+139))
		case v >= 108 && v <= 1131:
			v -= 108
			c.b = append(c.b, byte(v/256+247), byte(v%256))
		case v >= -1131 && v <= -108:
			v = -v - 108
			c.b = append(c.b, byte(v/256+251), byte(v%256))
		default:
			c.b = append(c.b, 28, byte(v>>8), byte(v))
		}
	}
	return c
}

// op appends operator bytes.
func (c *charstringBuilder) op(operators ...byte) *charstringBuilder {
	c.b = append(c.b, operators...)
	return c
}

func (c *charstringBuilder) bytes() []byte { return c.b }

// charstringFont builds a one-glyph CFF font from raw charstring bytes, so a
// test can state an operator's expected outline without a binary fixture.
func charstringFont(charstring []byte) *cffTable {
	return &cffTable{
		charStrings: [][]byte{charstring},
		fds:         []cffFontDict{{}},
	}
}

// TestCFFReaderFlexOperators checks the operators sfnt cannot interpret against
// coordinates worked out by hand: no decoder on hand can be the reference, so
// the expected points are written out.
func TestCFFReaderFlexOperators(t *testing.T) {
	// Moves to the origin.
	origin := func() *charstringBuilder { return new(charstringBuilder).num(0, 0).op(21) }
	tests := []struct {
		name string
		code []byte
		want []cffSegment
	}{
		{
			// hflex: dx1 dx2 dy2 dx3 dx4 dx5 dx6. The second curve mirrors
			// dy2, so the glyph ends where the first curve left the baseline.
			name: "hflex",
			code: origin().num(10, 20, 30, 40, 10, 20, 30).op(12, 34, 14).bytes(),
			want: []cffSegment{
				{Op: cffMoveTo, Args: [3]cffPoint{{}}},
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 10 << 6}, {X: 30 << 6, Y: 30 << 6}, {X: 70 << 6, Y: 30 << 6}}},
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 80 << 6, Y: 30 << 6}, {X: 100 << 6}, {X: 130 << 6}}},
			},
		},
		{
			// flex: twelve deltas and the flex depth, which does not move the
			// outline.
			name: "flex",
			code: origin().num(10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 50).op(12, 35, 14).bytes(),
			want: []cffSegment{
				{Op: cffMoveTo, Args: [3]cffPoint{{}}},
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 10 << 6, Y: 20 << 6}, {X: 40 << 6, Y: 60 << 6}, {X: 90 << 6, Y: 120 << 6}}},
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 160 << 6, Y: 200 << 6}, {X: 250 << 6, Y: 300 << 6}, {X: 360 << 6, Y: 420 << 6}}},
			},
		},
		{
			// hflex1: dx1 dy1 dx2 dy2 dx3 dx4 dx5 dy5 dx6. The last delta is
			// vertical and closes the curve: dy6 = -(dy1+dy2+dy5).
			name: "hflex1",
			code: origin().num(10, 5, 20, 10, 30, 40, 50, 15, 60).op(12, 36, 14).bytes(),
			want: []cffSegment{
				{Op: cffMoveTo, Args: [3]cffPoint{{}}},
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 10 << 6, Y: 5 << 6}, {X: 30 << 6, Y: 15 << 6}, {X: 60 << 6, Y: 15 << 6}}},
				// dy6 = -(dy1+dy2+dy5) = -(5+10+15) = -30, so the second
				// curve returns to the baseline.
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 100 << 6, Y: 15 << 6}, {X: 150 << 6, Y: 30 << 6}, {X: 210 << 6, Y: 0}}},
			},
		},
		{
			// flex1 with more horizontal than vertical travel: the last delta
			// is horizontal and the vertical one is computed.
			name: "flex1 horizontal",
			code: origin().num(10, 2, 30, 3, 50, 4, 70, 5, 90, 6, 110).op(12, 37, 14).bytes(),
			want: []cffSegment{
				{Op: cffMoveTo, Args: [3]cffPoint{{}}},
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 10 << 6, Y: 2 << 6}, {X: 40 << 6, Y: 5 << 6}, {X: 90 << 6, Y: 9 << 6}}},
				// The horizontal travel (250) exceeds the vertical one (20),
				// so the last delta is dx6 = 110 and dy6 = -(2+3+4+5+6) = -20.
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 160 << 6, Y: 14 << 6}, {X: 250 << 6, Y: 20 << 6}, {X: 360 << 6, Y: 0}}},
			},
		},
		{
			// flex1 with more vertical than horizontal travel: the last delta
			// is vertical and the horizontal one is computed.
			name: "flex1 vertical",
			code: origin().num(2, 10, 3, 30, 4, 50, 5, 70, 6, 90, 110).op(12, 37, 14).bytes(),
			want: []cffSegment{
				{Op: cffMoveTo, Args: [3]cffPoint{{}}},
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 2 << 6, Y: 10 << 6}, {X: 5 << 6, Y: 40 << 6}, {X: 9 << 6, Y: 90 << 6}}},
				// The vertical travel (250) exceeds the horizontal one (20),
				// so the last delta is dy6 = 110 and dx6 = -(2+3+4+5+6) = -20.
				{Op: cffCubeTo, Args: [3]cffPoint{{X: 14 << 6, Y: 160 << 6}, {X: 20 << 6, Y: 250 << 6}, {X: 0, Y: 360 << 6}}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := charstringFont(tc.code)
			got, err := f.glyphOutline(0)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("%d segments, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i].Op != tc.want[i].Op {
					t.Fatalf("segment %d: op %d, want %d", i, got[i].Op, tc.want[i].Op)
				}
				count := 1
				if got[i].Op == cffCubeTo {
					count = 3
				}
				for k := 0; k < count; k++ {
					if got[i].Args[k] != tc.want[i].Args[k] {
						t.Fatalf("segment %d point %d: %+v, want %+v", i, k, got[i].Args[k], tc.want[i].Args[k])
					}
				}
			}
		})
	}
}

// TestCFFReaderRejectsSeac checks that the deprecated accent composition is
// refused instead of drawn without its accent.
func TestCFFReaderRejectsSeac(t *testing.T) {
	// endchar with four operands is a seac: adx ady bchar achar.
	code := new(charstringBuilder).num(0, 0, 0, 0).op(14).bytes()
	f := charstringFont(code)
	_, err := f.glyphOutline(0)
	if err == nil || !strings.Contains(err.Error(), "seac") {
		t.Fatalf("got %v, want a seac diagnostic", err)
	}
}

// TestCFFReaderBoundsCharstring checks that a subroutine that calls itself is
// stopped by the nesting limit instead of recursing until the stack runs out.
func TestCFFReaderBoundsCharstring(t *testing.T) {
	// Subroutine 0 calls itself: operand -107 plus the bias of 107 is index 0.
	selfCall := new(charstringBuilder).num(-107).op(10).bytes()
	f := &cffTable{
		charStrings: [][]byte{new(charstringBuilder).num(-107).op(10).bytes()},
		fds:         []cffFontDict{{subrs: [][]byte{selfCall}}},
	}
	_, err := f.glyphOutline(0)
	if err == nil || !strings.Contains(err.Error(), "nests subroutines") {
		t.Fatalf("got %v, want a nesting diagnostic", err)
	}
}

// TestParseCFFFDSelect checks both layouts of the font-dict selection a
// CID-keyed font uses.
func TestParseCFFFDSelect(t *testing.T) {
	format0 := []byte{0, 0, 1}
	got, err := parseCFFFDSelect(format0, 0, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("format 0 selects %v, want [0 1]", got)
	}
	// format 3: two ranges, glyph 0 in font dict 0 and glyph 1 in font dict 1.
	format3 := []byte{3, 0, 2, 0, 0, 0, 0, 1, 1, 0, 2}
	got, err = parseCFFFDSelect(format3, 0, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("format 3 selects %v, want [0 1]", got)
	}
	// A range that names a font dict the font does not have is refused.
	if _, err := parseCFFFDSelect(format0, 0, 2, 1); err == nil {
		t.Fatal("a font-dict index outside the font was accepted")
	}
	// A glyph the selection does not cover is refused.
	short := []byte{3, 0, 1, 0, 0, 0, 0, 1}
	if _, err := parseCFFFDSelect(short, 0, 2, 2); err == nil {
		t.Fatal("an FDSelect that stops early was accepted")
	}
}

// TestCIDKeyedCFF checks a CID-keyed font, where each glyph draws with the local
// subroutines of the font dict that FDSelect names. The two glyphs use different
// font dicts and only the second declares a subroutine.
func TestCIDKeyedCFF(t *testing.T) {
	data := testCIDCFFFont()
	tables, err := tableDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	cff, err := parseCFFTable(tables["CFF "], 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(cff.fds) != 2 {
		t.Fatalf("%d font dicts, want 2", len(cff.fds))
	}
	if len(cff.fds[1].subrs) != 1 {
		t.Fatalf("the second font dict has %d subroutines, want 1", len(cff.fds[1].subrs))
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := f.GlyphPath(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("glyph 0 has %d contours, want none", len(empty))
	}
	outline, err := f.GlyphPath(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(outline) != 1 || len(outline[0]) != 2 {
		t.Fatalf("glyph 1 has %d contours of %d points, want one of two", len(outline), len(outline[0]))
	}
	// The subroutine draws a line to (10,20), which ends the contour.
	end := outline[0][1]
	if end.X != 10 || end.Y != 20 || !end.OnCurve {
		t.Fatalf("the contour ends at (%v,%v) on-curve=%v, want (10,20)", end.X, end.Y, end.OnCurve)
	}
}

// testCIDCFFFont builds a whole sfnt whose CFF table is CID-keyed: glyph 0 uses
// font dict 0 and draws nothing, glyph 1 uses font dict 1 and draws a line
// through a local subroutine.
func testCIDCFFFont() []byte {
	glyphs := [][]byte{
		{139, 139, 21, 14},         // rmoveto 0 0, endchar
		{139, 139, 21, 32, 10, 14}, // rmoveto 0 0, callsubr -107, endchar
	}
	subr := []byte{149, 159, 5, 11} // rlineto 10 20, return
	nameIndex := testCFFIndex([][]byte{[]byte("SyntheticCID")})
	charStrings := testCFFIndex(glyphs)
	subrs := testCFFIndex([][]byte{subr})
	// Each Font DICT is [29 size][29 offset][18]; they are patched below.
	fd0 := make([]byte, 11)
	fd1 := make([]byte, 11)
	fdArray := testCFFIndex([][]byte{fd0, fd1})
	fdSelect := []byte{0, 0, 1} // format 0: glyph 0 in FD 0, glyph 1 in FD 1
	topDict := make([]byte, 25)

	// Header, then the Name INDEX and the Top DICT INDEX.
	out := []byte{1, 0, 4, 4}
	out = append(out, nameIndex...)
	topIndexStart := len(out)
	out = append(out, testCFFIndex([][]byte{topDict})...)
	out = append(out, 0, 0, 0, 0) // String INDEX and Global Subr INDEX
	charStringsOffset := len(out)
	out = append(out, charStrings...)
	fdArrayOffset := len(out)
	out = append(out, fdArray...)
	fdSelectOffset := len(out)
	out = append(out, fdSelect...)
	private0Offset := len(out)
	private1Offset := private0Offset      // FD 0 has no Private data
	out = append(out, make([]byte, 6)...) // FD 1's Private DICT: [29 rel 19]
	subrsOffset := len(out)
	out = append(out, subrs...)

	// Top DICT: ROS (12 30), then the three offsets this font uses.
	copy(topDict[0:5], []byte{139, 139, 139, 12, 30})
	topDict[5] = 29
	binary.BigEndian.PutUint32(topDict[6:10], uint32(charStringsOffset))
	topDict[10] = 17
	topDict[11] = 29
	binary.BigEndian.PutUint32(topDict[12:16], uint32(fdArrayOffset))
	topDict[16], topDict[17] = 12, 36
	topDict[18] = 29
	binary.BigEndian.PutUint32(topDict[19:23], uint32(fdSelectOffset))
	topDict[23], topDict[24] = 12, 37

	// The Font DICTs point at their Private DICTs.
	for _, fd := range []struct {
		dict  []byte
		start int
		size  int
	}{
		{fd0, private0Offset, 0},
		{fd1, private1Offset, 6},
	} {
		fd.dict[0] = 29
		binary.BigEndian.PutUint32(fd.dict[1:5], uint32(fd.size))
		fd.dict[5] = 29
		binary.BigEndian.PutUint32(fd.dict[6:10], uint32(fd.start))
		fd.dict[10] = 18
	}
	// FD 1's Private DICT declares Subrs, relative to its own start.
	private1 := out[private1Offset : private1Offset+6]
	private1[0] = 29
	binary.BigEndian.PutUint32(private1[1:5], uint32(subrsOffset-private1Offset))
	private1[5] = 19

	// Patch the Top DICT and the two Font DICTs back into the font. The
	// placeholder lengths are the lengths of the final bytes.
	copy(out[topIndexStart+5:], topDict)
	fdArrayStart := fdArrayOffset + 6 // count, offSize and three one-byte offsets
	copy(out[fdArrayStart:fdArrayStart+11], fd0)
	copy(out[fdArrayStart+11:fdArrayStart+22], fd1)

	return assembleSFNT(0x4F54544F, map[string][]byte{
		"CFF ": out,
		"maxp": []byte{0, 0, 0x50, 0x00, 0, 2}, // version 0.5, two glyphs
		"head": testHead(),
		"hhea": testHhea(),
		"hmtx": []byte{0x01, 0xf4, 0, 0, 0x01, 0xf4, 0, 0},
	})
}
