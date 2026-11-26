package layout

import (
	"sort"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
)

// rtlGlyph is one painted glyph with the x position layout gave it.
type rtlGlyph struct {
	r  rune
	x  int32
	y  int32
	op *TextOp
}

// rtlGlyphs returns every painted glyph in content-stream order, which is
// the logical order the text layer keeps.
func rtlGlyphs(doc *Document) []rtlGlyph {
	var out []rtlGlyph
	for _, op := range textOps(doc) {
		x := op.X
		for _, g := range op.Run.Glyphs {
			out = append(out, rtlGlyph{r: g.Rune, x: x, y: op.Y, op: op})
			x += g.Advance
		}
	}
	return out
}

// rtlVisualString returns the glyphs sorted by their painted x position, the order
// a reader sees them in.
func rtlVisualString(glyphs []rtlGlyph) string {
	sorted := append([]rtlGlyph(nil), glyphs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].x < sorted[j].x })
	var b strings.Builder
	for _, g := range sorted {
		b.WriteRune(g.r)
	}
	return b.String()
}

// xOf returns the painted x of a rune, failing the test when it is absent.
func rtlXOf(t *testing.T, glyphs []rtlGlyph, r rune) int32 {
	t.Helper()
	for _, g := range glyphs {
		if g.r == r {
			return g.x
		}
	}
	t.Fatalf("rune %q is not painted", r)
	return 0
}

