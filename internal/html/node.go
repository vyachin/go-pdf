package html

import (
	"fmt"
	"slices"
	"strings"

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

// Class returns the class attribute split into tokens.
func (n *Node) Class() []string {
	v, ok := n.Attr("class")
	if !ok || v == "" {
		return nil
	}
	return strings.Fields(v)
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

// AppendChild adds a child node, maintaining Parent pointers.
func (n *Node) AppendChild(c *Node) {
	c.Parent = n
	n.Children = append(n.Children, c)
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
}

// Path renders a human readable element path such as "body > div.invoice > p"
// for use in diagnostics. The html element and the document root are omitted.
func (n *Node) Path() string {
	var parts []string
	for x := n; x != nil && x.Type != DocumentNode; x = x.Parent {
		if x.Type != ElementNode {
			continue
		}
		seg := x.Tag
		if id := x.ID(); id != "" {
			seg += "#" + id
		}
		for _, c := range x.Class() {
			seg += "." + c
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
	return strings.Join(parts, " > ")
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
}

// Charset returns the encoding the document was decoded from.
func (d *Document) Charset() *charset.Encoding { return d.charset }

// Position converts a byte offset into a file position. The offset is an
// offset in the decoded text; the returned column is the column of the byte
// that produced it in the original source.
func (d *Document) Position(offset int) diag.Position {
	if d.mapper != nil {
		line, col := d.mapper.Position(offset)
		return diag.Position{File: d.File, Line: line, Col: col}
	}
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
	col := offset - d.lines[i] + 1
	return diag.Position{File: d.File, Line: line, Col: col}
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
