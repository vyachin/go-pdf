package html

import (
	"fmt"
	"strings"

	"github.com/vyachin/go-pdf/internal/budget"
)

const (
	modeInitial = iota
	modeHead
	modeBody
)

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

var headContent = map[string]bool{
	"title": true, "meta": true, "link": true, "style": true, "base": true, "script": true,
}

var closesParagraph = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "details": true,
	"div": true, "dl": true, "fieldset": true, "figcaption": true, "figure": true,
	"footer": true, "form": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "header": true, "hr": true, "main": true, "nav": true,
	"ol": true, "p": true, "pre": true, "section": true, "table": true, "ul": true,
}

var tableAllowed = map[string]bool{
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true,
	"td": true, "th": true, "caption": true, "colgroup": true, "col": true,
	"style": true, "script": true,
}

// tableStructure is the set of elements that hold table structure: content
// directly inside them must be table structure too, or it is foster-parented.
var tableStructure = map[string]bool{
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true,
	"colgroup": true,
}

type parser struct {
	doc   *Document
	toks  []Token
	stack []*Node
	html  *Node
	head  *Node
	body  *Node
	mode  int
	// maxDepth bounds element nesting; zero means unlimited.
	maxDepth int
	budget   *budget.Guard
	// fosterIndex caches where the next foster-parented node goes for a table,
	// so inserting N nodes before it does not scan the child list N times.
	fosterIndex map[*Node]int
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
}

// Parse tokenizes src and builds a document tree. The file argument is used
// only for positions in diagnostics.
func Parse(src []byte, file string) (*Document, error) {
	return ParseWithOptions(src, ParseOptions{File: file})
}

// ParseWithOptions parses src under the given limits and cancellation.
func ParseWithOptions(src []byte, opts ParseOptions) (*Document, error) {
	if err := opts.Budget.Err(); err != nil {
		return nil, err
	}
	d := &Document{File: opts.File, src: src, Root: &Node{Type: DocumentNode}}
	d.lines = lineStarts(src)
	toks, err := tokenize(src, opts.Budget)
	if err != nil {
		return nil, err
	}
	p := &parser{doc: d, toks: toks, maxDepth: opts.MaxDepth, budget: opts.Budget}
	if err := p.run(); err != nil {
		return nil, err
	}
	return d, nil
}

