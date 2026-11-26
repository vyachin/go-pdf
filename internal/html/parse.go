package html

import (
	"bytes"
	"fmt"
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
	d := &Document{File: opts.File, src: src, text: dec.Text, charset: enc, guard: opts.Budget, Root: &Node{Type: DocumentNode}}
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
	tree, err := parseStandardTree(dec.Text)
	if err != nil {
		return nil, err
	}
	if err := checkTreeDepth(tree, opts); err != nil {
		return nil, err
	}
	b := &builder{
		doc:      d,
		scan:     sc,
		guard:    opts.Budget,
		cursors:  map[int32]int{},
		used:     make([]bool, len(sc.spans)),
		next:     map[string]int{},
		trimNext: map[string]int{},
	}
	b.children(tree, d.Root)
	if b.failure != nil {
		return nil, b.failure
	}
	attachBudget(d.Root, opts.Budget)
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

// ParseError reports a failure of the standard tree construction that has no
// token position of its own. Code is the diagnostic code the engine reports.
type ParseError struct {
	Code string
	Err  error
}

func (e *ParseError) Error() string { return fmt.Sprintf("%s: %v", e.Code, e.Err) }

func (e *ParseError) Unwrap() error { return e.Err }

// parseStandardTree runs the standard tree construction. Its open-stack limit is
// reported as a resource limit; any other failure becomes a coded parse error,
// and a panic in a third-party parser is turned into an error as well, so no
// input can take the host process down.
func parseStandardTree(text []byte) (tree *xhtml.Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			tree, err = nil, &ParseError{Code: string(diag.HTMLParse), Err: fmt.Errorf("tree construction failed: %v", r)}
		}
	}()
	tree, err = xhtml.Parse(bytes.NewReader(text))
	if err != nil {
		if strings.Contains(err.Error(), "open stack of elements exceeds") {
			return nil, fmt.Errorf("%w: %v", budget.ErrExceeded, err)
		}
		return nil, &ParseError{Code: string(diag.HTMLParse), Err: err}
	}
	return tree, nil
}

// checkTreeDepth measures element nesting on the tree the standard construction
// algorithm produced, after implied elements and implied end tags, and charges
// one unit per element visited. It runs before the conversion model, so an
// over-limit document fails before layout allocates.
func checkTreeDepth(root *xhtml.Node, opts ParseOptions) error {
	if opts.MaxDepth <= 0 {
		return nil
	}
	type item struct {
		node  *xhtml.Node
		depth int
	}
	stack := []item{{root, 0}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		depth := it.depth
		if it.node.Type == xhtml.ElementNode {
			depth++
			if err := opts.Budget.Add(1); err != nil {
				return err
			}
			if depth > opts.MaxDepth {
				return fmt.Errorf("%w: element nesting exceeds %d levels", budget.ErrExceeded, opts.MaxDepth)
			}
		}
		for child := it.node.FirstChild; child != nil; child = child.NextSibling {
			stack = append(stack, item{node: child, depth: depth})
		}
	}
	return nil
}

// hit is one text span that matches a key, with the start a node takes from it.
type hit struct {
	span  int32
	start int32
}

// builder converts a standard tree into this package's model.
type builder struct {
	doc   *Document
	scan  *scanResult
	guard *budget.Guard
	// failure is the guard's verdict, kept so the walk stops at the next node
	// instead of finishing a model that will be discarded.
	failure error
	// cursors holds, per interned tag name, the next unconsumed start-tag span.
	cursors map[int32]int
	// spanCur is the next span the text lookup may use, and lower is the
	// smallest offset an unvisited node can start at. Spans below lower belong
	// to content the tree construction dropped, such as white space before
	// <html>, so they are skipped instead of being given to the next node.
	spanCur int
	lower   int
	// used records the spans already given to a node, so an equal text found
	// out of source order takes the next occurrence instead of another node's
	// span.
	used []bool
	// exact maps a decoded text to its occurrences in document order; trim does
	// the same for a token whose leading newline the tree dropped. Both are
	// built on the first miss.
	exact map[string][]hit
	trim  map[string][]hit
	// next and trimNext hold the next unconsumed occurrence per key.
	next     map[string]int
	trimNext map[string]int
}

