package layout

import (
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

// srcRef is where one inline piece came from: the source position of the text
// that produced it and the node that owns it.
//
// The node is kept instead of its rendered path because building a path charges
// the work budget, and a piece that never fails must not pay for a path it never
// reports. A generated piece carries both the generating declaration and the
// element it belongs to, so a margin box and an a::after both name their
// declaration while the element path stays available.
type srcRef struct {
	// pos is the position of the source in the original file: the text node,
	// the element that owns generated content, or the declaration that
	// generated text.
	pos diag.Position
	// node is the owning element; nil when the piece has no element.
	node *html.Node
}

// at resolves the reference for a diagnostic: the position and the element path
// of the source. A zero position means the source is unknown, and the caller
// reports an unknown location rather than inventing coordinates.
func (s srcRef) at() (diag.Position, string) {
	var element string
	if s.node != nil {
		element = s.node.Path()
	}
	return s.pos, element
}

// sourceOf returns the provenance of one node: the position of its first byte in
// the original file and the node itself.
func (l *layouter) sourceOf(n *html.Node) srcRef {
	if n == nil || l.source == nil {
		return srcRef{}
	}
	return srcRef{pos: l.source.Position(n.Start), node: n}
}

// generatedSource returns the provenance of generated text: the declaration that
// produced it when the cascade recorded one, and the element that owns it.
func (l *layouter) generatedSource(b *Box, items []css.ContentItem) srcRef {
	ref := l.sourceOf(b.Node)
	for _, item := range items {
		if item.Pos.Line > 0 {
			ref.pos = item.Pos
			break
		}
	}
	return ref
}
