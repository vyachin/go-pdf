package text

import (
	"strings"
	"testing"
)

// visualText renders the visual sequence of s from the runs VisualRuns
// reports. A right-to-left run is painted from right to left, so its runes are
// reversed; a left-to-right run keeps them.
func visualText(s string, runs []VisualRun) string {
	runes := []rune(s)
	var b strings.Builder
	for _, run := range runs {
		part := runes[run.Start:run.End]
		if run.RTL {
			for i := len(part) - 1; i >= 0; i-- {
				b.WriteRune(part[i])
			}
			continue
		}
		b.WriteString(string(part))
	}
	return b.String()
}

// TestVisualRunsPureHebrew guards the core reordering: the whole line is one
// right-to-left run, so its glyphs are painted in reverse.
func TestVisualRunsPureHebrew(t *testing.T) {
	const s = "שלום עולם"
	runs := VisualRuns(s, DirectionAuto)
	if len(runs) != 1 {
		t.Fatalf("VisualRuns(%q) = %v, want one run", s, runs)
	}
	run := runs[0]
	if !run.RTL {
		t.Fatalf("the Hebrew run is not right-to-left: %v", run)
	}
	if run.Start != 0 || run.End != len([]rune(s)) {
		t.Fatalf("run range = %d..%d, want the whole line", run.Start, run.End)
	}
	if got, want := visualText(s, runs), "םלוע םולש"; got != want {
		t.Fatalf("visual sequence = %q, want %q", got, want)
	}
}

// TestVisualRunsPureLatin guards that a left-to-right line keeps its order and
// produces exactly one run.
func TestVisualRunsPureLatin(t *testing.T) {
	const s = "latin text 123"
	runs := VisualRuns(s, DirectionAuto)
	if len(runs) != 1 || runs[0].RTL {
		t.Fatalf("VisualRuns(%q) = %v, want one left-to-right run", s, runs)
	}
	if got := visualText(s, runs); got != s {
		t.Fatalf("visual sequence = %q, want %q", got, s)
	}
}

// TestVisualRunsNeutrals guards that a line with only neutrals follows the
// requested base direction instead of guessing.
func TestVisualRunsNeutrals(t *testing.T) {
	const s = " -- ( )"
	if runs := VisualRuns(s, DirectionLTR); len(runs) != 1 || runs[0].RTL {
		t.Fatalf("left-to-right neutrals = %v, want one left-to-right run", runs)
	}
	if runs := VisualRuns(s, DirectionRTL); len(runs) != 1 || !runs[0].RTL {
		t.Fatalf("right-to-left neutrals = %v, want one right-to-left run", runs)
	}
}

// TestVisualRunsMixedDirection guards UAX#9 rule L2: the digits after a Hebrew
// word are raised to level 2, so they move with the reversed block and keep
// their own left-to-right order.
func TestVisualRunsMixedDirection(t *testing.T) {
	const s = "abc שלום 123"
	runs := VisualRuns(s, DirectionLTR)
	if got, want := visualText(s, runs), "abc 123 םולש"; got != want {
		t.Fatalf("visual sequence = %q, want %q (runs %v)", got, want, runs)
	}
	// The Latin word stays first, the digits follow it and the Hebrew run is
	// last, painted right to left.
	if len(runs) != 3 {
		t.Fatalf("runs = %v, want three", runs)
	}
	if runs[0].RTL || runs[0].Start != 0 || runs[0].End != 4 {
		t.Fatalf("first run = %v, want the leading Latin word", runs[0])
	}
	if runs[1].RTL || runs[1].Start != 9 || runs[1].End != 12 {
		t.Fatalf("second run = %v, want the digits", runs[1])
	}
	if !runs[2].RTL || runs[2].Start != 4 || runs[2].End != 9 {
		t.Fatalf("third run = %v, want the Hebrew word", runs[2])
	}
}

// TestVisualRunsExplicitBase guards that an explicit base direction wins: a
// right-to-left line of Latin text is still one left-to-right run, and a
// left-to-right line of Hebrew is one right-to-left run.
func TestVisualRunsExplicitBase(t *testing.T) {
	if runs := VisualRuns("abc", DirectionRTL); len(runs) != 1 || runs[0].RTL {
		t.Fatalf("abc with an rtl base = %v, want one left-to-right run", runs)
	}
	if runs := VisualRuns("שלום", DirectionLTR); len(runs) != 1 || !runs[0].RTL {
		t.Fatalf("Hebrew with an ltr base = %v, want one right-to-left run", runs)
	}
}

// TestStrongDirection guards first-strong detection: digits and punctuation
// are skipped, and text with no strong character has none.
func TestStrongDirection(t *testing.T) {
	cases := []struct {
		s    string
		dir  Direction
		ok   bool
		name string
	}{
		{"שלום", DirectionRTL, true, "Hebrew"},
		{"abc", DirectionLTR, true, "Latin"},
		{"123 abc", DirectionLTR, true, "a leading number is skipped"},
		{"123 שלום", DirectionRTL, true, "a leading number is skipped before Hebrew"},
		{"123 --", DirectionLTR, false, "neutrals only"},
		{"", DirectionLTR, false, "empty"},
	}
	for _, c := range cases {
		dir, ok := StrongDirection(c.s)
		if ok != c.ok || (ok && dir != c.dir) {
			t.Errorf("%s: StrongDirection(%q) = %v, %v; want %v, %v", c.name, c.s, dir, ok, c.dir, c.ok)
		}
	}
}
