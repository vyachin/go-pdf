// Package text turns strings into positioned glyphs: normalisation, shaping,
// metrics and line breaking rules.
package text

import (
	"fmt"
	"math"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/font"
)

// Glyph is a positioned glyph with its advance in millipoints.
type Glyph struct {
	GID     uint16
	Advance int32
	// Rune is the character this glyph came from, used for the ToUnicode map.
	Rune rune
	// XOffset and YOffset move a combining mark from the pen to the anchor its
	// font places over the base glyph, in millipoints. YOffset is positive up.
	XOffset, YOffset int32
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
//
// A combining mark that normalisation leaves as a separate code point is
// positioned with the font's GPOS mark attachment: the mark's anchor is placed
// on the anchor of the preceding base or mark glyph. A font without a usable
// mark lookup for it is an error, so the mark is never drawn at the pen.
func Shape(f *font.Font, size int32, s string) (Run, error) {
	return ShapeWithOptions(f, size, s, ShapeOptions{})
}

// ShapeOptions carries the conversion's work guard into shaping.
type ShapeOptions struct {
	// Guard bounds the work and carries cancellation. Nil means unlimited.
	Guard *budget.Guard
}

// ShapeWithOptions is Shape with a work guard. Shaping charges one unit per
// rune it normalizes, maps and measures, before the glyph slice is allocated,
// so a single huge word cannot allocate memory out of proportion to the budget.
func ShapeWithOptions(f *font.Font, size int32, s string, opts ShapeOptions) (Run, error) {
	// Charge the rune count before converting the string: a two-megabyte word
	// must fail the budget before it allocates its rune slice, not after.
	if n := utf8.RuneCountInString(s); n > 0 {
		if err := opts.Guard.Add(uint64(n)); err != nil {
			return Run{}, err
		}
	}
	runes := []rune(Normalize(s))
	run := Run{Glyphs: make([]Glyph, 0, len(runes))}
	// Only three states are ever needed: the previous glyph for kerning, and
	// the reference base and mark for attachment. Keeping one entry per rune
	// doubled the per-rune memory of shaping.
	type glyphState struct {
		gid  uint16
		pen  int64 // pen before the glyph, font units
		adv  int64 // advance including kerning, font units
		offX int64 // attached mark offset, font units
		offY int64
	}
	var prev, baseState, markState glyphState
	var hasPrev, hasBase, hasMark bool
	lastBase := -1
	var lastBaseRune rune
	for i, r := range runes {
		gid, ok := f.GlyphIndex(r)
		if !ok && r == '\u00a0' {
			// A font without a dedicated no-break space draws the ordinary
			// space glyph; the run keeps U+00A0 so copied text is unchanged.
			gid, ok = f.GlyphIndex(' ')
		}
		if !ok {
			return Run{}, &MissingGlyphError{Rune: r, Family: f.Family()}
		}
		adv := int64(f.Advance(gid))
		pen := int64(0)
		if hasPrev {
			if k := f.Kerning(prev.gid, gid); k != 0 {
				prev.adv += int64(k)
				run.Glyphs[i-1].Advance = Scale(int32(prev.adv), size, f.UnitsPerEm())
			}
			pen = prev.pen + prev.adv
		}
		mark := unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
		var offX, offY int64
		if mark {
			attached := false
			if hasMark {
				// MarkMark: the previous mark is the reference, so a second
				// accent stacks on the first rather than on the base.
				if dx, dy, kind, ok := f.MarkAttachment(gid, markState.gid); ok && kind == 6 {
					offX = markState.pen + markState.offX + int64(dx) - pen
					offY = markState.offY + int64(dy)
					attached = true
				}
			}
			if !attached && hasBase {
				// MarkBase or MarkLig: the reference is the base glyph's pen.
				if dx, dy, kind, ok := f.MarkAttachment(gid, baseState.gid); ok && kind != 6 {
					offX = baseState.pen + int64(dx) - pen
					offY = int64(dy)
					attached = true
				}
			}
			if !attached {
				base := rune(0)
				if lastBase >= 0 {
					base = lastBaseRune
				}
				return Run{}, &MissingMarkError{Mark: r, Base: base, Family: f.Family()}
			}
		}
		st := glyphState{gid: gid, pen: pen, adv: adv, offX: offX, offY: offY}
		if mark {
			markState, hasMark = st, true
		} else {
			baseState, hasBase = st, true
			lastBase = i
			lastBaseRune = r
		}
		prev, hasPrev = st, true
		run.Glyphs = append(run.Glyphs, Glyph{
			GID:     gid,
			Advance: Scale(int32(adv), size, f.UnitsPerEm()),
			Rune:    r,
			XOffset: scaleFontUnits(offX, size, f.UnitsPerEm()),
			YOffset: scaleFontUnits(offY, size, f.UnitsPerEm()),
		})
	}
	run.Width = AdvanceWidth(run.Glyphs)
	return run, nil
}

// scaleFontUnits converts a font-unit offset to millipoints, clamping an
// extreme value instead of narrowing it.
func scaleFontUnits(v int64, size, upem int32) int32 {
	if upem == 0 {
		return 0
	}
	if v > math.MaxInt32 {
		v = math.MaxInt32
	}
	if v < math.MinInt32 {
		v = math.MinInt32
	}
	return Scale(int32(v), size, upem)
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

// MissingMarkError reports a combining mark the font cannot attach to a base.
// Drawing it at the pen would put the accent on the next letter, so the
// conversion stops and names the mark.
type MissingMarkError struct {
	Mark   rune
	Base   rune
	Family string
}

func (e *MissingMarkError) Error() string {
	base := ""
	if e.Base != 0 {
		base = " over " + quoteRune(e.Base)
	}
	return fmt.Sprintf("no mark attachment for %s%s in font %s", quoteRune(e.Mark), base, strconv.Quote(e.Family))
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
