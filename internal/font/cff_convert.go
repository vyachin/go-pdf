package font

import (
	"encoding/binary"
	"fmt"
)

// The conversion of a CFF outline into TrueType contours is bounded by a few
// constants. They are recorded in docs/fonts.md next to the tolerance.
const (
	// outlineFitTolerance is the greatest distance, in 26.6 font units, that
	// the quadratic fitted to a cubic may stray from it: one quarter of a font
	// unit, or 0.00025 em at 1000 units per em.
	outlineFitTolerance = 16
	// maxGlyphPoints bounds one converted glyph. The finished font stores
	// integer font units, so the tolerance above applies to the fitted curve and
	// the stored points then round to the nearest unit.
	maxGlyphPoints = 8192
	// convertedGlyfFactor and maxConvertedGlyf bound the synthesized glyf.
	// MaxFontBytes says nothing about a small CFF that describes very dense
	// outlines.
	convertedGlyfFactor = 4
	maxConvertedGlyf    = 64 << 20
)

// convertCFF builds a TrueType font from an OpenType/CFF font. The glyph
// numbering, the character map, the kerning and the metrics are those of the
// source: only the outlines and the tables that describe them are rewritten.
func convertCFF(data []byte, tables map[string][]byte) ([]byte, error) {
	cffData, ok := tables["CFF "]
	if !ok {
		return nil, fmt.Errorf("font: OpenType font has no CFF table")
	}
	maxp := tables["maxp"]
	if len(maxp) < 6 {
		return nil, fmt.Errorf("font: missing maxp table")
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:6]))
	cff, err := parseCFFTable(cffData, numGlyphs)
	if err != nil {
		return nil, err
	}

	limit := convertedGlyfLimit(len(data))
	glyf := make([]byte, 0, len(cffData))
	loca := make([]uint32, numGlyphs+1)
	maxPoints, maxContours := 0, 0
	var bbox [4]int16
	haveBBox := false
	for gid := 0; gid < numGlyphs; gid++ {
		loca[gid] = uint32(len(glyf))
		segments, err := cff.glyphOutline(uint16(gid))
		if err != nil {
			return nil, err
		}
		contours, err := cffContours(segments, uint16(gid))
		if err != nil {
			return nil, err
		}
		glyph, points, covers, err := encodeSimpleGlyph(contours, uint16(gid))
		if err != nil {
			return nil, err
		}
		glyf = append(glyf, glyph...)
		// Every glyph starts on a four-byte boundary, like the subsetter does.
		for len(glyf)%4 != 0 {
			glyf = append(glyf, 0)
		}
		if len(glyf) > limit {
			return nil, fmt.Errorf("%w: glyph %d makes the converted glyf larger than %d bytes", ErrLimit, gid, limit)
		}
		if points > maxPoints {
			maxPoints = points
		}
		if len(contours) > maxContours {
			maxContours = len(contours)
		}
		if points > 0 {
			if !haveBBox {
				bbox, haveBBox = covers, true
			} else {
				bbox = [4]int16{
					minInt16(bbox[0], covers[0]), minInt16(bbox[1], covers[1]),
					maxInt16(bbox[2], covers[2]), maxInt16(bbox[3], covers[3]),
				}
			}
		}
	}
	loca[numGlyphs] = uint32(len(glyf))

	locaBytes := make([]byte, len(loca)*4)
	for i, offset := range loca {
		binary.BigEndian.PutUint32(locaBytes[i*4:i*4+4], offset)
	}

	head := append([]byte(nil), tables["head"]...)
	if len(head) < 54 {
		return nil, fmt.Errorf("font: head table is too short to convert")
	}
	if haveBBox {
		for i, v := range bbox {
			binary.BigEndian.PutUint16(head[36+i*2:36+i*2+2], uint16(v))
		}
	}
	binary.BigEndian.PutUint32(head[8:12], 0)  // checkSumAdjustment
	binary.BigEndian.PutUint16(head[50:52], 1) // long loca offsets

	out := []outTable{
		{tag: "head", data: head},
		{tag: "hhea", data: append([]byte(nil), tables["hhea"]...)},
		{tag: "maxp", data: maxpTable(numGlyphs, maxPoints, maxContours)},
		{tag: "hmtx", data: append([]byte(nil), tables["hmtx"]...)},
		{tag: "loca", data: locaBytes},
		{tag: "glyf", data: glyf},
	}
	// The copied tables stay valid because the glyph numbering is preserved:
	// cmap and GPOS answer with the same glyph ids, hmtx with the same advances,
	// and post with the same names.
	for _, tag := range []string{"cmap", "GPOS", "name", "OS/2", "post"} {
		if t, ok := tables[tag]; ok {
			out = append(out, outTable{tag: tag, data: append([]byte(nil), t...)})
		}
	}
	return assembleFont(out)
}

