// Package text turns strings into positioned glyphs: normalisation, shaping,
// metrics and line breaking rules.
package text

import (
	"fmt"
	"math"
	"strconv"

	"github.com/vyachin/go-pdf/internal/font"
)

// Glyph is a positioned glyph with its advance in millipoints.
type Glyph struct {
	GID     uint16
	Advance int32
	// Rune is the character this glyph came from, used for the ToUnicode map.
	Rune rune
}

// Run is a shaped string.
type Run struct {
	Glyphs []Glyph
	Width  int32
}

// Scale converts a value in font units into millipoints at the given size.
func Scale(v, size, unitsPerEm int32) int32 {
	if unitsPerEm == 0 {
		return 0
	}
	return int32(math.Round(float64(v) * float64(size) / float64(unitsPerEm)))
}

// Shape maps text to glyphs using one font. It reports an error naming the
// character when the font has no glyph for it, because silently dropping text
// from a document is never acceptable.
func Shape(f *font.Font, size int32, s string) (Run, error) {
	runes := []rune(Normalize(s))
	var run Run
	run.Glyphs = make([]Glyph, 0, len(runes))
	havePrev := false
	var prev uint16
	for _, r := range runes {
		gid, ok := f.GlyphIndex(r)
		if !ok {
			return Run{}, &MissingGlyphError{Rune: r, Family: f.Family()}
		}
		adv := Scale(f.Advance(gid), size, f.UnitsPerEm())
		if havePrev {
			if k := Scale(f.Kerning(prev, gid), size, f.UnitsPerEm()); k != 0 {
				run.Glyphs[len(run.Glyphs)-1].Advance += k
			}
		}
		run.Glyphs = append(run.Glyphs, Glyph{GID: gid, Advance: adv, Rune: r})
		prev = gid
		havePrev = true
	}
	run.Width = AdvanceWidth(run.Glyphs)
	return run, nil
}

// AdvanceWidth sums the advances of a glyph run.
func AdvanceWidth(glyphs []Glyph) int32 {
	var w int32
	for _, g := range glyphs {
		w += g.Advance
	}
	return w
}

// MissingGlyphError reports a character absent from the selected font.
type MissingGlyphError struct {
	Rune   rune
	Family string
}

func (e *MissingGlyphError) Error() string {
	return fmt.Sprintf("no glyph for %s in font %s", quoteRune(e.Rune), strconv.Quote(e.Family))
}

func quoteRune(r rune) string {
	return fmt.Sprintf("'%s' (U+%04X)", string(r), r)
}

// Metrics holds vertical font metrics in millipoints.
type Metrics struct {
	// Ascent is the distance from the baseline to the ascender.
	Ascent int32
	// Descent is the distance from the baseline to the descender, positive.
	Descent int32
}

// MetricsOf returns the vertical metrics of a font at a size.
func MetricsOf(f *font.Font, size int32) Metrics {
	return Metrics{
		Ascent:  Scale(int32(f.Ascent()), size, f.UnitsPerEm()),
		Descent: Scale(-int32(f.Descent()), size, f.UnitsPerEm()),
	}
}
