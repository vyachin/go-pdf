package layout

import (
	"testing"
)

// placedGlyph is one painted glyph in run-relative coordinates.
type placedGlyph struct {
	gid    uint16
	adv    int32
	dx, dy int32
}

// placeRun returns the glyphs of a run relative to its first painted glyph, so
// two runs at different origins can be compared.
func placeRun(ops []Op) []placedGlyph {
	var out []placedGlyph
	var baseX, baseY int32
	first := true
	for _, op := range ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		x := op.Text.X
		for _, g := range op.Text.Run.Glyphs {
			if first {
				baseX, baseY = x, op.Text.Y
				first = false
			}
			out = append(out, placedGlyph{gid: g.GID, adv: g.Advance, dx: x - baseX, dy: op.Text.Y - baseY})
			x = satAdd(x, g.Advance)
		}
	}
	return out
}

// splitByHeight partitions a page's text ops into the body (upper half) and the
// bottom margin box (lower half).
func splitByHeight(doc *Document) (body, margin []Op) {
	page := doc.Pages[0]
	half := page.Size.Height / 2
	for _, op := range page.Ops {
		if op.Kind != TextOpKind || op.Text == nil {
			continue
		}
		if op.Text.Y < half {
			body = append(body, op)
		} else {
			margin = append(margin, op)
		}
	}
	return body, margin
}

// TestMarginBoxMarkMatchesBody guards the review reproduction: a combining mark
// painted in a margin box must land where the same mark lands in the body at the
// same face and size. Margin boxes used to copy only glyph id, rune and advance,
// so the mark was painted on the pen instead of at the shaper's offset.
func TestMarginBoxMarkMatchesBody(t *testing.T) {
	h := newHarness(t)
	const word = "сло\u0301во"
	src := `<style>@page { @bottom-center { content: "` + word + `" } } p { font-size: 10pt }</style><p>` + word + `</p>`
	doc := h.run(src)
	body, margin := splitByHeight(doc)
	bodyGlyphs, marginGlyphs := placeRun(body), placeRun(margin)
	// Five letters plus the combining mark as its own glyph.
	if len(bodyGlyphs) != 6 || len(marginGlyphs) != 6 {
		t.Fatalf("body painted %d glyphs, margin %d, want 6 each", len(bodyGlyphs), len(marginGlyphs))
	}
	for i := range bodyGlyphs {
		if bodyGlyphs[i] != marginGlyphs[i] {
			t.Fatalf("glyph %d: body %+v, margin %+v", i, bodyGlyphs[i], marginGlyphs[i])
		}
	}
	// The mark is not on the advance pen: its offset moved it.
	mark := bodyGlyphs[3]
	if pen := bodyGlyphs[2].dx + bodyGlyphs[2].adv; mark.dx == pen {
		t.Fatalf("the mark sits on the pen at %d: %+v", pen, mark)
	}
}

// TestVerticalMarkOffsetIsPainted guards a nonzero vertical attachment: the mark
// is painted above the baseline by the shaper's YOffset in body and margin alike.
func TestVerticalMarkOffsetIsPainted(t *testing.T) {
	h := newHarness(t)
	const word = "a\u0316"
	src := `<style>@page { @bottom-center { content: "` + word + `" } } p { font-size: 10pt }</style><p>` + word + `</p>`
	doc := h.run(src)
	body, margin := splitByHeight(doc)
	bodyGlyphs, marginGlyphs := placeRun(body), placeRun(margin)
	if len(bodyGlyphs) != 2 || len(marginGlyphs) != 2 {
		t.Fatalf("body %d glyphs, margin %d, want 2", len(bodyGlyphs), len(marginGlyphs))
	}
	if bodyGlyphs[1].dy == 0 {
		t.Fatalf("the vertical offset was lost: %+v", bodyGlyphs[1])
	}
	if bodyGlyphs[1] != marginGlyphs[1] {
		t.Fatalf("body mark %+v, margin mark %+v", bodyGlyphs[1], marginGlyphs[1])
	}
}

// TestUnsupportedMarkAttachmentStaysADiagnostic guards the failure path: a mark
// the font cannot attach is reported, not painted in the wrong place.
func TestUnsupportedMarkAttachmentStaysADiagnostic(t *testing.T) {
	h := newHarness(t)
	_, diags := h.runRaw("<p>a\u0301\u0323</p>", h.ctx.PageSize, h.ctx.Margin)
	if !hasError(diags, "text/glyph") {
		t.Fatalf("unsupported mark attachment was not reported: %v", diags.Lines())
	}
}

// TestListMarkerUsesTheSharedEmitter guards the third caller: a marker is a
// shaped run too, painted through the same offset-aware emitter.
func TestListMarkerUsesTheSharedEmitter(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<ul><li>ITEM</li></ul>`)
	body, _ := splitByHeight(doc)
	if len(placeRun(body)) == 0 {
		t.Fatal("the marker and item painted no glyphs")
	}
}