// convertedGlyfLimit is the greatest glyf the converter will build for an input
// of n bytes: four times the input, and never more than 64 MiB.
func convertedGlyfLimit(n int) int {
	limit := maxConvertedGlyf
	if factor := convertedGlyfFactor * n; factor < limit {
		limit = factor
	}
	return limit
}

// maxpTable writes a version 1.0 maxp with the maxima of the converted
// outlines. A CFF font ships maxp version 0.5, which carries only the glyph
// count and cannot describe TrueType glyphs.
func maxpTable(numGlyphs, maxPoints, maxContours int) []byte {
	out := make([]byte, 32)
	binary.BigEndian.PutUint32(out[0:4], 0x00010000)
	binary.BigEndian.PutUint16(out[4:6], uint16(numGlyphs))
	binary.BigEndian.PutUint16(out[6:8], uint16(maxPoints))
	binary.BigEndian.PutUint16(out[8:10], uint16(maxContours))
	// maxCompositePoints, maxCompositeContours, maxTwilightPoints, maxStorage,
	// maxFunctionDefs, maxInstructionDefs, maxStackElements,
	// maxSizeOfInstructions, maxComponentElements and maxComponentDepth stay
	// zero: the converted glyphs are simple and carry no instructions. maxZones
	// is the exception, which the format requires to be 1 or 2.
	binary.BigEndian.PutUint16(out[14:16], 2)
	return out
}

// cffOutPoint is one point of a converted glyph: a position in 26.6 font units.
type cffOutPoint struct {
	x, y    int32
	onCurve bool
}

// cffContour is one converted contour. TrueType closes it implicitly.
type cffContour []cffOutPoint

// cffContours turns the segments of one glyph into TrueType contours: every
// cubic becomes a run of quadratics, and the closing point is dropped because
// TrueType closes a contour on its own.
func cffContours(segments []cffSegment, gid uint16) ([]cffContour, error) {
	var contours []cffContour
	var contour cffContour
	points := 0
	for _, segment := range segments {
		switch segment.Op {
		case cffMoveTo:
			if closed := closeContour(contour); len(closed) > 1 {
				contours = append(contours, closed)
			}
			contour = cffContour{{x: segment.Args[0].X, y: segment.Args[0].Y, onCurve: true}}
			points++
		case cffLineTo:
			contour = append(contour, cffOutPoint{x: segment.Args[0].X, y: segment.Args[0].Y, onCurve: true})
			points++
		case cffCubeTo:
			if len(contour) == 0 {
				return nil, fmt.Errorf("font: CFF glyph %d draws a curve before its first move", gid)
			}
			last := contour[len(contour)-1]
			start := cffPoint{X: last.x, Y: last.y}
			quads := cubicQuadratics(start, segment.Args[0], segment.Args[1], segment.Args[2])
			points += 2 * len(quads)
			if points > maxGlyphPoints {
				return nil, fmt.Errorf("%w: glyph %d needs more than %d points", ErrLimit, gid, maxGlyphPoints)
			}
			for _, q := range quads {
				contour = append(contour,
					cffOutPoint{x: q.ctrl.X, y: q.ctrl.Y},
					cffOutPoint{x: q.end.X, y: q.end.Y, onCurve: true})
			}
		}
	}
	if closed := closeContour(contour); len(closed) > 1 {
		contours = append(contours, closed)
	}
	return contours, nil
}

// closeContour drops the final point when it repeats the first one: a stored
// duplicate would draw a zero-length segment. A contour left with a single point
// draws nothing and is dropped by the caller.
func closeContour(c cffContour) cffContour {
	if len(c) > 1 {
		first, last := c[0], c[len(c)-1]
		if last.onCurve && last.x == first.x && last.y == first.y {
			c = c[:len(c)-1]
		}
	}
	return c
}

// quadratic is one quadratic segment: an off-curve control point and the
// on-curve end point.
type quadratic struct {
	ctrl cffPoint
	end  cffPoint
}

// cubicQuadratics splits a cubic until each piece is within
// outlineFitTolerance of its quadratic. The split is de Casteljau at the
// midpoint in integer arithmetic, so two runs over the same font produce the
// same points on every platform.
//
// With the quadratic that passes through the cubic's midpoint, the distance
// between the two curves is |3(P1-P2)+(P3-P0)| / (12*sqrt(3)); comparing
// squared lengths keeps the test exact and integer.
func cubicQuadratics(p0, p1, p2, p3 cffPoint) []quadratic {
	vx := 3*(int64(p1.X)-int64(p2.X)) + int64(p3.X) - int64(p0.X)
	vy := 3*(int64(p1.Y)-int64(p2.Y)) + int64(p3.Y) - int64(p0.Y)
	if vx*vx+vy*vy <= 432*outlineFitTolerance*outlineFitTolerance {
		return []quadratic{{
			ctrl: cffPoint{
				X: divRound(3*(int64(p1.X)+int64(p2.X))-int64(p0.X)-int64(p3.X), 4),
				Y: divRound(3*(int64(p1.Y)+int64(p2.Y))-int64(p0.Y)-int64(p3.Y), 4),
			},
			end: p3,
		}}
	}
	p01 := cffMid(p0, p1)
	p12 := cffMid(p1, p2)
	p23 := cffMid(p2, p3)
	p012 := cffMid(p01, p12)
	p123 := cffMid(p12, p23)
	p0123 := cffMid(p012, p123)
	left := cubicQuadratics(p0, p01, p012, p0123)
	return append(left, cubicQuadratics(p0123, p123, p23, p3)...)
}

