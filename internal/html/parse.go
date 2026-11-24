package html

import (
	"strings"
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

type parser struct {
	doc   *Document
	toks  []Token
	stack []*Node
	html  *Node
	head  *Node
	body  *Node
	mode  int
}

// Parse tokenizes src and builds a document tree. The file argument is used
// only for positions in diagnostics.
func Parse(src []byte, file string) (*Document, error) {
	d := &Document{File: file, src: src, Root: &Node{Type: DocumentNode}}
	d.lines = lineStarts(src)
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{doc: d, toks: toks}
	p.run()
	return d, nil
}

func tokenize(src []byte) ([]Token, error) {
	z := NewTokenizer(src)
	var out []Token
	for {
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

func (p *parser) run() {
	for _, t := range p.toks {
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
	}
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
	p.html = &Node{Type: ElementNode, Tag: "html", Start: t.Offset, End: t.End}
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
		if top := p.current(); top.Tag == "title" || top.Tag == "style" {
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
	if p.inTable() {
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

func (p *parser) implicitClose(name string) {
	if closesParagraph[name] {
		p.closeOpen("p")
	}
	switch name {
	case "li":
		p.closeOpen("li")
	case "dt", "dd":
		p.closeOpen("dt")
		p.closeOpen("dd")
	case "td", "th":
		p.closeOpen("td")
		p.closeOpen("th")
	case "tr":
		p.closeOpen("td")
		p.closeOpen("th")
		p.closeOpen("tr")
	case "thead", "tbody", "tfoot":
		p.closeOpen("td")
		p.closeOpen("th")
		p.closeOpen("tr")
		p.closeOpen("thead")
		p.closeOpen("tbody")
		p.closeOpen("tfoot")
	case "option":
		p.closeOpen("option")
	}
}

func (p *parser) insertElement(t Token) {
	el := &Node{Type: ElementNode, Tag: t.Name, Attrs: t.Attrs, Start: t.Offset, End: t.End}
	if p.inTable() && !tableAllowed[t.Name] {
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

func (p *parser) inTable() bool {
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

// foster inserts a node before the nearest table ancestor, as the HTML parser
// does for content that is not allowed inside a table.
func (p *parser) foster(n *Node) {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Tag == "table" {
			table := p.stack[i]
			if table.Parent != nil {
				table.Parent.InsertBefore(table, n)
				return
			}
			break
		}
	}
	p.current().AppendChild(n)
}
