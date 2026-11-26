package text

import (
	"unicode"

	"golang.org/x/text/unicode/bidi"
)

// Direction is a base direction for bidirectional text.
type Direction int

const (
	// DirectionAuto selects the base direction from the first strong character
	// of the text, then left-to-right. It is what an absent or auto dir
	// attribute means.
	DirectionAuto Direction = iota
	// DirectionLTR is an explicit left-to-right base direction.
	DirectionLTR
	// DirectionRTL is an explicit right-to-left base direction.
	DirectionRTL
)

// StrongDirection returns the direction of the first strong character of s and
// whether s contains one. Digits and punctuation are not strong, as UAX#9 P2
// defines them.
func StrongDirection(s string) (Direction, bool) {
	for _, r := range s {
		switch bidiClass(r) {
		case bidi.L:
			return DirectionLTR, true
		case bidi.R, bidi.AL:
			return DirectionRTL, true
		}
	}
	return DirectionLTR, false
}

// FirstStrongDirection returns the direction of the first strong character of
// s, or left-to-right when there is none. It is the base direction of a
// paragraph that declares dir="auto" or no direction at all.
func FirstStrongDirection(s string) Direction {
	if d, ok := StrongDirection(s); ok {
		return d
	}
	return DirectionLTR
}

// IsRightToLeft reports whether r belongs to a right-to-left script or is an
// Arabic number, so a line containing it must be ordered by the bidi pass.
func IsRightToLeft(r rune) bool {
	switch bidiClass(r) {
	case bidi.R, bidi.AL, bidi.AN:
		return true
	}
	return false
}

