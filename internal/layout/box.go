// Package layout turns a styled document into positioned pages of drawing
// operations: boxes, lines, tables and page breaks.
package layout

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/html"
)

// Kind classifies a box in the layout tree.
type Kind int

const (
	// Block is a block level box, including anonymous blocks around inline runs.
	Block Kind = iota
	// Inline is an inline element such as span or a.
	Inline
	// Text is a text node.
	Text
	// Image is a replaced image element.
	Image
	// Table is a table box.
	Table
	// TableRow is a row of a table.
	TableRow
	// TableCell is a cell of a table.
	TableCell
	// ListItem is a block that draws a list marker.
	ListItem
)

// Box is a node of the layout tree. Geometry is filled during pagination.
type Box struct {
	Kind     Kind
	Node     *html.Node
	Style    *css.Style
	Text     string
	Children []*Box
	Image    *ImageData
	// Rows holds the rows of a table box.
	Rows []*Row
	// ColSpan and RowSpan are set on table cells.
	ColSpan, RowSpan int
	// ListIndex is the 1-based number of a list item.
	ListIndex int
	// Level is the nesting depth, used for list indentation.
	Level int

	// Pagination state.
	lines      []Line
	built      bool
	cursor     cursor
	started    bool
	entered    bool
	x, y       int32 // absolute position of the box origin
	width      int32
	startPage  int
	fragStartY int32
	fragStartPage int
	fragments  int
	listMarkerY int32
	contentTopY int32

	// Table state.
	columns     []int32
	rowHeights  []int32
	headerPage  int
	headerCount int
}

// Row is a table row.
type Row struct {
	Header bool
	Cells  []*Box
}

type cursor struct {
	child int
	line  int
	row   int
	done  bool
}

// ImageData is a decoded image ready to be painted.
type ImageData struct {
	Width, Height int32 // intrinsic size in millipoints
	PixelW        int
	PixelH        int
	Format        string
	Data          []byte
}

// FontFace is a font resolved at a size.
type FontFace struct {
	Font    *font.Font
	Size    int32
	Ascent  int32
	Descent int32
	Key     string
}

// FontSet resolves a font family list into a concrete face.
type FontSet interface {
	Face(families []string, weight int, italic bool, size int32) (*FontFace, error)
}

// ImageLoader resolves an image source into image data.
type ImageLoader interface {
	Load(src string) (*ImageData, error)
}

// Rect is a rectangle in millipoints.
type Rect struct {
	X, Y, W, H int32
}

// OpKind classifies a drawing operation.
type OpKind int

const (
	// FillOpKind fills a rectangle with a solid colour.
	FillOpKind OpKind = iota
	// TextOpKind draws a shaped run.
	TextOpKind
	// ImageOpKind draws an image.
	ImageOpKind
	// LinkOpKind records a clickable area.
	LinkOpKind
)

// Op is one drawing operation of a page.
type Op struct {
	Kind  OpKind
	Rect  Rect
	Color css.Color
	Text  *TextOp
	Image *ImageOp
	Link  *LinkOp
}

// TextOp is a shaped text run placed at a baseline.
type TextOp struct {
	X, Y  int32 // baseline origin
	Face  *FontFace
	Run   TextRun
	Color css.Color
}

// TextRun is a shaped run of glyphs.
type TextRun struct {
	Glyphs []Glyph
	Width  int32
}

// Glyph mirrors text.Glyph to keep the drawing API independent.
type Glyph struct {
	GID     uint16
	Advance int32
	Rune    rune
}

// ImageOp draws an image scaled into Rect.
type ImageOp struct {
	Image *ImageData
}

// LinkOp records a link target and its area.
type LinkOp struct {
	URL string
}

// Page is one rendered page.
type Page struct {
	Number int
	Size   css.PageSize
	Margin [4]int32
	Ops    []Op
}

// Document is the result of pagination.
type Document struct {
	Pages   []*Page
	Anchors map[string]Anchor
	// Outline lists document headings for PDF bookmarks.
	Outline []OutlineEntry
	Diags   diag.List
}

// OutlineEntry is one bookmark target.
type OutlineEntry struct {
	Title string
	Page  int
	Y     int32
	Level int
}

// Anchor is the position of a named element, used by target-counter.
type Anchor struct {
	Page int
	Y    int32
}

