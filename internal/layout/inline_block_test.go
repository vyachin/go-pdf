package layout

import (
	"strings"
	"testing"
)

// imageOps returns the image operations of the first page.
func imageOps(doc *Document) []Op {
	var out []Op
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind == ImageOpKind {
				out = append(out, op)
			}
		}
	}
	return out
}

// textOpTexts returns the text runs of the first page in painting order.
func textOpTexts(doc *Document) []string {
	var out []string
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind == TextOpKind && op.Text != nil {
				var b strings.Builder
				for _, g := range op.Text.Run.Glyphs {
					b.WriteRune(g.Rune)
				}
				out = append(out, b.String())
			}
		}
	}
	return out
}

// TestBlockImageInsideSpanIsPainted guards the review reproduction: the wrapped
// block image used to disappear, because the builder replaced the block child
// with a line break. It must paint exactly like the unwrapped image.
func TestBlockImageInsideSpanIsPainted(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "logo.png", 40, 20)
	wrapped := h.run(`<p><span><img src="logo.png" style="display:block"></span></p>`)
	plain := h.run(`<p><img src="logo.png" style="display:block"></p>`)
	wrappedOps, plainOps := imageOps(wrapped), imageOps(plain)
	if len(wrappedOps) != 1 {
		t.Fatalf("the wrapped image produced %d image operations, want 1", len(wrappedOps))
	}
	if len(plainOps) != 1 {
		t.Fatalf("the plain image produced %d image operations, want 1", len(plainOps))
	}
	if wrappedOps[0].Rect.W != plainOps[0].Rect.W || wrappedOps[0].Rect.H != plainOps[0].Rect.H {
		t.Fatalf("wrapped image is %dx%d, plain is %dx%d",
			wrappedOps[0].Rect.W, wrappedOps[0].Rect.H, plainOps[0].Rect.W, plainOps[0].Rect.H)
	}
}

// TestTextAroundBlockImageKeepsOrder guards the split: text before and after the
// block stays in the document order around it.
func TestTextAroundBlockImageKeepsOrder(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "logo.png", 40, 20)
	doc := h.run(`<p><span>BEFORE<img src="logo.png" style="display:block">AFTER</span></p>`)
	var before, image, after int
	for i, page := range doc.Pages {
		_ = i
		for _, op := range page.Ops {
			switch op.Kind {
			case TextOpKind:
				text := ""
				for _, g := range op.Text.Run.Glyphs {
					text += string(g.Rune)
				}
				switch {
				case strings.Contains(text, "BEFORE") && before == 0:
					before = 1
				case strings.Contains(text, "AFTER") && image > 0 && after == 0:
					after = 1
				}
			case ImageOpKind:
				image = 1
			}
		}
	}
	if before != 1 || image != 1 || after != 1 {
		t.Fatalf("order broken: before=%d image=%d after=%d; ops=%v", before, image, after, textOpTexts(doc))
	}
}

// TestInheritedStyleSurvivesTheSplit guards the fragment style: the text on both
// sides of the block still inherits the wrapper's colour.
func TestInheritedStyleSurvivesTheSplit(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "logo.png", 40, 20)
	doc := h.run(`<p><span style="color:#ff0000">LEFT<img src="logo.png" style="display:block">RIGHT</span></p>`)
	seen := 0
	for _, page := range doc.Pages {
		for _, op := range page.Ops {
			if op.Kind != TextOpKind || op.Text == nil {
				continue
			}
			if !op.Text.Color.Set || op.Text.Color.R != 255 || op.Text.Color.G != 0 {
				t.Fatalf("text colour = %+v, want the inherited red", op.Text.Color)
			}
			seen++
		}
	}
	if seen != 2 {
		t.Fatalf("painted %d text runs, want 2", seen)
	}
}

// TestHiddenBlockInsideSpanStaysHidden guards display:none and the HTML hidden
// attribute: they still omit the descendant instead of painting it.
func TestHiddenBlockInsideSpanStaysHidden(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "logo.png", 40, 20)
	for _, src := range []string{
		`<p><span><img src="logo.png" style="display:block;display:none"></span></p>`,
		`<p><span><img src="logo.png" style="display:block" hidden></span></p>`,
	} {
		doc := h.run(src)
		if ops := imageOps(doc); len(ops) != 0 {
			t.Fatalf("%s painted %d images", src, len(ops))
		}
	}
}

// TestAnchorStaysOnTheSplitWrapper guards anchor ownership: the id of the
// wrapper is registered even though the wrapper became several fragments.
func TestAnchorStaysOnTheSplitWrapper(t *testing.T) {
	h := newHarness(t)
	writeCoverPNG(t, h.images.dir, "logo.png", 40, 20)
	doc := h.run(`<p><span id="split">BEFORE<img src="logo.png" style="display:block">AFTER</span></p>`)
	if _, ok := doc.Anchors["split"]; !ok {
		t.Fatal("the anchor of the split wrapper is missing")
	}
}

// TestNestedTableInsideSpanIsBuilt guards a non-image block descendant: a table
// inside an inline wrapper keeps its cells instead of being flattened.
func TestNestedTableInsideSpanIsBuilt(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<p><span><table><tr><td>CELL-A</td><td>CELL-B</td></tr></table></span></p>`)
	// Runs are split at spaces, so join without a separator.
	texts := strings.Join(textOpTexts(doc), "")
	if !strings.Contains(texts, "CELL-A") || !strings.Contains(texts, "CELL-B") {
		t.Fatalf("the table cells are missing: %v", textOpTexts(doc))
	}
}

// TestNestedListInsideSpanIsBuilt guards list structure inside an inline
// wrapper: the marker and the item text survive.
func TestNestedListInsideSpanIsBuilt(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<div><span><ul><li>LIST-ITEM</li></ul></span></div>`)
	texts := strings.Join(textOpTexts(doc), "")
	if !strings.Contains(texts, "LIST-ITEM") {
		t.Fatalf("the list item is missing: %v", textOpTexts(doc))
	}
	if !strings.Contains(texts, "•") && !strings.Contains(texts, "\u2022") {
		t.Fatalf("the list marker is missing: %v", textOpTexts(doc))
	}
}