func cffMid(a, b cffPoint) cffPoint {
	return cffPoint{X: int32((int64(a.X) + int64(b.X)) / 2), Y: int32((int64(a.Y) + int64(b.Y)) / 2)}
}

// encodeSimpleGlyph writes one simple glyph and returns it with the number of
// points it holds and the box its points cover. A glyph without contours has no
// data at all, which loca records as an empty range.
func encodeSimpleGlyph(contours []cffContour, gid uint16) (data []byte, points int, bbox [4]int16, err error) {
	for _, c := range contours {
		points += len(c)
	}
	if points == 0 {
		return nil, 0, [4]int16{}, nil
	}
	// glyf stores int16 font units; a CFF outline outside that range cannot be
	// represented, and clamping it would move the glyph.
	rounded := make([]cffContour, len(contours))
	all := make([]cffOutPoint, 0, points)
	for ci, c := range contours {
		out := make(cffContour, len(c))
		for i, p := range c {
			x := divRound(int64(p.x), 64)
			y := divRound(int64(p.y), 64)
			if x < -32768 || x > 32767 || y < -32768 || y > 32767 {
				return nil, 0, [4]int16{}, fmt.Errorf("font: glyph %d has the point (%d,%d) outside the TrueType coordinate range", gid, x, y)
			}
			out[i] = cffOutPoint{x: x, y: y, onCurve: p.onCurve}
			all = append(all, out[i])
		}
		rounded[ci] = out
	}
	bbox = pointBounds(all)
	out := make([]byte, 0, 10+2*len(rounded)+2+points*3)
	out = appendInt16(out, int16(len(rounded)))
	for _, v := range bbox {
		out = appendInt16(out, v)
	}
	last := -1
	flags := make([]byte, 0, points)
	var xs, ys []byte
	prevX, prevY := 0, 0
	for _, c := range rounded {
		last += len(c)
		out = appendUint16(out, uint16(last))
		for _, p := range c {
			dx, dy := int(p.x)-prevX, int(p.y)-prevY
			prevX, prevY = int(p.x), int(p.y)
			var flag byte
			if p.onCurve {
				flag |= 0x01
			}
			flag |= appendDelta(&xs, dx, 0x02, 0x10)
			flag |= appendDelta(&ys, dy, 0x04, 0x20)
			flags = append(flags, flag)
		}
	}
	out = appendUint16(out, 0) // no instructions
	out = append(out, flags...)
	out = append(out, xs...)
	out = append(out, ys...)
	return out, points, bbox, nil
}

// appendDelta writes one coordinate delta and returns the flag bits that
// describe its encoding: a zero delta is flagged as "same", a delta within a
// byte uses a short vector, and anything else is a signed 16-bit vector.
func appendDelta(dst *[]byte, delta int, shortFlag, sameOrPositiveFlag byte) byte {
	switch {
	case delta == 0:
		return sameOrPositiveFlag
	case delta >= -255 && delta <= 255:
		flag := shortFlag
		if delta > 0 {
			flag |= sameOrPositiveFlag
			*dst = append(*dst, byte(delta))
		} else {
			*dst = append(*dst, byte(-delta))
		}
		return flag
	default:
		*dst = appendInt16(*dst, int16(delta))
		return 0
	}
}

func pointBounds(points []cffOutPoint) [4]int16 {
	if len(points) == 0 {
		return [4]int16{}
	}
	minX, minY := points[0].x, points[0].y
	maxX, maxY := minX, minY
	for _, p := range points[1:] {
		minX, minY = min(minX, p.x), min(minY, p.y)
		maxX, maxY = max(maxX, p.x), max(maxY, p.y)
	}
	return [4]int16{int16(minX), int16(minY), int16(maxX), int16(maxY)}
}

func appendInt16(dst []byte, v int16) []byte {
	return append(dst, byte(v>>8), byte(v))
}

func appendUint16(dst []byte, v uint16) []byte {
	return append(dst, byte(v>>8), byte(v))
}

func minInt16(a, b int16) int16 {
	if a < b {
		return a
	}
	return b
}

func maxInt16(a, b int16) int16 {
	if a > b {
		return a
	}
	return b
}

// divRound divides and rounds halves away from zero, which keeps negative
// coordinates symmetric with positive ones.
func divRound(n, d int64) int32 {
	if n >= 0 {
		return int32((n + d/2) / d)
	}
	return int32(-((-n + d/2) / d))
}
