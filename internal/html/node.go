package html

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/diag"
)

// NodeType classifies a node in the parsed document tree.
type NodeType int

const (
	// DocumentNode is the synthetic root of the tree.
	DocumentNode NodeType = iota
	// ElementNode is an HTML element.
	ElementNode
	// TextNode is a run of decoded text.
	TextNode
	// CommentNode is an HTML comment. Layout ignores comments.
	CommentNode
	// DoctypeNode is the document type declaration.
	DoctypeNode
)

func (t NodeType) String() string {
	switch t {
	case ElementNode:
		return "element"
	case TextNode:
		return "text"
	case CommentNode:
		return "comment"
	case DoctypeNode:
		return "doctype"
	default:
		return "document"
	}
}

// Attr is a single element attribute.
type Attr struct {
	Key   string
	Value string
}

// Node is a node of the parsed document.
type Node struct {
	Type     NodeType
	Tag      string
	Attrs    []Attr
	Data     string
	Parent   *Node
	Children []*Node
	// Start and End are byte offsets into the decoded text the document was
	// parsed from, End is exclusive. Document.Position translates them to the
	// bytes of the original file; they are the same when nothing was decoded.
	Start int
	End   int
	// prevElement is the nearest preceding element sibling, maintained when
	// the tree is built. A `+` selector hops to it in constant time instead of
	// scanning every preceding sibling.
	prevElement *Node
	// guard bounds the work of building a diagnostic path for this node. Nil
	// means unlimited work and is what a manually built tree has.
	guard *budget.Guard
	// classes caches the split class attribute. Attributes are immutable after
	// the tree is built, so the cache never needs invalidating. classesCached
	// distinguishes "not split yet" from "split to no tokens".
	classes       []string
	classesCached bool
}

// attachBudget gives every node of a tree the parser's work guard, so Path can
// account for the diagnostic strings it builds.
func attachBudget(root *Node, g *budget.Guard) {
	if root == nil {
		return
	}
	stack := []*Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n.guard = g
		stack = append(stack, n.Children...)
	}
}

// charge records path-building work. A nil guard accepts everything.
func (n *Node) charge(units uint64) bool {
	if n.guard == nil {
		return true
	}
	return n.guard.Add(units) == nil
}

// Attr returns the value of an attribute and whether it is present.
func (n *Node) Attr(key string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Key == key {
			return a.Value, true
		}
	}
	return "", false
}

// Class returns the class attribute split into tokens. The split follows
// HTML's rule: only ASCII whitespace separates tokens, so a no-break space
// stays inside its token. The slice is computed once per node and shared;
// callers must not modify it.
func (n *Node) Class() []string {
	if n.classesCached {
		return n.classes
	}
	n.classesCached = true
	if v, ok := n.Attr("class"); ok {
		n.classes = splitClasses(v)
	}
	return n.classes
}

// splitClasses splits a class attribute on the ASCII whitespace characters
// HTML defines: space, tab, line feed, form feed and carriage return. Every
// other byte, including the no-break space U+00A0, belongs to its token.
func splitClasses(v string) []string {
	var out []string
	start := 0
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case ' ', '\t', '\n', '\f', '\r':
			if i > start {
				out = append(out, v[start:i])
			}
			start = i + 1
		}
	}
	if start < len(v) {
		out = append(out, v[start:])
	}
	return out
}

// HasClass reports whether the element carries the given class.
func (n *Node) HasClass(name string) bool {
	for _, c := range n.Class() {
		if c == name {
			return true
		}
	}
	return false
}

// ID returns the value of the id attribute.
func (n *Node) ID() string {
	v, _ := n.Attr("id")
	return v
}

// ElementChildren returns child nodes that are elements.
func (n *Node) ElementChildren() []*Node {
	out := make([]*Node, 0, len(n.Children))
	for _, c := range n.Children {
		if c.Type == ElementNode {
			out = append(out, c)
		}
	}
	return out
}