// TestHebrewLinePaintsRightToLeft guards RV5-04: the first logical word is
// painted on the right and the glyphs of the word run right to left, while the
// content stream and therefore the text layer stay logical.
func TestHebrewLinePaintsRightToLeft(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p style="margin:0">שלום עולם</p>`)
	if got, want := textOf(doc), "שלום עולם"; got != want {
		t.Fatalf("text layer = %q, want the logical sequence %q", got, want)
	}
	glyphs := rtlGlyphs(doc)
	if got, want := rtlVisualString(glyphs), "םלוע םולש"; got != want {
		t.Fatalf("visual sequence = %q, want %q", got, want)
	}
	first := []rune("שלום")
	for i := 1; i < len(first); i++ {
		if rtlXOf(t, glyphs, first[i]) >= rtlXOf(t, glyphs, first[i-1]) {
			t.Fatalf("the first word does not run right to left: %q x = %d, %q x = %d",
				first[i-1], rtlXOf(t, glyphs, first[i-1]), first[i], rtlXOf(t, glyphs, first[i]))
		}
	}
}

// TestMixedDirectionLine guards the mixed fixture: Latin runs keep their
// order, the Hebrew run is reversed, the digits keep their order, and every
// character is painted exactly once.
func TestMixedDirectionLine(t *testing.T) {
	h := newHarness(t)
	const src = "abc שלום 123"
	doc := h.run(`<p style="margin:0">` + src + `</p>`)
	if got := textOf(doc); got != src {
		t.Fatalf("text layer = %q, want %q", got, src)
	}
	glyphs := rtlGlyphs(doc)
	if len(glyphs) != len([]rune(src)) {
		t.Fatalf("painted %d glyphs for %d characters", len(glyphs), len([]rune(src)))
	}
	if got, want := rtlVisualString(glyphs), "abc 123 םולש"; got != want {
		t.Fatalf("visual sequence = %q, want %q", got, want)
	}
	// Digits keep their left-to-right order.
	for _, pair := range [][2]rune{{'1', '2'}, {'2', '3'}} {
		if rtlXOf(t, glyphs, pair[0]) >= rtlXOf(t, glyphs, pair[1]) {
			t.Fatalf("digits are not left to right: %q x = %d, %q x = %d",
				pair[0], rtlXOf(t, glyphs, pair[0]), pair[1], rtlXOf(t, glyphs, pair[1]))
		}
	}
	// The Latin word keeps its order.
	for _, pair := range [][2]rune{{'a', 'b'}, {'b', 'c'}} {
		if rtlXOf(t, glyphs, pair[0]) >= rtlXOf(t, glyphs, pair[1]) {
			t.Fatalf("Latin runs are not left to right: %q x = %d, %q x = %d",
				pair[0], rtlXOf(t, glyphs, pair[0]), pair[1], rtlXOf(t, glyphs, pair[1]))
		}
	}
}

// TestDirPrecedence guards the fixture set of RV5-04: an explicit dir wins over
// first-strong detection, auto selects the first strong character, and a
// nested dir shadows its parent.
func TestDirPrecedence(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// first is the rune painted leftmost; its identity shows the base
		// direction that was chosen.
		first rune
	}{
		{"rtl wins over a Latin first strong", `<p dir="rtl" style="margin:0">abc שלום 123</p>`, '1'},
		{"ltr wins over a Hebrew first strong", `<p dir="ltr" style="margin:0">שלוםa</p>`, 'ם'},
		{"auto selects a Latin first strong", `<p dir="auto" style="margin:0">abc שלום 123</p>`, 'a'},
		{"auto selects a Hebrew first strong", `<p dir="auto" style="margin:0">שלוםa</p>`, 'a'},
		{"no dir selects the first strong character", `<p style="margin:0">שלוםa</p>`, 'a'},
		{"a nested ltr shadows an rtl parent", `<div dir="rtl"><p dir="ltr" style="margin:0">abc שלום 123</p></div>`, 'a'},
		{"a nested rtl shadows an ltr parent", `<div dir="ltr"><p dir="rtl" style="margin:0">abc שלום 123</p></div>`, '1'},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			doc := h.run(c.src)
			glyphs := rtlGlyphs(doc)
			var leftmost *rtlGlyph
			for i := range glyphs {
				if leftmost == nil || glyphs[i].x < leftmost.x {
					leftmost = &glyphs[i]
				}
			}
			if leftmost == nil || leftmost.r != c.first {
				t.Fatalf("leftmost glyph = %v, want %q", leftmost, c.first)
			}
		})
	}
}

// TestDirResolutionInTheBoxBuilder guards that the dir attribute reaches the box
// style and inherits, so the paragraph direction has the nearest ancestor as
// its source.
func TestDirResolutionInTheBoxBuilder(t *testing.T) {
	h := newHarness(t)
	root := h.buildTree(`<div dir="rtl"><p dir="ltr">a</p><p>b</p><p dir="auto">c</p><p dir="bogus">d</p></div>`)
	div := root.Children[0]
	if div.Style.Dir != css.DirRTL {
		t.Fatalf("div dir = %v, want rtl", div.Style.Dir)
	}
	// The div holds one anonymous block around its inline text plus the four
	// paragraphs; find the paragraphs by their node tag.
	var ps []*Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b.Node != nil && b.Node.Tag == "p" {
			ps = append(ps, b)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(root)
	if len(ps) != 4 {
		t.Fatalf("found %d paragraphs, want 4", len(ps))
	}
	want := []css.Direction{css.DirLTR, css.DirRTL, css.DirAuto, css.DirRTL}
	for i, p := range ps {
		if p.Style.Dir != want[i] {
			t.Errorf("paragraph %d dir = %v, want %v", i, p.Style.Dir, want[i])
		}
	}
}

// TestArabicIsRefusedAtTheElement guards RV5-04: a joining script is refused
// with the documented code at the owning element and nothing is painted.
func TestArabicIsRefusedAtTheElement(t *testing.T) {
	h := newHarness(t)
	doc, diags := h.runRaw(`<p>مرحبا بالعالم</p>`, h.ctx.PageSize, h.ctx.Margin)
	found := false
	for _, d := range diags {
		if d.Code != "text/unsupported-script" {
			continue
		}
		found = true
		if d.Severity != 0 {
			t.Errorf("diagnostic is not an error: %s", d.String())
		}
		if d.Pos.Line == 0 || d.Element == "" {
			t.Errorf("diagnostic does not name its position: %s", d.String())
		}
	}
	if !found {
		t.Fatalf("no text/unsupported-script diagnostic: %v", diags.Lines())
	}
	if got := textOf(doc); got != "" {
		t.Fatalf("Arabic text was painted as %q", got)
	}
}

// lineSpan returns the leftmost and rightmost painted x of the text operations
// sharing the first baseline of a document.
func lineSpan(t *testing.T, doc *Document) (int32, int32) {
	t.Helper()
	glyphs := rtlGlyphs(doc)
	if len(glyphs) == 0 {
		t.Fatal("no text is painted")
	}
	firstY := glyphs[0].y
	minX, maxX := glyphs[0].x, glyphs[0].x
	for _, g := range glyphs {
		if g.y != firstY {
			continue
		}
		if g.x < minX {
			minX = g.x
		}
		adv := int32(0)
		for _, gl := range g.op.Run.Glyphs {
			adv = satAdd(adv, gl.Advance)
		}
		if end := satAdd(g.x, adv); end > maxX {
			maxX = end
		}
	}
	return minX, maxX
}

// TestJustificationStretchesRTLSpaces guards that justification still spreads
// the line: the justified first line fills the content width while the same
// line left-aligned stays narrower.
func TestJustificationStretchesRTLSpaces(t *testing.T) {
	h := newHarness(t)
	const words = "שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם"
	justified := h.run(`<p style="margin:0; text-align:justify">` + words + `</p>`)
	left := h.run(`<p style="margin:0">` + words + `</p>`)
	jMin, jMax := lineSpan(t, justified)
	lMin, lMax := lineSpan(t, left)
	if want := h.ctx.PageSize.Width - 2*h.ctx.Margin[1]; jMax-jMin != want {
		t.Fatalf("justified first line spans %d, want the %d-wide content box", jMax-jMin, want)
	}
	if lMax-lMin >= jMax-jMin {
		t.Fatalf("left-aligned first line spans %d, the justified one %d; justification did not stretch it", lMax-lMin, jMax-jMin)
	}
	strip := func(v string) string { return strings.ReplaceAll(v, " ", "") }
	if strip(textOf(justified)) != strip(words) {
		t.Fatalf("text layer changed: %q", textOf(justified))
	}
}

// TestMarginBoxHebrewIsOrdered guards that a page margin box follows the same
// visual placement as body text.
func TestMarginBoxHebrewIsOrdered(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>@page { @bottom-center { content: "שלום עולם" } }</style><p style="margin:0">body</p>`)
	_, margin := splitByHeight(doc)
	var glyphs []rtlGlyph
	for _, op := range margin {
		x := op.Text.X
		for _, g := range op.Text.Run.Glyphs {
			glyphs = append(glyphs, rtlGlyph{r: g.Rune, x: x, y: op.Text.Y, op: op.Text})
			x += g.Advance
		}
	}
	if len(glyphs) == 0 {
		t.Fatal("the margin box painted nothing")
	}
	if got, want := rtlVisualString(glyphs), "םלוע םולש"; got != want {
		t.Fatalf("margin box visual sequence = %q, want %q", got, want)
	}
	var stream strings.Builder
	for _, g := range glyphs {
		stream.WriteRune(g.r)
	}
	if got, want := stream.String(), "שלום עולם"; got != want {
		t.Fatalf("margin box text layer = %q, want %q", got, want)
	}
}