// Context carries the collaborators of layout.
type Context struct {
	Fonts    FontSet
	Images   ImageLoader
	PageSize css.PageSize
	Margin   [4]int32
	Cascade  *css.Cascade
	// PageStyle is the fully resolved page setup, including margin boxes.
	PageStyle PageStyle
	// TargetPages maps an element id to the page it landed on. It is filled
	// by the engine between layout passes to resolve target-counter().
	TargetPages map[string]int
}

// Build constructs the layout tree of a document body.
func Build(doc *html.Document, ctx Context) (*Box, diag.List) {
	body := doc.Body()
	if body == nil {
		return nil, diag.List{{Severity: diag.Error, Code: "html/no-body", Message: "document has no body"}}
	}
	b := &builder{cascade: ctx.Cascade, images: ctx.Images, diags: &diag.List{}}
	bodyStyle := ctx.Cascade.Compute(body, nil)
	root := &Box{Kind: Block, Node: body, Style: bodyStyle}
	root.Children = b.buildBlocks(body, bodyStyle)
	return root, *b.diags
}

type builder struct {
	cascade *css.Cascade
	images  ImageLoader
	diags   *diag.List
}

// buildBlocks builds the children of a block container, wrapping runs of
// inline content into anonymous block boxes.
func (b *builder) buildBlocks(n *html.Node, parentStyle *css.Style) []*Box {
	var out []*Box
	var inlineRun []*Box
	flushInline := func() {
		if len(inlineRun) == 0 {
			return
		}
		out = append(out, &Box{Kind: Block, Style: anonymousStyle(parentStyle), Children: inlineRun})
		inlineRun = nil
	}
	for _, child := range n.Children {
		switch child.Type {
		case html.TextNode:
			if strings.TrimSpace(child.Data) == "" && !preservesSpace(parentStyle) {
				// Keep one space between inline siblings and drop indentation.
				if len(inlineRun) > 0 {
					inlineRun = append(inlineRun, &Box{Kind: Text, Node: child, Style: parentStyle, Text: " "})
				}
				continue
			}
			inlineRun = append(inlineRun, &Box{Kind: Text, Node: child, Style: parentStyle, Text: child.Data})
		case html.ElementNode:
			style := b.cascade.Compute(child, parentStyle)
			if style.Display == css.DisplayNone {
				continue
			}
			box := b.buildElement(child, style, parentStyle)
			if box == nil {
				continue
			}
			if isInlineDisplay(style.Display) {
				inlineRun = append(inlineRun, box)
				continue
			}
			flushInline()
			out = append(out, box)
		}
	}
	flushInline()
	return out
}

func preservesSpace(style *css.Style) bool { return style.WhiteSpace == css.WhiteSpacePre }

// anonymousStyle is the style of an anonymous block: inherited properties come
// from the parent, box properties are reset.
func anonymousStyle(parent *css.Style) *css.Style {
	s := *parent
	s.Margin = [4]css.Length{}
	s.Padding = [4]css.Length{}
	s.Border = [4]css.BorderSide{}
	s.Background = css.Color{}
	s.Width = css.Length{Kind: css.Auto}
	s.Height = css.Length{Kind: css.Auto}
	s.BreakBefore = css.BreakAuto
	s.BreakAfter = css.BreakAuto
	s.BreakInside = css.BreakAuto
	s.TextIndent = css.Length{}
	return &s
}

func isInlineDisplay(d css.Display) bool {
	return d == css.DisplayInline || d == css.DisplayInlineBlock
}

func (b *builder) buildElement(n *html.Node, style *css.Style, parentStyle *css.Style) *Box {
	switch n.Tag {
	case "br":
		return &Box{Kind: Text, Node: n, Style: style, Text: "\n"}
	case "img":
		return b.buildImage(n, style)
	case "table":
		return b.buildTable(n, style)
	case "td", "th", "tr", "thead", "tbody", "tfoot":
		// Outside a table these are treated as blocks, which matches the
		// forgiving error handling of HTML.
		return &Box{Kind: Block, Node: n, Style: style, Children: b.buildBlocks(n, style)}
	case "li":
		box := &Box{Kind: ListItem, Node: n, Style: style, Children: b.buildBlocks(n, style)}
		box.ListIndex = liIndex(n)
		box.Level = listLevel(n)
		return box
	}
	kind := Block
	switch style.Display {
	case css.DisplayInline, css.DisplayInlineBlock:
		kind = Inline
	case css.DisplayListItem:
		kind = ListItem
	}
	box := &Box{Kind: kind, Node: n, Style: style}
	if kind == Inline {
		box.Children = b.buildInline(n, style)
		return box
	}
	box.Children = b.buildBlocks(n, style)
	return box
}