func tokenize(src []byte, g *budget.Guard) ([]Token, error) {
	z := NewTokenizer(src)
	var out []Token
	for {
		if err := g.Add(1); err != nil {
			return out, err
		}
		t := z.Next()
		if t.Kind == EOFToken {
			return out, nil
		}
		if t.Kind == ErrorToken {
			return out, nil
		}
		out = append(out, t)
		if t.Kind == StartTagToken && (t.Name == "script" || t.Name == "style") {
			z.SetRawTag(t.Name)
		}
	}
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

func (p *parser) run() error {
	for _, t := range p.toks {
		if err := p.budget.Add(0); err != nil {
			return err
		}
		switch t.Kind {
		case TextToken:
			p.text(t)
		case StartTagToken:
			p.start(t)
		case EndTagToken:
			p.end(t)
		case CommentToken:
			p.comment(t)
		case DoctypeToken:
			p.doc.Root.AppendChild(&Node{Type: DoctypeNode, Data: t.Data, Start: t.Offset, End: t.End})
		}
		// The stack grows by at most one node per token, so checking after the
		// token bounds nesting before the next node is allocated.
		if p.maxDepth > 0 && len(p.stack) > p.maxDepth {
			return fmt.Errorf("%w: element nesting exceeds %d levels", budget.ErrExceeded, p.maxDepth)
		}
	}
	return nil
}

func (p *parser) current() *Node {
	if len(p.stack) == 0 {
		return p.doc.Root
	}
	return p.stack[len(p.stack)-1]
}

func (p *parser) ensureHTML(t Token) {
	if p.html != nil {
		return
	}
	// Only the real <html> tag carries attributes; an implicit html created
	// from another start tag must not inherit that tag's attributes.
	var attrs []Attr
	if t.Name == "html" {
		attrs = t.Attrs
	}
	p.html = &Node{Type: ElementNode, Tag: "html", Attrs: attrs, Start: t.Offset, End: t.End}
	p.doc.Root.AppendChild(p.html)
	p.stack = []*Node{p.html}
}

func (p *parser) ensureHead(t Token) {
	p.ensureHTML(t)
	if p.head != nil {
		return
	}
	p.head = &Node{Type: ElementNode, Tag: "head", Start: t.Offset, End: t.End}
	p.html.AppendChild(p.head)
}

func (p *parser) ensureBody(t Token) {
	p.ensureHTML(t)
	p.ensureHead(t)
	if p.body != nil {
		return
	}
	p.body = &Node{Type: ElementNode, Tag: "body", Start: t.Offset, End: t.End}
	p.html.AppendChild(p.body)
}

func (p *parser) enterBody(t Token) {
	p.ensureBody(t)
	p.stack = []*Node{p.html, p.body}
	p.mode = modeBody
}

func (p *parser) text(t Token) {
	if p.mode == modeHead {
		if top := p.current(); top.Tag == "title" || top.Tag == "style" || top.Tag == "script" {
			p.appendText(t)
			return
		}
	}
	if p.mode != modeBody {
		if strings.TrimSpace(t.Data) == "" {
			return
		}
		p.enterBody(t)
	}
	p.appendText(t)
}

func (p *parser) appendText(t Token) {
	if p.inTableStructure() {
		if strings.TrimSpace(t.Data) == "" {
			return
		}
		p.foster(&Node{Type: TextNode, Data: t.Data, Start: t.Offset, End: t.End})
		return
	}
	parent := p.current()
	if len(parent.Children) > 0 {
		if last := parent.Children[len(parent.Children)-1]; last.Type == TextNode && last.End == t.Offset {
			last.Data += t.Data
			last.End = t.End
			return
		}
	}
	parent.AppendChild(&Node{Type: TextNode, Data: t.Data, Start: t.Offset, End: t.End})
}

func (p *parser) comment(t Token) {
	node := &Node{Type: CommentNode, Data: t.Data, Start: t.Offset, End: t.End}
	if len(p.stack) == 0 {
		p.doc.Root.AppendChild(node)
		return
	}
	p.current().AppendChild(node)
}

func (p *parser) start(t Token) {
	switch t.Name {
	case "html":
		p.ensureHTML(t)
		return
	case "head":
		p.ensureHTML(t)
		if p.head == nil {
			p.head = &Node{Type: ElementNode, Tag: "head", Start: t.Offset, End: t.End}
			p.html.AppendChild(p.head)
		}
		p.stack = []*Node{p.html, p.head}
		p.mode = modeHead
		return
	case "body":
		p.ensureHTML(t)
		p.ensureHead(t)
		if p.body == nil {
			p.body = &Node{Type: ElementNode, Tag: "body", Start: t.Offset, End: t.End, Attrs: t.Attrs}
			p.html.AppendChild(p.body)
		}
		p.stack = []*Node{p.html, p.body}
		p.mode = modeBody
		return
	}

	if p.mode != modeBody {
		if headContent[t.Name] {
			p.ensureHead(t)
			p.stack = []*Node{p.html, p.head}
			p.mode = modeHead
			p.insertElement(t)
			return
		}
		p.enterBody(t)
	}

	p.implicitClose(t.Name)
	p.insertElement(t)
}

func isHeading(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}

func (p *parser) implicitClose(name string) {
	if closesParagraph[name] {
		p.closeOpen("p")
	}
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		// A heading start tag closes the heading that is the current node, so
		// "<h1>a<h2>b" produces siblings, not a heading inside a heading.
		if isHeading(p.current().Tag) {
			p.stack = p.stack[:len(p.stack)-1]
		}
	case "li":
		p.closeOpenIn("li", "ol", "ul")
	case "dt", "dd":
		p.closeOpenIn("dt", "dl")
		p.closeOpenIn("dd", "dl")
	case "td", "th":
		p.closeOpenIn("td", "table")
		p.closeOpenIn("th", "table")
	case "tr":
		p.closeOpenIn("td", "table")
		p.closeOpenIn("th", "table")
		p.closeOpenIn("tr", "table")
	case "thead", "tbody", "tfoot":
		p.closeOpenIn("td", "table")
		p.closeOpenIn("th", "table")
		p.closeOpenIn("tr", "table")
		p.closeOpenIn("thead", "table")
		p.closeOpenIn("tbody", "table")
		p.closeOpenIn("tfoot", "table")
	case "option":
		p.closeOpenIn("option", "select")
	}
}

