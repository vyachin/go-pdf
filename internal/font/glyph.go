package font

import (
	"encoding/binary"
	"fmt"
)

// Point is a point of a glyph outline in font units, y pointing up.
type Point struct {
	X, Y    float32
	OnCurve bool
}

// Contour is a closed sequence of points. Off-curve points are quadratic
// control points, exactly as stored in the glyf table.
type Contour []Point

// Limits on the expanded outline of one glyph. A composite glyph may reference
// other composites, so a font of a few hundred bytes can describe an
// exponential number of contours; the caps keep the expansion bounded.
const (
	maxCompositeDepth   = 8
	maxExpandedContours = 1 << 14
	maxExpandedPoints   = 1 << 20
)

// ErrGlyphTooComplex reports a composite glyph whose expanded outline exceeds
// the limits above.
var ErrGlyphTooComplex = fmt.Errorf("font: composite glyph expands beyond %d contours", maxExpandedContours)

// GlyphPath returns the outlines of a glyph in font units. A composite glyph is
// expanded recursively, and the expansion is bounded: a deeply nested font is
// rejected with ErrGlyphTooComplex instead of allocating without limit.
func (f *Font) GlyphPath(gid uint16) ([]Contour, error) {
	e := &expander{cache: map[uint16][]Contour{}}
	return f.glyphPath(gid, 0, e)
}

// expander memoizes the outline of every glyph reached while expanding one
// top-level glyph. Several components often reference the same child, so
// caching turns repeated expansion into a bounded number of copies.
type expander struct {
	cache    map[uint16][]Contour
	contours int
	points   int
}

func (e *expander) take(contours []Contour) error {
	e.contours += len(contours)
	for _, c := range contours {
		e.points += len(c)
	}
	if e.contours > maxExpandedContours || e.points > maxExpandedPoints {
		return ErrGlyphTooComplex
	}
	return nil
}

func (f *Font) glyphPath(gid uint16, depth int, e *expander) ([]Contour, error) {
	if depth > maxCompositeDepth {
		return nil, nil
	}
	if cached, ok := e.cache[gid]; ok {
		return cached, e.take(cached)
	}
	data := f.glyphData(gid)
	if len(data) < 10 {
		return nil, nil
	}
	numContours := int16(binary.BigEndian.Uint16(data[0:2]))
	if numContours >= 0 {
		contours := f.simpleGlyph(data, int(numContours))
		if err := e.take(contours); err != nil {
			return nil, err
		}
		e.cache[gid] = contours
		return contours, nil
	}
	contours, err := f.compositeGlyph(data, depth, e)
	if err != nil {
		return nil, err
	}
	e.cache[gid] = contours
	return contours, nil
}

