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
	// Saturated reports that the true width of the run is larger than Width
	// because an advance or the sum of the advances left the int32 range. It
	// distinguishes an exact MaxInt32 width from a clamped one.
	Saturated bool
}

// Scale converts a value in font units into millipoints at the given size. The
// product is computed in float64, rounded once and clamped to the int32 range,
// so an extreme font size produces a saturated value instead of an
// implementation-defined one.
func Scale(v, size, unitsPerEm int32) int32 {
	if unitsPerEm == 0 {
		return 0
	}
	mp, _ := clampMP(float64(v) * float64(size) / float64(unitsPerEm))
	return mp
}

// clampMP converts a float64 millipoint value to int32, rounding once and
// clamping to the int32 range before narrowing. Every float-to-millipoint
// conversion in this package uses this one rule, so an out-of-range value
// cannot depend on the implementation's float-to-integer behaviour. It reports
// whether the value was clamped.
func clampMP(v float64) (int32, bool) {
	if math.IsNaN(v) {
		return 0, false
	}
	r := math.Round(v)
	if r > math.MaxInt32 {
		return math.MaxInt32, true
	}
	if r < math.MinInt32 {
		return math.MinInt32, true
	}
	return int32(r), false
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
	// A script whose shaping needs joining that the engine does not implement
	// is refused before a glyph is chosen: drawing it unjoined or in logical
	// order would be silently wrong, and a font that happens to carry the
	// isolated forms must not make that look like success.
	if script, ok := JoiningScript(s); ok {
		return Run{}, &UnsupportedScriptError{Script: script}
	}
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
				mp, clamped := scaleAdvance(prev.adv, size, f.UnitsPerEm())
				run.Glyphs[i-1].Advance = mp
				run.Saturated = run.Saturated || clamped
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
		advMP, advClamped := scaleAdvance(adv, size, f.UnitsPerEm())
		run.Saturated = run.Saturated || advClamped
		run.Glyphs = append(run.Glyphs, Glyph{
			GID:     gid,
			Advance: advMP,
			Rune:    r,
			XOffset: scaleFontUnits(offX, size, f.UnitsPerEm()),
			YOffset: scaleFontUnits(offY, size, f.UnitsPerEm()),
		})
	}
	width, widthClamped := AdvanceWidth(run.Glyphs)
	run.Width = width
	run.Saturated = run.Saturated || widthClamped
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
	mp, _ := scaleFontUnitsChecked(v, size, upem)
	return mp
}

// scaleFontUnitsChecked is scaleFontUnits with the saturation flag: it reports
// whether the millipoint value had to be clamped to the int32 range.
func scaleFontUnitsChecked(v int64, size, upem int32) (int32, bool) {
	if upem == 0 {
		return 0, false
	}
	if v > math.MaxInt32 {
		v = math.MaxInt32
	}
	if v < math.MinInt32 {
		v = math.MinInt32
	}
	return clampMP(float64(v) * float64(size) / float64(upem))
}

// scaleAdvance converts a font-unit advance to millipoints. Accumulated kerning
// can make a malformed font's advance negative, but a glyph advance is never
// negative: it saturates at zero and reports the clamp so the run width and the
// painted segment cannot run backwards.
func scaleAdvance(v int64, size, upem int32) (int32, bool) {
	mp, clamped := scaleFontUnitsChecked(v, size, upem)
	if mp < 0 {
		return 0, true
	}
	return mp, clamped
}

// AdvanceWidth sums the advances of a glyph run in 64-bit and clamps the sum to
// the int32 geometry range. It reports whether an advance or the sum had to be
// clamped, so the caller can distinguish an exact MaxInt32 width from a
// saturated one. A run width is never negative.
func AdvanceWidth(glyphs []Glyph) (width int32, saturated bool) {
	var sum int64
	for _, g := range glyphs {
		sum += int64(g.Advance)
	}
	if sum < 0 {
		// Glyph advances are not negative in a valid font; a malformed one can
		// only produce a negative sum, which would paint a run backwards. The
		// width saturates at zero and the caller reports it.
		return 0, true
	}
	return ClampWidth(sum)
}

// ClampWidth narrows a 64-bit width sum to the int32 geometry range and reports
// whether the sum was out of range. A sum above the range saturates at
// MaxInt32; a sum below the int32 range saturates at MinInt32. It is the one
// rule a run width and a line width use before they become int32 geometry. A
// line width may legitimately be negative when a positive content width is
// reduced by a negative text-indent, so only the range itself is clamped here.
func ClampWidth(sum int64) (int32, bool) {
	if sum > math.MaxInt32 {
		return math.MaxInt32, true
	}
	if sum < math.MinInt32 {
		return math.MinInt32, true
	}
	return int32(sum), false
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

// MetricsOf returns the vertical metrics of a font at a size. The product is
// clamped to the int32 range with the same rule as Scale, so an extreme size
// cannot narrow into a wrapped metric.
func MetricsOf(f *font.Font, size int32) Metrics {
	if f == nil || f.UnitsPerEm() == 0 {
		return Metrics{}
	}
	upem := f.UnitsPerEm()
	ascent, _ := clampMP(float64(int32(f.Ascent())) * float64(size) / float64(upem))
	descent, _ := clampMP(float64(-int32(f.Descent())) * float64(size) / float64(upem))
	return Metrics{Ascent: ascent, Descent: descent}
}

// MetricsClamped reports whether the vertical metrics of a font at a size leave
// the int32 range and are clamped by MetricsOf. Layout uses it to diagnose a
// font size that is out of range once, at the box that uses the face, instead
// of rendering wrapped geometry.
func MetricsClamped(f *font.Font, size int32) bool {
	if f == nil || f.UnitsPerEm() == 0 {
		return false
	}
	upem := f.UnitsPerEm()
	_, ascent := clampMP(float64(int32(f.Ascent())) * float64(size) / float64(upem))
	_, descent := clampMP(float64(-int32(f.Descent())) * float64(size) / float64(upem))
	return ascent || descent
}