func (p *parser) insertElement(t Token) {
	el := &Node{Type: ElementNode, Tag: t.Name, Attrs: t.Attrs, Start: t.Offset, End: t.End}
	// A table start tag inside table structure closes the outer table and opens
	// a sibling: nesting it would make layout silently drop the inner rows.
	if t.Name == "table" && p.inTableContext() {
		p.closeOpen("table")
	}
	// A start tag for table structure clears a foster-parented element that is
	// still open, as the HTML parser returns to a table context.
	if tableStructure[t.Name] && p.inTableContext() && !p.inTableStructure() {
		p.clearToTableStructure()
	}
	if p.inTableStructure() && !tableAllowed[t.Name] {
		p.foster(el)
	} else {
		p.current().AppendChild(el)
	}
	if voidElements[t.Name] || t.SelfClosing {
		return
	}
	p.stack = append(p.stack, el)
}

func (p *parser) end(t Token) {
	switch t.Name {
	case "html", "body":
		return
	case "head":
		p.closeOpen("head")
		if p.mode == modeHead {
			p.mode = modeInitial
			p.stack = []*Node{p.html}
		}
		return
	case "br":
		p.insertElement(Token{Kind: StartTagToken, Name: "br", Offset: t.Offset, End: t.End})
		return
	}
	p.closeOpen(t.Name)
}

// closeOpenIn is closeOpen restricted to the current scope: it stops at the
// first boundary element instead of closing an element in an enclosing table,
// list or select.
func (p *parser) closeOpenIn(tag string, boundaries ...string) bool {
	for i := len(p.stack) - 1; i >= 0; i-- {
		current := p.stack[i].Tag
		for _, boundary := range boundaries {
			if current == boundary {
				return false
			}
		}
		if current == tag {
			p.stack = p.stack[:i]
			return true
		}
	}
	return false
}

// closeOpen pops the stack down to and including the nearest element with the
// given tag. It reports whether such an element was open.
func (p *parser) closeOpen(tag string) bool {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Tag == tag {
			p.stack = p.stack[:i]
			return true
		}
	}
	return false
}

// inTableContext reports whether a table is open and the stack has not entered
// a cell or caption.
func (p *parser) inTableContext() bool {
	for i := len(p.stack) - 1; i >= 0; i-- {
		switch p.stack[i].Tag {
		case "td", "th", "caption":
			return false
		case "table":
			return true
		}
	}
	return false
}

// inTableStructure reports whether the insertion point is table structure
// itself. Inside a foster-parented element the insertion point is that element,
// so its text and child elements belong to it and must not be fostered again:
// otherwise a hidden block inside a table leaks its content next to the table.
func (p *parser) inTableStructure() bool {
	if len(p.stack) == 0 {
		return false
	}
	return tableStructure[p.stack[len(p.stack)-1].Tag]
}

// clearToTableStructure pops foster-parented elements until table structure is
// current again, the "clear the stack back to a table context" step.
func (p *parser) clearToTableStructure() {
	for len(p.stack) > 0 && !p.inTableStructure() {
		p.stack = p.stack[:len(p.stack)-1]
	}
}

// foster inserts a node before the nearest table ancestor, as the HTML parser
// does for content that is not allowed inside a table.
func (p *parser) foster(n *Node) {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Tag == "table" {
			table := p.stack[i]
			parent := table.Parent
			if parent == nil {
				break
			}
			idx, cached := p.fosterIndex[table]
			if !cached || idx < 0 || idx >= len(parent.Children) || parent.Children[idx] != table {
				idx = parent.ChildIndex(table)
			}
			if idx < 0 {
				parent.AppendChild(n)
				return
			}
			parent.InsertAt(idx, n)
			if p.fosterIndex == nil {
				p.fosterIndex = map[*Node]int{}
			}
			p.fosterIndex[table] = idx + 1
			return
		}
	}
	p.current().AppendChild(n)
}