// buildInline builds inline descendants without anonymous block wrapping.
func (b *builder) buildInline(n *html.Node, parentStyle *css.Style) []*Box {
	var out []*Box
	for _, child := range n.Children {
		switch child.Type {
		case html.TextNode:
			out = append(out, &Box{Kind: Text, Node: child, Style: parentStyle, Text: child.Data})
		case html.ElementNode:
			style := b.cascade.Compute(child, parentStyle)
			if style.Display == css.DisplayNone {
				continue
			}
			if style.Display == css.DisplayBlock || style.Display == css.DisplayTable || style.Display == css.DisplayListItem {
				// A block inside an inline element starts a new line; keep it
				// simple and treat its text as an inline break.
				out = append(out, &Box{Kind: Text, Node: child, Style: style, Text: "\n"})
				out = append(out, b.buildInline(child, style)...)
				continue
			}
			out = append(out, b.buildElement(child, style, parentStyle))
		}
	}
	return out
}

func (b *builder) buildImage(n *html.Node, style *css.Style) *Box {
	src, _ := n.Attr("src")
	box := &Box{Kind: Image, Node: n, Style: style}
	if b.images == nil {
		b.diags.Errorf("img/no-loader", diag.Position{}, n.Path(), "images are not available in this context")
		return box
	}
	img, err := b.images.Load(src)
	if err != nil {
		b.diags.Errorf("img/load", diag.Position{}, n.Path(), "%v", err)
		return box
	}
	box.Image = img
	return box
}

func (b *builder) buildTable(n *html.Node, style *css.Style) *Box {
	tb := &Box{Kind: Table, Node: n, Style: style}
	var addRows func(*html.Node, bool)
	addRows = func(node *html.Node, header bool) {
		for _, c := range node.ElementChildren() {
			switch c.Tag {
			case "thead":
				addRows(c, true)
			case "tbody", "tfoot":
				addRows(c, header)
			case "tr":
				row := &Row{Header: header}
				for _, cellNode := range c.ElementChildren() {
					if cellNode.Tag != "td" && cellNode.Tag != "th" {
						continue
					}
					cellStyle := b.cascade.Compute(cellNode, style)
					// border-collapse is a property of the table, but cells
					// need it to decide which edges they own.
					cellStyle.BorderCollapse = style.BorderCollapse
					if cellNode.Tag == "th" {
						if _, ok := cellNode.Attr("style"); !ok {
							cellStyle.FontWeight = 700
							cellStyle.TextAlign = css.AlignCenter
						}
					}
					cell := &Box{Kind: TableCell, Node: cellNode, Style: cellStyle}
					cell.ColSpan = attrInt(cellNode, "colspan", 1)
					cell.RowSpan = attrInt(cellNode, "rowspan", 1)
					cell.Children = b.buildBlocks(cellNode, cellStyle)
					row.Cells = append(row.Cells, cell)
				}
				tb.Rows = append(tb.Rows, row)
			}
		}
	}
	addRows(n, false)
	return tb
}

func liIndex(n *html.Node) int {
	base := 1
	if n.Parent != nil && n.Parent.Tag == "ol" {
		if v, ok := n.Parent.Attr("start"); ok {
			if parsed, err := strconv.Atoi(v); err == nil {
				base = parsed
			}
		}
	}
	idx := 0
	if n.Parent != nil {
		for _, c := range n.Parent.Children {
			if c == n {
				return base + idx
			}
			if c.Type == html.ElementNode && c.Tag == "li" {
				idx++
			}
		}
	}
	return base + idx
}

func listLevel(n *html.Node) int {
	level := 0
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && (p.Tag == "ul" || p.Tag == "ol") {
			level++
		}
	}
	return level
}

func attrInt(n *html.Node, name string, def int) int {
	v, ok := n.Attr(name)
	if !ok {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil || i < 1 {
		return def
	}
	return i
}

// Describe renders a readable summary of a box for snapshots and messages.
func Describe(b *Box) string {
	if b.Node != nil {
		return b.Node.Path()
	}
	return fmt.Sprintf("anonymous(%d)", b.Kind)
}
