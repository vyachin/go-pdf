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
		// An empty outline would make the glyph silently disappear from the
		// page and from the subset; the caller reports the unsupported glyph
		// instead.
		return nil, fmt.Errorf("%w: a composite glyph nests deeper than %d levels", ErrGlyphTooComplex, maxCompositeDepth)
	}
	if cached, ok := e.cache[gid]; ok {
		return cached, e.take(cached)
	}
	data := f.glyphData(gid)
	if len(data) == 0 {
		// Equal loca offsets describe a legal empty glyph: space and some
		// .notdef glyphs have no data at all. This is not a truncation.
		return nil, nil
	}
	if len(data) < 10 {
		return nil, fmt.Errorf("font: glyph %d is truncated: %d bytes of glyph data, want at least 10", gid, len(data))
	}
	numContours := int16(binary.BigEndian.Uint16(data[0:2]))
	if numContours >= 0 {
		contours, err := f.simpleGlyph(data, int(numContours), gid)
		if err != nil {
			return nil, err
		}
		if err := e.take(contours); err != nil {
			return nil, err
		}
		e.cache[gid] = contours
		return contours, nil
	}
	contours, err := f.compositeGlyph(data, depth, e, gid)
	if err != nil {
		return nil, err
	}
	e.cache[gid] = contours
	return contours, nil
}

func truncatedGlyph(gid uint16, where string) error {
	return fmt.Errorf("font: glyph %d is truncated in its %s", gid, where)
}

