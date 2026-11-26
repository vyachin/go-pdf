package text

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/testfont"
)

// bundledFontData reads the bundled face's bytes. The anchor tests edit this
// fixture rather than a system font, so the crafted anchors are the same on
// every platform.
func bundledFontData(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func loadEditedFont(t *testing.T, data []byte) *font.Font {
	t.Helper()
	f, err := font.Load(data)
	if err != nil {
		t.Fatalf("loading the edited font: %v", err)
	}
	return f
}

func shapeText(t *testing.T, f *font.Font, text string) Run {
	t.Helper()
	run, err := Shape(f, 12000, text)
	if err != nil {
		t.Fatalf("Shape(%q): %v", text, err)
	}
	return run
}

func glyphOffset(run Run, i int) [2]int32 {
	return [2]int32{run.Glyphs[i].XOffset, run.Glyphs[i].YOffset}
}

// TestShapeClusterStateTable walks every transition between a base and a mark.
// A base always starts a fresh cluster: the mark after it takes the anchor of
// that base, never an anchor left over from an earlier cluster. A mark's offset
// is measured from its own pen, so a base and mark pair gives the same offset
// wherever the pair appears in a run.
func TestShapeClusterStateTable(t *testing.T) {
	f := loadBundledFont(t)
	shape := func(t *testing.T, text string) Run {
		t.Helper()
		return shapeText(t, f, text)
	}
	cases := []struct {
		name      string
		prev, cur string
		text      string
		check     func(t *testing.T, run Run)
	}{
		{
			name: "base then base", prev: "base", cur: "base",
			text: "оо",
			check: func(t *testing.T, run Run) {
				if len(run.Glyphs) != 2 {
					t.Fatalf("shaped %d glyphs, want 2", len(run.Glyphs))
				}
				for i := range run.Glyphs {
					if off := glyphOffset(run, i); off != [2]int32{} {
						t.Fatalf("base %d has offset %v", i, off)
					}
				}
			},
		},
		{
			name: "base then mark", prev: "base", cur: "mark",
			text: "о\u0301",
			check: func(t *testing.T, run Run) {
				if off := glyphOffset(run, 1); off[0] == 0 {
					t.Fatalf("the mark was drawn at the pen: %v", off)
				}
			},
		},
		{
			name: "mark then mark", prev: "mark", cur: "mark",
			text: "о\u0301\u0302",
			check: func(t *testing.T, run Run) {
				// Both marks are over the one base, so they share its anchor.
				if got, want := glyphOffset(run, 2)[0], glyphOffset(run, 1)[0]; got != want {
					t.Fatalf("second mark x offset %d, want %d (both over the same base)", got, want)
				}
				// The font's mark-to-mark anchor stacks the second mark above
				// the first; the base anchor for о + U+0302 alone would not.
				baseAnchor := shape(t, "о\u0302")
				if got, base := glyphOffset(run, 2)[1], glyphOffset(baseAnchor, 1)[1]; got == base {
					t.Fatalf("the second mark used the base anchor (y %d) instead of stacking", got)
				}
			},
		},
		{
			name: "mark then base", prev: "mark", cur: "base",
			text: "о\u0301о",
			check: func(t *testing.T, run Run) {
				if off := glyphOffset(run, 2); off != [2]int32{} {
					t.Fatalf("the base after a mark has offset %v", off)
				}
			},
		},
		{
			name: "base after a mark starts a new cluster", prev: "base", cur: "mark",
			text: "о\u0301о\u0301",
			check: func(t *testing.T, run Run) {
				// A new base clears the mark state: the second acute takes the
				// same base anchor as the first acute of an о́ pair.
				want := shape(t, "о\u0301")
				if got, expect := glyphOffset(run, 3), glyphOffset(want, 1); got != expect {
					t.Fatalf("the second acute is at %v, want the base anchor %v", got, expect)
				}
				if got, first := glyphOffset(run, 3), glyphOffset(run, 1); got != first {
					t.Fatalf("the second acute is at %v, the first at %v; both bases are the same glyph", got, first)
				}
			},
		},
		{
			name: "each acute on its own base", prev: "base", cur: "mark",
			text: "а\u0301б\u0301",
			check: func(t *testing.T, run Run) {
				for i, pair := range []string{"а\u0301", "б\u0301"} {
					want := shape(t, pair)
					if got, expect := glyphOffset(run, 2*i+1), glyphOffset(want, 1); got != expect {
						t.Fatalf("acute %d is at %v, want its base anchor %v", i, got, expect)
					}
				}
			},
		},
		{
			name: "second base keeps its own anchor", prev: "base", cur: "mark",
			text: "оо\u0301",
			check: func(t *testing.T, run Run) {
				want := shape(t, "о\u0301")
				if got, expect := glyphOffset(run, 2), glyphOffset(want, 1); got != expect {
					t.Fatalf("the acute after the second о is at %v, want %v", got, expect)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("previous glyph: %s, current glyph: %s", tc.prev, tc.cur)
			tc.check(t, shape(t, tc.text))
		})
	}
}

// TestShapeStacksMarkToMarkOrFallsBackToTheBase guards the mark-to-mark
// precedence: two marks on one base stack when the font has the anchor, and a
// font without it falls back to the current base's anchor. Neither case may
// reuse an anchor from another cluster.
func TestShapeStacksMarkToMarkOrFallsBackToTheBase(t *testing.T) {
	data := bundledFontData(t)
	t.Run("with a mark-to-mark anchor", func(t *testing.T) {
		f := loadBundledFont(t)
		run := shapeText(t, f, "о\u0301\u0302")
		if got, want := glyphOffset(run, 2)[0], glyphOffset(run, 1)[0]; got != want {
			t.Fatalf("the stacked mark is at x offset %d, want %d", got, want)
		}
		baseAnchor := shapeText(t, f, "о\u0302")
		if got, base := glyphOffset(run, 2)[1], glyphOffset(baseAnchor, 1)[1]; got == base {
			t.Fatalf("the second mark used the base anchor (y %d) instead of stacking", got)
		}
	})
	t.Run("without a mark-to-mark anchor", func(t *testing.T) {
		f := loadEditedFont(t, testfont.WithoutMarkMark(t, data))
		run := shapeText(t, f, "о\u0301\u0302")
		baseAnchor := shapeText(t, f, "о\u0302")
		if got, want := glyphOffset(run, 2), glyphOffset(baseAnchor, 1); got != want {
			t.Fatalf("the second mark is at %v, want the base anchor %v", got, want)
		}
		// A base after the first cluster still starts a new one, so the acute
		// in the second cluster attaches to its own base and not to the mark of
		// the first cluster.
		second := shapeText(t, f, "о\u0301о\u0301")
		if got, first := glyphOffset(second, 3), glyphOffset(second, 1); got != first {
			t.Fatalf("the second acute is at %v, the first at %v; both bases are the same glyph", got, first)
		}
	})
}

// TestShapeReportsAMarkWithoutABase guards the diagnostic for a run that starts
// with a combining mark: the mark has nowhere to attach, so it is reported
// instead of being drawn at the pen.
func TestShapeReportsAMarkWithoutABase(t *testing.T) {
	f := loadBundledFont(t)
	for _, text := range []string{"\u0301", "\u0301о", "\u0301\u0302"} {
		_, err := Shape(f, 12000, text)
		var missing *MissingMarkError
		if !errors.As(err, &missing) {
			t.Fatalf("Shape(%q) returned %v, want MissingMarkError", text, err)
		}
		if missing.Mark != '\u0301' || missing.Base != 0 {
			t.Fatalf("Shape(%q) reported %+v, want the mark with no base", text, missing)
		}
	}
}

// TestShapeRejectsAnAnchorOutsideTheBase guards the resolved-position check: a
// font can declare a syntactically valid base anchor that paints the mark away
// from its base. The conversion must report the missing-mark diagnostic naming
// the base instead of painting the mark there.
func TestShapeRejectsAnAnchorOutsideTheBase(t *testing.T) {
	data := bundledFontData(t)
	for _, x := range []int16{-30000, 30000} {
		f := loadEditedFont(t, testfont.WithMarkBaseAnchorsX(t, data, x))
		_, err := Shape(f, 12000, "о\u0301")
		var missing *MissingMarkError
		if !errors.As(err, &missing) {
			t.Fatalf("anchor x=%d: Shape returned %v, want MissingMarkError", x, err)
		}
		if missing.Mark != '\u0301' || missing.Base != 'о' {
			t.Fatalf("anchor x=%d: reported %+v, want the acute over о", x, missing)
		}
	}
}

// TestShapeAcceptsRealFontAnchors checks the tolerance against fixtures whose
// accents overhang their base's advance box. The bundled face is required;
// Liberation runs where it is installed and adds a second foundry's anchors.
// Both leave the pen inside the tolerance, so the conversion reports nothing.
func TestShapeAcceptsRealFontAnchors(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"),
		"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
	}
	texts := []string{"о\u0301", "о\u0301о\u0301", "а\u0301б\u0301", "о\u0301\u0302", "сло\u0301во", "a\u0316", "f\u0301", "b\u0323"}
	checked := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := font.Load(data)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		anchored := 0
		for _, text := range texts {
			_, err := Shape(f, 12000, text)
			if err == nil {
				anchored++
				continue
			}
			var missing *MissingMarkError
			if errors.As(err, &missing) && !fontHasAnchor(f, missing.Base, missing.Mark) {
				// This face has no anchor for the pair at all: Liberation's
				// Cyrillic coverage ships no GPOS marks, so there is nothing
				// for the tolerance to accept and nothing to reject.
				continue
			}
			t.Errorf("%s: Shape(%q) = %v, want a normal anchor", filepath.Base(path), text, err)
		}
		if anchored == 0 {
			t.Logf("%s: no test text has an attachment anchor in this face", filepath.Base(path))
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("the bundled font is missing")
	}
	if checked == 1 {
		t.Log("Liberation is not installed; only the bundled face was checked")
	}
}

// TestShapeAdvancesAreUnaffectedByAttachment guards that attachment only sets
// offsets: every rune still keeps its glyph and the run width stays the sum of
// the glyph advances for every regression fixture.
func TestShapeAdvancesAreUnaffectedByAttachment(t *testing.T) {
	f := loadBundledFont(t)
	for _, text := range []string{"", "о\u0301", "о\u0301о\u0301", "а\u0301б\u0301", "о\u0301\u0302", "сло\u0301во", "a\u0316"} {
		run := shapeText(t, f, text)
		if got, want := len(run.Glyphs), len([]rune(Normalize(text))); got != want {
			t.Fatalf("Shape(%q) has %d glyphs, want one per rune (%d)", text, got, want)
		}
		var sum int32
		for _, g := range run.Glyphs {
			sum += g.Advance
		}
		w, saturated := AdvanceWidth(run.Glyphs)
		if run.Width != sum || run.Width != w || saturated || run.Saturated {
			t.Fatalf("Shape(%q) width %d, sum of advances %d", text, run.Width, sum)
		}
	}
}

// fontHasAnchor reports whether the face declares an attachment anchor for the
// mark over the base. It separates "this face has no anchor for the pair" from
// "the shaper rejected an anchor the face declares", which is the defect the
// test guards.
func fontHasAnchor(f *font.Font, base, mark rune) bool {
	baseGID, ok := f.GlyphIndex(base)
	if !ok {
		return false
	}
	markGID, ok := f.GlyphIndex(mark)
	if !ok {
		return false
	}
	_, _, _, ok = f.MarkAttachment(markGID, baseGID)
	return ok
}