// IsMark reports whether r is a combining mark the shaper keeps in its base's
// cluster. A positioned run must keep a mark next to the glyph it attaches to.
func IsMark(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

// VisualRun is one maximal run of a single embedding level, reported in
// visual order. Start and End are rune offsets into the logical text, End
// exclusive. RTL reports that the run is laid out right-to-left; its glyphs
// are painted from right to left even though the run keeps the logical order
// of the shaped glyphs.
type VisualRun struct {
	Start, End int
	RTL        bool
}

// VisualRuns orders the runs of s for painting. It applies UAX#9 through
// golang.org/x/text/unicode/bidi: the paragraph level comes from base, and when
// base is DirectionAuto from the first strong character. The result lists the
// maximal same-level runs in the order they are painted, left to right, so a
// caller can place each run at the current pen and advance by its width.
//
// The rune ranges are logical offsets: a right-to-left run is reported with
// its logical range and RTL set, and the caller paints its glyphs from right to
// left. Reordering runs rather than their glyphs keeps the shaped order and
// therefore pair kerning and mark attachment inside a run.
func VisualRuns(s string, base Direction) []VisualRun {
	if s == "" {
		return nil
	}
	runes := []rune(s)
	eff := base
	if eff == DirectionAuto {
		eff = FirstStrongDirection(s)
	}
	runs, ok := paragraphRuns(s, eff)
	if !ok || len(runes) == 0 {
		return []VisualRun{{Start: 0, End: len(runes), RTL: eff == DirectionRTL}}
	}
	levels := runeLevels(runes, runs, eff)
	return reorderSegments(levelSegments(levels))
}

// logicalRun is one run the bidi algorithm reports, in logical order.
type logicalRun struct {
	start, end int
	rtl        bool
}

// paragraphRuns asks the bidi implementation for the runs of s with the given
// paragraph level. It reports false when the standard library refuses the
// input, which lets the caller fall back to the paragraph direction.
func paragraphRuns(s string, eff Direction) ([]logicalRun, bool) {
	dir := bidi.LeftToRight
	if eff == DirectionRTL {
		dir = bidi.RightToLeft
	}
	var p bidi.Paragraph
	if _, err := p.SetString(s, bidi.DefaultDirection(dir)); err != nil {
		return nil, false
	}
	order, err := p.Order()
	if err != nil {
		return nil, false
	}
	runs := make([]logicalRun, 0, order.NumRuns())
	for i := 0; i < order.NumRuns(); i++ {
		r := order.Run(i)
		start, end := r.Pos()
		if start < 0 || end < start {
			continue
		}
		runs = append(runs, logicalRun{start: start, end: end + 1, rtl: r.Direction() == bidi.RightToLeft})
	}
	return runs, len(runs) > 0
}

// runeLevels derives one embedding level per rune. The bidi package reports
// runs by direction but not their level, and rule L2 needs the exact level to
// reverse a block of right-to-left and nested left-to-right text as one unit.
//
// A right-to-left run is level 1. In a right-to-left paragraph every
// left-to-right run is the nested level 2. In a left-to-right paragraph a
// left-to-right run is level 0, except for a number that follows a
// right-to-left strong character: W7 leaves such a European number anchored to
// the right-to-left context and I1 raises it to level 2, so the number moves
// with the reversed block instead of staying beside the Latin runs.
func runeLevels(runes []rune, runs []logicalRun, eff Direction) []int {
	levels := make([]int, len(runes))
	for _, run := range runs {
		lvl := 0
		if run.rtl {
			lvl = 1
		} else if eff == DirectionRTL {
			lvl = 2
		}
		for i := run.start; i < run.end && i < len(levels); i++ {
			levels[i] = lvl
		}
	}
	if eff == DirectionRTL {
		return levels
	}
	// W7: a European number after a left-to-right strong character is
	// left-to-right itself; after a right-to-left strong character it stays a
	// number and is raised by I1. Arabic numbers are always raised.
	prevStrong := bidi.L
	for i, r := range runes {
		switch bidiClass(r) {
		case bidi.L:
			prevStrong = bidi.L
		case bidi.R, bidi.AL:
			prevStrong = bidi.R
		case bidi.EN:
			if levels[i] == 0 && prevStrong == bidi.R {
				levels[i] = 2
			}
		case bidi.AN:
			if levels[i] == 0 {
				levels[i] = 2
			}
		case bidi.NSM:
			if i > 0 {
				levels[i] = levels[i-1]
			}
		}
	}
	// W4, W5 and W6: a separator inside a number belongs to the number, so it
	// travels with the raised run instead of breaking it.
	for i := range runes {
		if levels[i] != 0 {
			continue
		}
		switch bidiClass(runes[i]) {
		case bidi.ES, bidi.CS:
			if i > 0 && i+1 < len(runes) && isNumberClass(bidiClass(runes[i-1])) && isNumberClass(bidiClass(runes[i+1])) {
				levels[i] = levels[i-1]
			}
		case bidi.ET:
			if i > 0 && levels[i-1] == 2 {
				levels[i] = 2
			} else if i+1 < len(runes) && levels[i+1] == 2 {
				levels[i] = 2
			}
		}
	}
	return levels
}

func isNumberClass(c bidi.Class) bool { return c == bidi.EN || c == bidi.AN }

// levelSegment is a maximal run of one embedding level in logical order.
type levelSegment struct {
	start, end int
	level      int
}

// levelSegments splits the per-rune levels into maximal same-level runs.
func levelSegments(levels []int) []levelSegment {
	var out []levelSegment
	for i := 0; i < len(levels); {
		j := i + 1
		for j < len(levels) && levels[j] == levels[i] {
			j++
		}
		out = append(out, levelSegment{start: i, end: j, level: levels[i]})
		i = j
	}
	return out
}

// reorderSegments applies UAX#9 rule L2 to the level runs: at every level from
// the highest down, a contiguous sequence whose level is at least that level is
// reversed. The resulting order is the visual order of the runs.
func reorderSegments(segs []levelSegment) []VisualRun {
	order := make([]int, len(segs))
	maxLevel := 0
	for i := range segs {
		order[i] = i
		if segs[i].level > maxLevel {
			maxLevel = segs[i].level
		}
	}
	for lvl := maxLevel; lvl >= 1; lvl-- {
		for i := 0; i < len(order); {
			if segs[order[i]].level < lvl {
				i++
				continue
			}
			j := i + 1
			for j < len(order) && segs[order[j]].level >= lvl {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
	}
	out := make([]VisualRun, 0, len(segs))
	for _, idx := range order {
		seg := segs[idx]
		out = append(out, VisualRun{Start: seg.start, End: seg.end, RTL: seg.level%2 == 1})
	}
	return out
}

func bidiClass(r rune) bidi.Class {
	props, _ := bidi.LookupRune(r)
	return props.Class()
}