func (b *builder) children(n *xhtml.Node, parent *Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if b.failure != nil {
			return
		}
		b.node(c, parent)
	}
}

func (b *builder) node(n *xhtml.Node, parent *Node) {
	if b.failure != nil {
		return
	}
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
	data := n.Data
	start, end, ok := b.textSpanFor(data, parent)
	// A style or script element holds its source verbatim: the tree's text is
	// unescaped and has its line endings normalized, which would shift the byte
	// offsets the CSS parser maps back to the document.
	if ok && (parent.Tag == "style" || parent.Tag == "script") {
		// The span was verified against the node's own text, so the decoded
		// slice is this element's content.
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

// charge records work against the shared guard and keeps its verdict.
func (b *builder) charge(n uint64) {
	if b.failure != nil {
		return
	}
	if err := b.guard.Add(n); err != nil {
		b.failure = err
	}
}

// textSpanFor returns the range of the text token whose decoded text is data.
// Foster parenting reorders the tree, so the next token in tree order is not
// necessarily this node's content: matching the text keeps the content and its
// position together instead of giving a node another node's span. A node the
// fast path cannot place uses the index, which returns the first occurrence no
// other node holds. When no token matches, the node falls back to its parent's
// range.
func (b *builder) textSpanFor(data string, parent *Node) (int, int, bool) {
	if data == "" || b.failure != nil {
		return 0, 0, false
	}
	for b.spanCur < len(b.scan.spans) {
		s := b.scan.spans[b.spanCur]
		if s.kind != spanText || b.used[b.spanCur] || int(s.start) < b.lower {
			b.spanCur++
			continue
		}
		if start, ok := b.matchSpan(s, data, parent); ok {
			b.used[b.spanCur] = true
			b.spanCur++
			b.lower = int(s.end)
			return start, int(s.end), true
		}
		break
	}
	if h, ok := b.lookup(data); ok {
		return int(h.start), int(b.scan.spans[h.span].end), true
	}
	if parent != nil && isPreformatted(parent.Tag) {
		if h, ok := b.lookupTrim(data); ok {
			return int(h.start), int(b.scan.spans[h.span].end), true
		}
	}
	return 0, 0, false
}

// matchSpan reports whether s is the node's token and which start the node takes
// from it. A pre, listing or textarea text node may have had its first newline
// dropped by the tree construction, so its data is the token's text without that
// newline.
func (b *builder) matchSpan(s span, data string, parent *Node) (int, bool) {
	text := b.scan.textOf(b.doc.text, s)
	if text == data {
		return int(s.start), true
	}
	if strings.IndexByte(text, 0) >= 0 {
		if nulReplaced(text) == data || nulRemoved(text) == data {
			return int(s.start), true
		}
	}
	if parent != nil && isPreformatted(parent.Tag) {
		if rest, ok := trimmedText(text); ok {
			matched := rest == data
			if !matched && strings.IndexByte(rest, 0) >= 0 {
				matched = nulReplaced(rest) == data || nulRemoved(rest) == data
			}
			if matched {
				if n, ok := rawNewlineLen(b.doc.text[s.start:s.end]); ok {
					return int(s.start) + n, true
				}
			}
		}
	}
	return 0, false
}

// isPreformatted reports whether the tree may drop a leading newline from the
// element's first text token.
func isPreformatted(tag string) bool {
	switch tag {
	case "pre", "listing", "textarea":
		return true
	}
	return false
}

// trimmedText returns text without its first newline, which is what the tree
// construction keeps for a pre, listing or textarea first text token. A CR
// written as a character reference reaches the tree without being normalized, so
// both line endings count.
func trimmedText(text string) (string, bool) {
	if text == "" || (text[0] != '\n' && text[0] != '\r') {
		return "", false
	}
	return text[1:], true
}

// rawNewlineLen returns the raw byte length that produces the token's first
// decoded newline. It is one byte for LF or CR, two for CRLF, and the reference's
// length when the newline was written as a character reference.
func rawNewlineLen(raw []byte) (int, bool) {
	best := 0
	for k := 1; k <= len(raw) && k <= 32; k++ {
		switch decodeText(raw[:k]) {
		case "\n", "\r":
			best = k
		}
	}
	return best, best > 0
}

// nulReplaced and nulRemoved are the two forms the tree construction produces
// from a NUL: its replacement character in body content, or nothing in table
// content.
func nulReplaced(text string) string { return strings.ReplaceAll(text, "\x00", "\uFFFD") }
func nulRemoved(text string) string  { return strings.ReplaceAll(text, "\x00", "") }

// decodeText decodes a text prefix the way the tree construction sees it: line
// endings normalized, then character references.
func decodeText(raw []byte) string {
	s := string(raw)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return xhtml.UnescapeString(s)
}

// buildIndex records every text span by decoded text. It runs once, on the first
// node the fast path cannot place, so a document whose tree order matches its
// source order never pays for it.
func (b *builder) buildIndex() {
	if b.exact != nil {
		return
	}
	b.exact = make(map[string][]hit)
	b.trim = make(map[string][]hit)
	for i, s := range b.scan.spans {
		if s.kind != spanText {
			continue
		}
		b.charge(1)
		if b.failure != nil {
			return
		}
		b.addOccurrence(i, s)
	}
}

// addOccurrence indexes one text span under every key a node's data can take
// from it.
func (b *builder) addOccurrence(i int, s span) {
	text := b.scan.textOf(b.doc.text, s)
	h := hit{span: int32(i), start: s.start}
	b.exact[text] = append(b.exact[text], h)
	if strings.IndexByte(text, 0) >= 0 {
		replaced, removed := nulReplaced(text), nulRemoved(text)
		b.exact[replaced] = append(b.exact[replaced], h)
		b.exact[removed] = append(b.exact[removed], h)
	}
	if rest, ok := trimmedText(text); ok {
		if n, ok := rawNewlineLen(b.doc.text[s.start:s.end]); ok {
			trimmed := hit{span: int32(i), start: s.start + int32(n)}
			b.trim[rest] = append(b.trim[rest], trimmed)
			if strings.IndexByte(rest, 0) >= 0 {
				replaced, removed := nulReplaced(rest), nulRemoved(rest)
				b.trim[replaced] = append(b.trim[replaced], trimmed)
				b.trim[removed] = append(b.trim[removed], trimmed)
			}
		}
	}
}

// lookup returns the first occurrence of data that no node holds yet.
func (b *builder) lookup(data string) (hit, bool) {
	b.charge(1)
	if b.failure != nil {
		return hit{}, false
	}
	b.buildIndex()
	if b.failure != nil {
		return hit{}, false
	}
	list := b.exact[data]
	if len(list) == 0 {
		return hit{}, false
	}
	i := b.next[data]
	for i < len(list) {
		h := list[i]
		i++
		if b.used[h.span] {
			continue
		}
		b.next[data] = i
		b.used[h.span] = true
		return h, true
	}
	b.next[data] = i
	return hit{}, false
}

// lookupTrim returns the first occurrence of data whose token had a newline
// dropped from its start.
func (b *builder) lookupTrim(data string) (hit, bool) {
	b.charge(1)
	if b.failure != nil {
		return hit{}, false
	}
	b.buildIndex()
	if b.failure != nil {
		return hit{}, false
	}
	list := b.trim[data]
	if len(list) == 0 {
		return hit{}, false
	}
	i := b.trimNext[data]
	for i < len(list) {
		h := list[i]
		i++
		if b.used[h.span] {
			continue
		}
		b.trimNext[data] = i
		b.used[h.span] = true
		return h, true
	}
	b.trimNext[data] = i
	return hit{}, false
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
