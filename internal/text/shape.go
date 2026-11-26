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
// positioned with the font's GPOS mark attachment, inside the cluster of the
// glyph it follows: it is placed on the current base's anchor, or mark-to-mark
// on the previous mark of the same cluster, and the resolved position is
// checked to stay over that base. A font without a usable anchor for it is an
// error, so the mark is never drawn at the pen.
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
	// Only three states are ever needed: the previous glyph for kerning, the
	// current cluster's base, and the last mark attached to it. Keeping one
	// entry per rune doubled the per-rune memory of shaping. The mark is part
	// of the cluster, not a second reference that outlives the base: a non-mark
	// glyph clears it, so a mark never reads an anchor from an earlier cluster.
	type glyphState struct {
		gid  uint16
		pen  int64 // pen before the glyph, font units
		adv  int64 // advance including kerning, font units
		offX int64 // attached mark offset, font units
		offY int64
	}
	var prev, base, clusterMark glyphState
	var hasPrev, hasBase, hasClusterMark bool
	var baseRune rune
	// An attached mark may sit outside its base's advance box by up to this
	// much; see markOverhangLimit.
	overhang := int64(f.UnitsPerEm()) * markOverhangLimit / 1000
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
			if hasClusterMark {
				// MarkMark: only a mark of the current cluster can be the
				// reference, so a second accent stacks on the first of the
				// same cluster rather than on an older one.
				if dx, dy, kind, ok := f.MarkAttachment(gid, clusterMark.gid); ok && kind == markMarkLookup {
					offX = clusterMark.pen + clusterMark.offX + int64(dx) - pen
					offY = clusterMark.offY + int64(dy)
					attached = true
				}
			}
			if !attached && hasBase {
				// MarkBase or MarkLig: the reference is the current cluster's
				// base, never a mark left over from an earlier cluster.
				if dx, dy, kind, ok := f.MarkAttachment(gid, base.gid); ok && kind != markMarkLookup {
					offX = base.pen + int64(dx) - pen
					offY = int64(dy)
					attached = true
				}
			}
			if !attached || !markPositionOK(pen+offX, base.pen, base.adv, pen+adv, overhang) {
				// A font can return an anchor that would paint the mark away
				// from its base, and a run can start with a mark that has no
				// base at all. Both are the missing-mark diagnostic: drawing
				// the mark at the pen would put the accent on another letter.
				return Run{}, &MissingMarkError{Mark: r, Base: baseRune, Family: f.Family()}
			}
		}
		st := glyphState{gid: gid, pen: pen, adv: adv, offX: offX, offY: offY}
		if mark {
			clusterMark, hasClusterMark = st, true
		} else {
			base, hasBase = st, true
			baseRune = r
			// A new base starts a new cluster: any mark attached before it is
			// no longer a valid mark-to-mark reference.
			clusterMark, hasClusterMark = glyphState{}, false
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

// markMarkLookup is the GPOS lookup kind font.MarkAttachment reports for a
// mark-to-mark anchor. A base anchor (MarkBase or MarkLig) is any other kind.
const markMarkLookup = 6

// markOverhangLimit is how far, in thousandths of an em, an attached mark's pen
// may sit outside its base's advance box and still count as over it. Fonts put
// mark anchors over the ink of the base rather than over its advance box, so
// the pen legitimately leaves the box for an overhanging accent: Arial draws
// U+0323 on U+0285 with the pen 615 units past a 455-unit box at 2048 units per
// em (300 thousandths), and Times New Roman is close behind. Half an em covers
// those faces; a mark whose anchor belongs to another cluster or leaves the run
// is still refused. A later reader can widen this with a font that needs more.
const markOverhangLimit = 500

// markPositionOK reports whether a mark painted at markPen, in font units,
// stays over its base's advance box, which starts at basePen and is baseAdv
// wide, and inside the shaped run up to runEnd. overhang is the tolerance in
// font units described by markOverhangLimit.
func markPositionOK(markPen, basePen, baseAdv, runEnd, overhang int64) bool {
	if markPen < basePen-overhang || markPen > basePen+baseAdv+overhang {
		return false
	}
	return markPen >= -overhang && markPen <= runEnd+overhang
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