func (f *Font) simpleGlyph(data []byte, numContours int) []Contour {
	if numContours <= 0 {
		return nil
	}
	endBase := 10
	if endBase+numContours*2 > len(data) {
		return nil
	}
	endPts := make([]int, numContours)
	for i := 0; i < numContours; i++ {
		endPts[i] = int(binary.BigEndian.Uint16(data[endBase+i*2 : endBase+i*2+2]))
	}
	numPoints := endPts[numContours-1] + 1
	if numPoints <= 0 {
		return nil
	}
	pos := endBase + numContours*2
	if pos+2 > len(data) {
		return nil
	}
	instrLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2 + instrLen
	if pos > len(data) {
		return nil
	}
	flags := make([]byte, numPoints)
	for i := 0; i < numPoints; {
		if pos >= len(data) {
			return nil
		}
		fl := data[pos]
		pos++
		flags[i] = fl
		i++
		if fl&0x08 != 0 { // repeat
			if pos >= len(data) {
				return nil
			}
			repeat := int(data[pos])
			pos++
			for r := 0; r < repeat && i < numPoints; r++ {
				flags[i] = fl
				i++
			}
		}
	}
	xs := make([]int, numPoints)
	x := 0
	for i := 0; i < numPoints; i++ {
		fl := flags[i]
		switch {
		case fl&0x02 != 0:
			if pos >= len(data) {
				return nil
			}
			d := int(data[pos])
			pos++
			if fl&0x10 != 0 {
				x += d
			} else {
				x -= d
			}
		case fl&0x10 == 0:
			if pos+2 > len(data) {
				return nil
			}
			x += int(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
			pos += 2
		}
		xs[i] = x
	}
	ys := make([]int, numPoints)
	y := 0
	for i := 0; i < numPoints; i++ {
		fl := flags[i]
		switch {
		case fl&0x04 != 0:
			if pos >= len(data) {
				return nil
			}
			d := int(data[pos])
			pos++
			if fl&0x20 != 0 {
				y += d
			} else {
				y -= d
			}
		case fl&0x20 == 0:
			if pos+2 > len(data) {
				return nil
			}
			y += int(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
			pos += 2
		}
		ys[i] = y
	}
	var contours []Contour
	start := 0
	for _, end := range endPts {
		if end < start || end >= numPoints {
			return contours
		}
		c := make(Contour, 0, end-start+1)
		for i := start; i <= end; i++ {
			c = append(c, Point{X: float32(xs[i]), Y: float32(ys[i]), OnCurve: flags[i]&0x01 != 0})
		}
		contours = append(contours, c)
		start = end + 1
	}
	return contours
}

func (f *Font) compositeGlyph(data []byte, depth int, e *expander) ([]Contour, error) {
	pos := 10
	var out []Contour
	for {
		if pos+4 > len(data) {
			return out, nil
		}
		flags := binary.BigEndian.Uint16(data[pos : pos+2])
		component := binary.BigEndian.Uint16(data[pos+2 : pos+4])
		pos += 4
		var dx, dy float32
		if flags&0x0001 != 0 {
			if pos+4 > len(data) {
				return out, nil
			}
			dx = float32(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
			dy = float32(int16(binary.BigEndian.Uint16(data[pos+2 : pos+4])))
			pos += 4
		} else {
			if pos+2 > len(data) {
				return out, nil
			}
			dx = float32(int8(data[pos]))
			dy = float32(int8(data[pos+1]))
			pos += 2
		}
		a, b, c, d := float32(1), float32(0), float32(0), float32(1)
		switch {
		case flags&0x0008 != 0:
			if pos+2 > len(data) {
				return out, nil
			}
			s := f2dot14(data[pos : pos+2])
			pos += 2
			a, d = s, s
		case flags&0x0040 != 0:
			if pos+4 > len(data) {
				return out, nil
			}
			a = f2dot14(data[pos : pos+2])
			d = f2dot14(data[pos+2 : pos+4])
			pos += 4
		case flags&0x0080 != 0:
			if pos+8 > len(data) {
				return out, nil
			}
			a = f2dot14(data[pos : pos+2])
			b = f2dot14(data[pos+2 : pos+4])
			c = f2dot14(data[pos+4 : pos+6])
			d = f2dot14(data[pos+6 : pos+8])
			pos += 8
		}
		child, err := f.glyphPath(component, depth+1, e)
		if err != nil {
			return nil, err
		}
		for _, contour := range child {
			transformed := make(Contour, len(contour))
			for i, p := range contour {
				transformed[i] = Point{
					X:       a*p.X + c*p.Y + dx,
					Y:       b*p.X + d*p.Y + dy,
					OnCurve: p.OnCurve,
				}
			}
			out = append(out, transformed)
		}
		if flags&0x0020 == 0 { // MORE_COMPONENTS
			return out, nil
		}
	}
}

func f2dot14(b []byte) float32 {
	return float32(int16(binary.BigEndian.Uint16(b))) / 16384
}

// components returns the glyph indices referenced by a composite glyph.
func (f *Font) components(gid uint16) []uint16 {
	data := f.glyphData(gid)
	if len(data) < 12 || int16(binary.BigEndian.Uint16(data[0:2])) >= 0 {
		return nil
	}
	pos := 10
	var out []uint16
	for {
		if pos+4 > len(data) {
			return out
		}
		flags := binary.BigEndian.Uint16(data[pos : pos+2])
		component := binary.BigEndian.Uint16(data[pos+2 : pos+4])
		out = append(out, component)
		pos += 4
		if flags&0x0001 != 0 {
			pos += 4
		} else {
			pos += 2
		}
		switch {
		case flags&0x0008 != 0:
			pos += 2
		case flags&0x0040 != 0:
			pos += 4
		case flags&0x0080 != 0:
			pos += 8
		}
		if flags&0x0020 == 0 {
			return out
		}
	}
}
