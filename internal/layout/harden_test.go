package layout

import (
	"strings"
	"testing"
)

// TestGeneratedContentDecodesEscapes guards the rendered value: a hexadecimal
// escape in content is an em dash, not the characters "2014".
func TestGeneratedContentDecodesEscapes(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<style>a::after { content: "\2014" }</style><p><a href="#">link</a></p>`)
	if !strings.Contains(pageText(doc, 1), "\u2014") {
		t.Fatalf("page text = %q, want an em dash", pageText(doc, 1))
	}
}

// TestTransparentBorderPaintsNothing guards that a transparent border keeps its
// layout width but produces no pixels.
func TestTransparentBorderPaintsNothing(t *testing.T) {
	h := newHarness(t)
	doc := h.run(`<div style="border: 5pt solid transparent; background-color: #ff0000">x</div>`)
	fills := 0
	for _, op := range doc.Pages[0].Ops {
		if op.Kind == FillOpKind {
			fills++
		}
	}
	if fills != 1 {
		t.Fatalf("page has %d fills, want only the background", fills)
	}
}