// FirstElementChild returns the first element child or nil.
func (n *Node) FirstElementChild() *Node {
	for _, c := range n.Children {
		if c.Type == ElementNode {
			return c
		}
	}
	return nil
}

// TextContent returns the concatenated text of the node and its descendants.
func (n *Node) TextContent() string {
	if n.Type == TextNode {
		return n.Data
	}
	var b strings.Builder
	var walk func(*Node)
	walk = func(x *Node) {
		for _, c := range x.Children {
			if c.Type == TextNode {
				b.WriteString(c.Data)
				continue
			}
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// AppendChild adds a child node, maintaining Parent and sibling pointers.
func (n *Node) AppendChild(c *Node) {
	c.Parent = n
	c.prevElement = nil
	if c.Type == ElementNode {
		c.prevElement = n.lastElementChild()
	}
	n.Children = append(n.Children, c)
}

// PrevElementSibling returns the nearest preceding element sibling, or nil.
// The pointer is maintained when the tree is built, so a caller never scans
// the preceding siblings.
func (n *Node) PrevElementSibling() *Node { return n.prevElement }

// lastElementChild returns the last element among n's children.
func (n *Node) lastElementChild() *Node {
	for i := len(n.Children) - 1; i >= 0; i-- {
		if n.Children[i].Type == ElementNode {
			return n.Children[i]
		}
	}
	return nil
}

// InsertBefore inserts c immediately before ref, which must be a child of n.
func (n *Node) InsertBefore(ref, c *Node) {
	if i := n.ChildIndex(ref); i >= 0 {
		n.InsertAt(i, c)
		return
	}
	n.AppendChild(c)
}

// ChildIndex returns the position of child among n's children, or -1.
func (n *Node) ChildIndex(child *Node) int {
	for i, c := range n.Children {
		if c == child {
			return i
		}
	}
	return -1
}

// InsertAt inserts c at position i, shifting the following children. An index
// outside the current children appends.
func (n *Node) InsertAt(i int, c *Node) {
	if i < 0 || i > len(n.Children) {
		n.AppendChild(c)
		return
	}
	c.Parent = n
	n.Children = append(n.Children, nil)
	copy(n.Children[i+1:], n.Children[i:])
	n.Children[i] = c
	// Foster parenting and an explicit InsertBefore both land here, so the
	// hop from every following element is recomputed instead of left stale.
	n.relinkFrom(i)
}

// relinkFrom recomputes the previous-element pointers of the children at and
// after i.
func (n *Node) relinkFrom(i int) {
	var prev *Node
	for k := 0; k < i && k < len(n.Children); k++ {
		if n.Children[k].Type == ElementNode {
			prev = n.Children[k]
		}
	}
	for k := i; k < len(n.Children); k++ {
		c := n.Children[k]
		if c.Type != ElementNode {
			c.prevElement = nil
			continue
		}
		c.prevElement = prev
		prev = c
	}
}

// Path renders a human readable element path such as "body > div.invoice > p"
// for use in diagnostics. The html element and the document root are omitted.
// Every byte written is charged to the work budget, so an element with a very
// long id or class list cannot build an unbounded diagnostic string for free.
func (n *Node) Path() string {
	var parts []string
	for x := n; x != nil && x.Type != DocumentNode; x = x.Parent {
		if x.Type != ElementNode {
			continue
		}
		seg, ok := x.pathSegment()
		if !ok {
			// The work budget or a cancellation stopped the path; the partial
			// path is still a usable position and the guard carries the reason.
			break
		}
		parts = append(parts, seg)
	}
	// Drop the html element: paths start at head or body.
	for len(parts) > 1 && parts[len(parts)-1] == "html" {
		parts = parts[:len(parts)-1]
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	var b strings.Builder
	for i, seg := range parts {
		if i > 0 {
			if !n.charge(uint64(len(pathSeparator))) {
				return b.String()
			}
			b.WriteString(pathSeparator)
		}
		b.WriteString(seg)
	}
	return b.String()
}

// pathSeparator joins two path segments.
const pathSeparator = " > "

// pathSegment builds "tag#id.class1.class2" with a builder and charges one
// unit per byte it writes. The builder grows amortised linearly, so the cost
// follows the length of the segment rather than the number of class tokens.
func (x *Node) pathSegment() (string, bool) {
	var b strings.Builder
	if !x.writePath(&b, x.Tag) {
		return "", false
	}
	if id := x.ID(); id != "" {
		if !x.writePath(&b, "#") || !x.writePath(&b, id) {
			return "", false
		}
	}
	for _, c := range x.Class() {
		if !x.writePath(&b, ".") || !x.writePath(&b, c) {
			return "", false
		}
	}
	return b.String(), true
}

// writePath appends s to a path builder and charges one unit per byte written.
func (x *Node) writePath(b *strings.Builder, s string) bool {
	if !x.charge(uint64(len(s))) {
		return false
	}
	b.WriteString(s)
	return true
}

// Document is a parsed HTML document together with its source mapping.
type Document struct {
	Root  *Node
	File  string
	lines []int
	// src is the file as it was read, and text is the decoded text the node
	// offsets refer to. They are the same slice when no decoding was needed.
	src  []byte
	text []byte
	// charset is the encoding the document was decoded from, and mapper
	// translates a decoded offset back to the source bytes. The mapper is nil
	// when the two coordinate spaces are the same.
	charset *charset.Encoding
	mapper  *charset.Mapper
	// guard is the parser's work guard. Position charges the bytes it scans to
	// it, so a stage that reports many positions on one line cannot avoid the
	// budget. Nil means unlimited work.
	guard *budget.Guard
	// Position keeps a cursor on the line of the last query, like
	// charset.Mapper does, so the bytes between two positions on one line are
	// scanned once instead of from the line start every time. The text and the
	// line starts never change after the parse, so the cursor needs no
	// invalidation. A Document belongs to one conversion: Position, like the
	// rest of the parse, is not safe for concurrent use.
	posLine  int // index into lines, meaningless unless posValid
	posCol   int // byte offset into that line, always a rune boundary
	posRunes int // runes from the line start to posCol
	posValid bool
}

// Charset returns the encoding the document was decoded from.
func (d *Document) Charset() *charset.Encoding { return d.charset }

// Position converts a byte offset into a file position. The offset is an
// offset in the decoded text; the returned column counts characters (Unicode
// code points) from the start of the line, not bytes, so a diagnostic after a
// Cyrillic word names the column the author sees in an editor. A legacy
// encoding decodes to one character per source byte, so the same rule holds for
// both coordinate spaces.
//
// The bytes between the previous query and this one are scanned once and
// charged to the work budget, so a minified document costs one pass over its
// line rather than one pass per diagnostic. A query behind the cursor rescans
// from the line start; diagnostics are normally in document order.
func (d *Document) Position(offset int) diag.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(d.text) {
		offset = len(d.text)
	}
	// Largest line start <= offset.
	i, _ := slices.BinarySearch(d.lines, offset+1)
	i--
	if i < 0 {
		i = 0
	}
	line := i + 1
	start := d.lines[i]
	colByte := offset - start
	if !d.posValid || d.posLine != i || colByte < d.posCol {
		d.posLine, d.posCol, d.posRunes, d.posValid = i, 0, 0, true
	}
	if d.mapper != nil {
		// The mapper keeps its own cursor on the decoded widths. Charge the
		// decoded bytes since the previous query, which are proportional to the
		// source bytes the mapper walks.
		if colByte > d.posCol {
			_ = d.guard.Add(uint64(colByte - d.posCol))
			d.posCol = colByte
		}
		mapperLine, col := d.mapper.Position(offset)
		return diag.Position{File: d.File, Line: mapperLine, Col: col}
	}
	colRunes := d.posRunes
	if colByte > d.posCol {
		from := start + d.posCol
		// One unit per byte examined, so the charge follows the scanned span.
		_ = d.guard.Add(uint64(colByte - d.posCol))
		whole, end := countRunesTo(d.text, from, offset)
		// An offset inside a character counts every remaining byte as one
		// rune, the same way a RuneCount from the line start does.
		colRunes += whole + (offset - end)
		// Keep the cursor on a whole rune boundary: an offset inside a
		// character must not make the next query start in the middle of one.
		d.posRunes += whole
		d.posCol = end - start
	}
	return diag.Position{File: d.File, Line: line, Col: colRunes + 1}
}

// countRunesTo counts the whole runes in text[from:to] and returns the last
// rune boundary at or before to. It advances one character at a time, so it
// allocates nothing, unlike utf8.RuneCount, whose non-ASCII path converts the
// remaining bytes to a string. The text is valid UTF-8, so only the final
// character can be truncated; the caller counts its bytes as RuneCount would.
func countRunesTo(text []byte, from, to int) (whole, end int) {
	end = from
	for end < to {
		if text[end] < utf8.RuneSelf {
			end++
			whole++
			continue
		}
		if !utf8.FullRune(text[end:to]) {
			return whole, end
		}
		_, size := utf8.DecodeRune(text[end:to])
		whole++
		end += size
	}
	return whole, end
}

// Pos returns the position of the node's start offset.
func (d *Document) Pos(n *Node) diag.Position { return d.Position(n.Start) }

// Source returns the original document bytes.
func (d *Document) Source() []byte { return d.src }

// FindByID returns the first element with the given id, or nil.
func (d *Document) FindByID(id string) *Node {
	var found *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if found != nil {
			return
		}
		if n.Type == ElementNode && n.ID() == id {
			found = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(d.Root)
	return found
}

// HTML returns the html element, or nil for documents without one.
func (d *Document) HTML() *Node {
	for _, c := range d.Root.Children {
		if c.Type == ElementNode && c.Tag == "html" {
			return c
		}
	}
	return nil
}

// Body returns the body element, or nil for documents without one.
func (d *Document) Body() *Node {
	for _, c := range d.Root.Children {
		if c.Type == ElementNode && c.Tag == "html" {
			for _, g := range c.Children {
				if g.Type == ElementNode && g.Tag == "body" {
					return g
				}
			}
		}
	}
	return nil
}

// Head returns the head element, or nil.
func (d *Document) Head() *Node {
	for _, c := range d.Root.Children {
		if c.Type == ElementNode && c.Tag == "html" {
			for _, g := range c.Children {
				if g.Type == ElementNode && g.Tag == "head" {
					return g
				}
			}
		}
	}
	return nil
}

// Dump renders the tree in a stable, readable form used by tests and by the
// --dump-tree debug flag.
func Dump(n *Node) string {
	var b strings.Builder
	var walk func(*Node, int)
	walk = func(x *Node, depth int) {
		indent := strings.Repeat("  ", depth)
		switch x.Type {
		case DocumentNode:
			b.WriteString("#document\n")
		case ElementNode:
			fmt.Fprintf(&b, "%s<%s", indent, x.Tag)
			for _, a := range x.Attrs {
				fmt.Fprintf(&b, " %s=%q", a.Key, a.Value)
			}
			b.WriteString(">\n")
		case TextNode:
			fmt.Fprintf(&b, "%s%q\n", indent, x.Data)
		case CommentNode:
			fmt.Fprintf(&b, "%s<!--%s-->\n", indent, x.Data)
		case DoctypeNode:
			fmt.Fprintf(&b, "%s<!doctype %s>\n", indent, x.Data)
		}
		for _, c := range x.Children {
			walk(c, depth+1)
		}
	}
	walk(n, 0)
	return b.String()
}
