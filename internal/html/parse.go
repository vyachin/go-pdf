package html

import (
	"bytes"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/diag"
)

// TruncatedError reports that the document ends inside an HTML tag. The tree
// construction drops an incomplete tag at the end of the input, which would
// convert a shorter document than the author wrote, so the parse fails instead.
type TruncatedError struct {
	// Offset is where the incomplete tag begins.
	Offset int
	// Position is Offset as a file position, using the file the parse was
	// given.
	Position diag.Position
}

func (e *TruncatedError) Error() string {
	return "the document ends inside an HTML tag"
}

// ParseOptions configures one parse.
type ParseOptions struct {
	// File is used only for positions in diagnostics.
	File string
	// MaxDepth bounds element nesting; zero means unlimited. A document nested
	// deeper is rejected before the tree allocates that many nodes.
	MaxDepth int
	// Budget records parser work and reports cancellation. Nil means unlimited
	// work and no cancellation.
	Budget *budget.Guard
	// Charset names the input encoding explicitly. It wins over the document's
	// own declaration and its byte order mark. Empty means detect: the byte
	// order mark, then a <meta charset> or http-equiv declaration in the first
	// 1024 bytes, then UTF-8. An encoding outside the supported set fails with
	// an encoding error.
	Charset string
}

// Parse tokenizes src and builds a document tree. The file argument is used
// only for positions in diagnostics.
func Parse(src []byte, file string) (*Document, error) {
	return ParseWithOptions(src, ParseOptions{File: file})
}

// ParseWithOptions parses src under the given limits and cancellation.
//
// It runs two passes over the input: a token pass that records every token's
// source range under the work budget and the nesting limit, and the standard
// tree-construction pass. The tree is then converted into this package's node
// model, taking each element's position from the recorded range of its start
// tag. An element the tree construction implied has no start tag of its own, so
// it inherits the range of its nearest positioned ancestor.
func ParseWithOptions(src []byte, opts ParseOptions) (*Document, error) {
	if err := opts.Budget.Err(); err != nil {
		return nil, err
	}
	enc, err := charset.Resolve(src, opts.Charset)
	if err != nil {
		return nil, err
	}
	dec, err := charset.Decode(enc, src)
	if err != nil {
		return nil, err
	}
	// The scan and the tree construction run over the decoded text, so node
	// ranges and inline <style> offsets share one coordinate space. A position
	// is translated back to the source bytes through the mapper.
	d := &Document{File: opts.File, src: src, text: dec.Text, charset: enc, Root: &Node{Type: DocumentNode}}
	d.lines = lineStarts(dec.Text)
	if dec.Widths != nil || dec.Skip != 0 {
		d.mapper = charset.NewMapper(src, dec.Text, dec.Widths, dec.Skip)
	}
	sc, err := scan(dec.Text, opts)
	if err != nil {
		return nil, err
	}
	if sc.truncated >= 0 {
		return nil, &TruncatedError{Offset: sc.truncated, Position: d.Position(sc.truncated)}
	}
	tree, err := xhtml.Parse(bytes.NewReader(dec.Text))
	if err != nil {
		return nil, err
	}
	b := &builder{doc: d, scan: sc, cursors: map[uint16]int{}}
	b.children(tree, d.Root)
	return d, nil
}

func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// builder converts a standard tree into this package's model.
type builder struct {
	doc  *Document
	scan *scanResult
	// cursors holds, per interned tag name, the next unconsumed start-tag span.
	cursors map[uint16]int
	// spanCur is the next span the text lookup may use, and lower is the
	// smallest offset an unvisited node can start at. Spans below lower belong
	// to content the tree construction dropped, such as white space before
	// <html>, so they are skipped instead of being given to the next node.
	spanCur int
	lower   int
}

func (b *builder) children(n *xhtml.Node, parent *Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.node(c, parent)
	}
}