func (f *Font) simpleGlyph(data []byte, numContours int, gid uint16) ([]Contour, error) {
	if numContours <= 0 {
		// A simple glyph with no contours is an empty outline with a header.
		return nil, nil
	}
	endBase := 10
	if endBase+numContours*2 > len(data) {
		return nil, truncatedGlyph(gid, "contour endpoints")
	}
	endPts := make([]int, numContours)
	for i := 0; i < numContours; i++ {
		endPts[i] = int(binary.BigEndian.Uint16(data[endBase+i*2 : endBase+i*2+2]))
	}
	numPoints := endPts[numContours-1] + 1
	if numPoints <= 0 {
		return nil, fmt.Errorf("font: glyph %d declares no points", gid)
	}
	pos := endBase + numContours*2
	if pos+2 > len(data) {
		return nil, truncatedGlyph(gid, "instruction length")
	}
	instrLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2 + instrLen
	if pos > len(data) {
		return nil, truncatedGlyph(gid, "instructions")
	}
	flags := make([]byte, numPoints)
	for i := 0; i < numPoints; {
		if pos >= len(data) {
			return nil, truncatedGlyph(gid, "flags")
		}
		fl := data[pos]
		pos++
		flags[i] = fl
		i++
		if fl&0x08 != 0 { // repeat
			if pos >= len(data) {
				return nil, truncatedGlyph(gid, "flag repeat count")
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
				return nil, truncatedGlyph(gid, "x coordinates")
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
				return nil, truncatedGlyph(gid, "x coordinates")
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
				return nil, truncatedGlyph(gid, "y coordinates")
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
				return nil, truncatedGlyph(gid, "y coordinates")
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
			return nil, fmt.Errorf("font: glyph %d has an inconsistent contour endpoint %d", gid, end)
		}
		c := make(Contour, 0, end-start+1)
		for i := start; i <= end; i++ {
			c = append(c, Point{X: float32(xs[i]), Y: float32(ys[i]), OnCurve: flags[i]&0x01 != 0})
		}
		contours = append(contours, c)
		start = end + 1
	}
	return contours, nil
}

func (f *Font) compositeGlyph(data []byte, depth int, e *expander, gid uint16) ([]Contour, error) {
	pos := 10
	var out []Contour
	for {
		if pos+4 > len(data) {
			return nil, truncatedGlyph(gid, "component records")
		}
		flags := binary.BigEndian.Uint16(data[pos : pos+2])
		component := binary.BigEndian.Uint16(data[pos+2 : pos+4])
		if component >= f.numGlyphs {
			return nil, fmt.Errorf("font: composite glyph %d references component %d outside the %d glyphs", gid, component, f.numGlyphs)
		}
		pos += 4
		var dx, dy float32
		if flags&0x0001 != 0 {
			if pos+4 > len(data) {
				return nil, truncatedGlyph(gid, "component offset")
			}
			dx = float32(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
			dy = float32(int16(binary.BigEndian.Uint16(data[pos+2 : pos+4])))
			pos += 4
		} else {
			if pos+2 > len(data) {
				return nil, truncatedGlyph(gid, "component offset")
			}
			dx = float32(int8(data[pos]))
			dy = float32(int8(data[pos+1]))
			pos += 2
		}
		if flags&0x0002 == 0 {
			// ARGS_ARE_XY_VALUES is clear: the two arguments are point indices
			// (point matching), not an offset. Drawing them as coordinates
			// silently places the component somewhere else, which is worse
			// than refusing the glyph.
			return nil, fmt.Errorf("font: a composite glyph places a component by point matching, which is not supported")
		}
		a, b, c, d := float32(1), float32(0), float32(0), float32(1)
		switch {
		case flags&0x0008 != 0:
			if pos+2 > len(data) {
				return nil, truncatedGlyph(gid, "component scale")
			}
			s := f2dot14(data[pos : pos+2])
			pos += 2
			a, d = s, s
		case flags&0x0040 != 0:
			if pos+4 > len(data) {
				return nil, truncatedGlyph(gid, "component scale")
			}
			a = f2dot14(data[pos : pos+2])
			d = f2dot14(data[pos+2 : pos+4])
			pos += 4
		case flags&0x0080 != 0:
			if pos+8 > len(data) {
				return nil, truncatedGlyph(gid, "component transform")
			}
			a = f2dot14(data[pos : pos+2])
			b = f2dot14(data[pos+2 : pos+4])
			c = f2dot14(data[pos+4 : pos+6])
			d = f2dot14(data[pos+6 : pos+8])
			pos += 8
		}
		if flags&0x0800 != 0 {
			// SCALED_COMPONENT_OFFSET: the offset is expressed in the child's
			// coordinate system, so the component's scale matrix transforms it.
			dx, dy = a*dx+c*dy, b*dx+d*dy
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

// components returns the glyph indices referenced by a composite glyph. A
// component id at or beyond numGlyphs, and a record that ends inside its data,
// are errors: dropping either would leave a dangling or partial reference in
// the subset.
func (f *Font) components(gid uint16) ([]uint16, error) {
	data := f.glyphData(gid)
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) < 2 {
		return nil, truncatedGlyph(gid, "glyph header")
	}
	if int16(binary.BigEndian.Uint16(data[0:2])) >= 0 {
		return nil, nil
	}
	if len(data) < 10 {
		return nil, truncatedGlyph(gid, "composite header")
	}
	pos := 10
	var out []uint16
	for {
		if pos+4 > len(data) {
			return nil, truncatedGlyph(gid, "component records")
		}
		flags := binary.BigEndian.Uint16(data[pos : pos+2])
		component := binary.BigEndian.Uint16(data[pos+2 : pos+4])
		if component >= f.numGlyphs {
			return nil, fmt.Errorf("font: composite glyph %d references component %d outside the %d glyphs", gid, component, f.numGlyphs)
		}
		out = append(out, component)
		pos += 4
		size := 2
		if flags&0x0001 != 0 {
			size = 4
		}
		switch {
		case flags&0x0008 != 0:
			size += 2
		case flags&0x0040 != 0:
			size += 4
		case flags&0x0080 != 0:
			size += 8
		}
		if pos+size > len(data) {
			return nil, truncatedGlyph(gid, "component data")
		}
		pos += size
		if flags&0x0020 == 0 {
			return out, nil
		}
	}
}