func (b *builder) node(n *xhtml.Node, parent *Node) {
	switch n.Type {
	case xhtml.ElementNode:
		el := &Node{Type: ElementNode, Tag: n.Data, Attrs: attrsOf(n)}
		start, end, ok := b.startPosition(n.Data)
		b.place(el, parent, start, end, ok)
		if ok && end > b.lower {
			b.lower = end
		}
		parent.AppendChild(el)
		b.children(n, el)
	case xhtml.TextNode:
		b.textNode(n, parent)
	case xhtml.CommentNode:
		parent.AppendChild(&Node{Type: CommentNode, Data: n.Data, Start: parent.Start, End: parent.End})
	case xhtml.DoctypeNode:
		parent.AppendChild(&Node{Type: DoctypeNode, Data: n.Data, Start: parent.Start, End: parent.End})
	}
}

// textNode appends character data, merging it into the previous text node when
// there is one, so a run of text between two elements is a single node.
func (b *builder) textNode(n *xhtml.Node, parent *Node) {
	start, end, ok := b.textPosition()
	data := n.Data
	// A style or script element holds its source verbatim: the tree's text is
	// unescaped and has its line endings normalized, which would shift the byte
	// offsets the CSS parser maps back to the document.
	if ok && (parent.Tag == "style" || parent.Tag == "script") {
		// The offsets are decoded ones, so the slice is of the decoded text.
		data = string(b.doc.text[start:end])
	}
	// White space between table structure takes no part in layout.
	if tableStructure[parent.Tag] && strings.TrimSpace(data) == "" {
		return
	}
	if last := lastChild(parent); last != nil && last.Type == TextNode {
		last.Data += data
		if ok {
			last.End = end
		}
		return
	}
	tn := &Node{Type: TextNode, Data: data}
	b.place(tn, parent, start, end, ok)
	parent.AppendChild(tn)
}

// place sets the node's range, falling back to its parent's when the node has
// no start tag of its own.
func (b *builder) place(n *Node, parent *Node, start, end int, ok bool) {
	if !ok {
		start, end = parent.Start, parent.End
	}
	n.Start, n.End = start, end
}

// startPosition returns the range of the next start tag with the given name.
func (b *builder) startPosition(tag string) (int, int, bool) {
	idx, ok := b.scan.index[normalTagName(tag)]
	if !ok {
		return 0, 0, false
	}
	list := b.scan.startSpans[idx]
	cur := b.cursors[idx]
	if cur >= len(list) {
		return 0, 0, false
	}
	b.cursors[idx] = cur + 1
	s := b.scan.spans[list[cur]]
	return int(s.start), int(s.end), true
}

// textPosition returns the range of the next text token at or after the current
// document position.
func (b *builder) textPosition() (int, int, bool) {
	for b.spanCur < len(b.scan.spans) {
		s := b.scan.spans[b.spanCur]
		b.spanCur++
		if int(s.start) < b.lower {
			continue
		}
		if s.kind != spanText {
			continue
		}
		b.lower = int(s.end)
		return int(s.start), int(s.end), true
	}
	return 0, 0, false
}

// lastChild returns the last child of n, or nil.
func lastChild(n *Node) *Node {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[len(n.Children)-1]
}

// tableStructure is the set of elements that hold table structure rather than
// content.
var tableStructure = map[string]bool{
	"table": true, "thead": true, "tbody": true, "tfoot": true,
	"tr": true, "colgroup": true,
}

// attrsOf converts the standard tree's attributes into this package's form. An
// attribute in a namespace keeps its prefix, so "xlink:href" survives.
func attrsOf(n *xhtml.Node) []Attr {
	if len(n.Attr) == 0 {
		return nil
	}
	attrs := make([]Attr, 0, len(n.Attr))
	for _, a := range n.Attr {
		key := a.Key
		if a.Namespace != "" {
			key = a.Namespace + ":" + a.Key
		}
		attrs = append(attrs, Attr{Key: key, Value: a.Val})
	}
	return attrs
}
