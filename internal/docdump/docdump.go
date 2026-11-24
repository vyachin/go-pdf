// Package docdump serializes the document model the converter prepares before
// layout, as canonical XML.
//
// A dump is the golden artifact for the parsing, cascade and profile stages:
// it shows the elements the engine understood, their effective styles, the
// page setup, the margin boxes, the font faces and the diagnostics. It stops
// before layout on purpose, so it never contains line breaks, table widths or
// resolved page counters.
package docdump

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/profile"
)

// Version is the format version written to the root element. It changes when
// the serializer output changes in a way that is not a document change.
const Version = "1"

// Input is everything the converter has prepared when a dump is taken: the
// parsed document, the cascade, the collected stylesheets, the resolved page
// setup and the diagnostics of the preparation stage.
type Input struct {
	Document *html.Document
	Cascade  *css.Cascade
	Sheets   []*css.Stylesheet
	Size     css.PageSize
	Margin   [4]int32
	Boxes    map[string][]css.ContentItem
	Diags    diag.List
}

// XML renders the model as canonical XML: attributes are sorted, one element
// per line, two-space indentation and LF endings. The output depends only on
// the input, so two dumps of the same model are byte identical.
func XML(in Input) []byte {
	styles := Compute(in)
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<document version=\"" + Version + "\">\n")
	w := &writer{styles: styles}
	w.meta(&b, in.Document)
	w.page(&b, in.Size, in.Margin)
	w.marginBoxes(&b, in.Boxes)
	w.fontFaces(&b, in.Sheets)
	if in.Document != nil {
		if n := in.Document.HTML(); n != nil {
			w.element(&b, n, 1, false)
		}
	}
	w.diagnostics(&b, in.Diags)
	b.WriteString("</document>\n")
	return []byte(b.String())
}

// Compute resolves the style of every element in the element chain, starting
// from the html element and following the parent pointers the cascade uses. It
// is the same traversal the serializer reads, exposed so tests can compare it
// with the styles layout actually puts on boxes.
func Compute(in Input) map[*html.Node]*css.Style {
	out := map[*html.Node]*css.Style{}
	if in.Document == nil || in.Cascade == nil {
		return out
	}
	var htmlStyle *css.Style
	if n := in.Document.HTML(); n != nil {
		htmlStyle = in.Cascade.Compute(n, nil)
		out[n] = htmlStyle
	}
	body := in.Document.Body()
	if body == nil {
		return out
	}
	bodyStyle := in.Cascade.Compute(body, htmlStyle)
	out[body] = bodyStyle
	walkStyles(body, bodyStyle, in.Cascade, out)
	return out
}

func walkStyles(parent *html.Node, parentStyle *css.Style, cascade *css.Cascade, out map[*html.Node]*css.Style) {
	for _, child := range parent.Children {
		if child.Type != html.ElementNode {
			continue
		}
		style := cascade.Compute(child, parentStyle)
		out[child] = style
		walkStyles(child, style, cascade, out)
	}
}

type writer struct {
	styles map[*html.Node]*css.Style
}

// meta renders the document metadata taken from the head. The head itself is
// not part of the flow, so it appears only through this element.
func (w *writer) meta(b *strings.Builder, doc *html.Document) {
	if doc == nil {
		b.WriteString("  <meta/>\n")
		return
	}
	title := ""
	if head := doc.Head(); head != nil {
		for _, c := range head.Children {
			if c.Type == html.ElementNode && c.Tag == "title" {
				title = strings.TrimSpace(c.TextContent())
			}
		}
	}
	attrs := []prop{}
	if title != "" {
		attrs = append(attrs, prop{"title", title})
	}
	for _, name := range []string{"author", "description", "keywords"} {
		if v := metaContent(doc, name); v != "" {
			attrs = append(attrs, prop{name, v})
		}
	}
	b.WriteString("  <meta")
	for _, a := range attrs {
		b.WriteString(" " + a.name + "=\"" + escAttr(a.value) + "\"")
	}
	b.WriteString("/>\n")
}

func metaContent(doc *html.Document, name string) string {
	head := doc.Head()
	if head == nil {
		return ""
	}
	for _, c := range head.Children {
		if c.Type != html.ElementNode || c.Tag != "meta" {
			continue
		}
		if n, _ := c.Attr("name"); strings.EqualFold(n, name) {
			v, _ := c.Attr("content")
			return v
		}
	}
	return ""
}

func (w *writer) page(b *strings.Builder, size css.PageSize, margin [4]int32) {
	b.WriteString("  <page width=\"" + points(size.Width) + "\" height=\"" + points(size.Height) + "\"")
	for i, side := range []string{"top", "right", "bottom", "left"} {
		b.WriteString(" margin-" + side + "=\"" + points(margin[i]) + "\"")
	}
	b.WriteString("/>\n")
}

func (w *writer) marginBoxes(b *strings.Builder, boxes map[string][]css.ContentItem) {
	names := make([]string, 0, len(boxes))
	for name := range boxes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString("  <margin-box name=\"" + escAttr(name) + "\" content=\"" + escAttr(contentItems(boxes[name])) + "\"/>\n")
	}
}

func contentItems(items []css.ContentItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case css.ContentText:
			parts = append(parts, strconv.Quote(item.Text))
		case css.ContentPageNumber:
			parts = append(parts, "counter(page)")
		case css.ContentPageCount:
			parts = append(parts, "counter(pages)")
		case css.ContentTarget:
			parts = append(parts, "target-counter(attr(href), page)")
		}
	}
	return strings.Join(parts, " | ")
}

func (w *writer) fontFaces(b *strings.Builder, sheets []*css.Stylesheet) {
	for _, rule := range css.RulesForPrint(sheets) {
		if rule.Kind != css.FontFaceRule {
			continue
		}
		family, src := "", ""
		weight, italic := 400, false
		for _, d := range rule.Decls {
			switch d.Property {
			case "font-family":
				st := css.DefaultStyle()
				if err := css.ApplyDecl(st, "font-family", d.Value, 12000); err == nil && len(st.FontFamily) > 0 {
					family = st.FontFamily[0]
				}
			case "src":
				if source, err := profile.ParseFontFaceSrc(d.Value); err == nil {
					src = source.Path
				}
			case "font-weight":
				st := css.DefaultStyle()
				if err := css.ApplyDecl(st, "font-weight", d.Value, 12000); err == nil {
					weight = st.FontWeight
				}
			case "font-style":
				st := css.DefaultStyle()
				if err := css.ApplyDecl(st, "font-style", d.Value, 12000); err == nil {
					italic = st.FontStyle == css.FontItalic
				}
			}
		}
		if family == "" || src == "" {
			continue
		}
		style := "normal"
		if italic {
			style = "italic"
		}
		b.WriteString("  <font-face family=\"" + escAttr(family) + "\" src=\"" + escAttr(src) +
			"\" weight=\"" + strconv.Itoa(weight) + "\" style=\"" + style + "\"/>\n")
	}
}

func (w *writer) diagnostics(b *strings.Builder, list diag.List) {
	sorted := append(diag.List(nil), list...)
	sorted.Sort()
	if len(sorted) == 0 {
		b.WriteString("  <diagnostics/>\n")
		return
	}
	b.WriteString("  <diagnostics>\n")
	for _, d := range sorted {
		b.WriteString("    <diagnostic severity=\"" + d.Severity.String() + "\" code=\"" + escAttr(d.Code) + "\"")
		if at := position(d.Pos); at != "" {
			b.WriteString(" at=\"" + escAttr(at) + "\"")
		}
		if d.Element != "" {
			b.WriteString(" element=\"" + escAttr(d.Element) + "\"")
		}
		b.WriteString(">" + escText(d.Message) + "</diagnostic>\n")
	}
	b.WriteString("  </diagnostics>\n")
}

func position(p diag.Position) string {
	if p.File == "" {
		return ""
	}
	switch {
	case p.Line > 0 && p.Col > 0:
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
	case p.Line > 0:
		return fmt.Sprintf("%s:%d", p.File, p.Line)
	default:
		return p.File
	}
}

// metadataTags are elements that carry metadata rather than flow content. They
// are represented by <meta> or skipped, never by their text.
var metadataTags = map[string]bool{
	"head": true, "title": true, "meta": true, "link": true, "style": true, "script": true,
}

// element writes one element. inline suppresses indentation and line breaks for
// mixed content, where whitespace is part of the text.
func (w *writer) element(b *strings.Builder, n *html.Node, depth int, inline bool) {
	if metadataTags[n.Tag] {
		return
	}
	style := w.styles[n]
	attrs := w.attrs(n, style)
	indent := strings.Repeat("  ", depth)
	if !inline {
		b.WriteString(indent)
	}
	b.WriteString("<" + n.Tag)
	for _, a := range attrs {
		b.WriteString(" " + a.name + "=\"" + escAttr(a.value) + "\"")
	}

	type kid struct {
		node *html.Node
		text string
	}
	var kids []kid
	mixed := false
	pre := style != nil && style.WhiteSpace == css.WhiteSpacePre
	for i, child := range n.Children {
		switch child.Type {
		case html.TextNode:
			text := w.text(n, i, pre)
			if text == "" {
				continue
			}
			kids = append(kids, kid{text: text})
			mixed = true
		case html.ElementNode:
			if metadataTags[child.Tag] {
				continue
			}
			kids = append(kids, kid{node: child})
		}
	}
	if len(kids) == 0 {
		b.WriteString("/>")
		if !inline {
			b.WriteString("\n")
		}
		return
	}
	b.WriteString(">")
	if mixed || inline {
		for _, k := range kids {
			if k.node != nil {
				w.element(b, k.node, 0, true)
			} else {
				b.WriteString(escText(k.text))
			}
		}
		b.WriteString("</" + n.Tag + ">")
		if !inline {
			b.WriteString("\n")
		}
		return
	}
	b.WriteString("\n")
	for _, k := range kids {
		w.element(b, k.node, depth+1, false)
	}
	b.WriteString(indent + "</" + n.Tag + ">\n")
}

// text normalises one text node. Whitespace runs collapse to a single space;
// a leading or trailing space survives only when it separates inline content.
// Inside a pre element the text is kept verbatim.
func (w *writer) text(parent *html.Node, index int, pre bool) string {
	s := parent.Children[index].Data
	if pre {
		return normalizeNewlines(s)
	}
	lead := startsWithSpace(s)
	trail := endsWithSpace(s)
	core := strings.Join(strings.Fields(s), " ")
	if core == "" {
		if lead && trail && w.inlineNeighbour(parent, index, -1) && w.inlineNeighbour(parent, index, 1) {
			return " "
		}
		return ""
	}
	if lead && w.inlineNeighbour(parent, index, -1) {
		core = " " + core
	}
	if trail && w.inlineNeighbour(parent, index, 1) {
		core += " "
	}
	return core
}

// inlineNeighbour reports whether the nearest content sibling in the given
// direction is inline-level. Comments and doctypes are skipped.
func (w *writer) inlineNeighbour(parent *html.Node, index, dir int) bool {
	for i := index + dir; i >= 0 && i < len(parent.Children); i += dir {
		sib := parent.Children[i]
		switch sib.Type {
		case html.TextNode:
			return strings.TrimSpace(sib.Data) != ""
		case html.ElementNode:
			if metadataTags[sib.Tag] {
				continue
			}
			style := w.styles[sib]
			return style != nil && style.Display == css.DisplayInline
		}
	}
	return false
}

func startsWithSpace(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}

func endsWithSpace(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(r)
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

type prop struct {
	name  string
	value string
}

// inheritedProps lists the attributes that follow inheritance. They are elided
// when they match the parent; every other attribute is elided when it matches
// the initial value, so an element shows only what the cascade changed. The
// list mirrors css.Table.
var inheritedProps = map[string]bool{
	"color": true, "font-family": true, "font-size": true, "font-style": true,
	"font-weight": true, "line-height": true, "list-style-type": true,
	"orphans": true, "text-align": true, "text-indent": true,
	"white-space": true, "widows": true,
}

// attrs returns id, class and every computed property whose value differs from
// the parent's. The initial style is the base for an element without a parent.
func (w *writer) attrs(n *html.Node, style *css.Style) []prop {
	var out []prop
	if v, ok := n.Attr("id"); ok && v != "" {
		out = append(out, prop{"id", v})
	}
	if v, ok := n.Attr("class"); ok && v != "" {
		out = append(out, prop{"class", v})
	}
	if style == nil {
		return out
	}
	parent := styleMap(w.styles[n.Parent])
	for _, p := range styleList(style) {
		// An inherited property is elided when it matches the parent, because
		// that is the value inheritance would produce. Every other property is
		// elided when it matches the initial value, so an element shows only
		// what the cascade actually changed.
		base := initialProps
		if inheritedProps[p.name] && parent != nil {
			base = parent
		}
		if v, ok := base[p.name]; ok && v == p.value {
			continue
		}
		out = append(out, p)
	}
	out = compressSides(out)
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// compressSides writes a uniform four-sided value as the shorthand the profile
// actually supports (margin, padding, border). A partial set stays in longhand,
// because a one-value shorthand would change the other sides.
func compressSides(list []prop) []prop {
	for _, group := range []struct {
		name  string
		sides [4]string
	}{
		{"margin", [4]string{"margin-top", "margin-right", "margin-bottom", "margin-left"}},
		{"padding", [4]string{"padding-top", "padding-right", "padding-bottom", "padding-left"}},
		{"border", [4]string{"border-top", "border-right", "border-bottom", "border-left"}},
	} {
		list = mergeSides(list, group.name, group.sides)
	}
	return list
}

func mergeSides(list []prop, name string, sides [4]string) []prop {
	values := make([]string, 4)
	found := [4]int{-1, -1, -1, -1}
	for idx, p := range list {
		for i, side := range sides {
			if p.name == side {
				values[i] = p.value
				found[i] = idx
			}
		}
	}
	for _, idx := range found {
		if idx < 0 {
			return list
		}
	}
	for i := 1; i < 4; i++ {
		if values[i] != values[0] {
			return list
		}
	}
	out := make([]prop, 0, len(list)-3)
	for idx, p := range list {
		if idx == found[0] || idx == found[1] || idx == found[2] || idx == found[3] {
			continue
		}
		out = append(out, p)
	}
	return append(out, prop{name, values[0]})
}

func styleMap(style *css.Style) map[string]string {
	list := styleList(style)
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]string, len(list))
	for _, p := range list {
		out[p.name] = p.value
	}
	return out
}

// initialProps is the baseline for properties that are not inherited.
var initialProps = styleMap(css.DefaultStyle())

func styleList(style *css.Style) []prop {
	if style == nil {
		return nil
	}
	out := []prop{
		{"background-color", colorValue(style.Background)},
		{"border-bottom", borderValue(style.Border[2])},
		{"border-collapse", borderCollapseValue(style.BorderCollapse)},
		{"border-left", borderValue(style.Border[3])},
		{"border-right", borderValue(style.Border[1])},
		{"border-top", borderValue(style.Border[0])},
		{"break-after", breakValue(style.BreakAfter)},
		{"break-before", breakValue(style.BreakBefore)},
		{"break-inside", breakValue(style.BreakInside)},
		{"color", colorValue(style.Color)},
		{"display", displayValue(style.Display)},
		{"font-family", strings.Join(style.FontFamily, ", ")},
		{"font-size", points(style.FontSize)},
		{"font-style", fontStyleValue(style.FontStyle)},
		{"font-weight", strconv.Itoa(style.FontWeight)},
		{"height", style.Height.String()},
		{"line-height", lineHeightValue(style)},
		{"list-style-type", listStyleValue(style.ListStyleType)},
		{"margin-bottom", offsetValue(style.Margin[2])},
		{"margin-left", offsetValue(style.Margin[3])},
		{"margin-right", offsetValue(style.Margin[1])},
		{"margin-top", offsetValue(style.Margin[0])},
		{"orphans", strconv.Itoa(style.Orphans)},
		{"padding-bottom", offsetValue(style.Padding[2])},
		{"padding-left", offsetValue(style.Padding[3])},
		{"padding-right", offsetValue(style.Padding[1])},
		{"padding-top", offsetValue(style.Padding[0])},
		{"text-align", textAlignValue(style.TextAlign)},
		{"text-indent", offsetValue(style.TextIndent)},
		{"vertical-align", verticalAlignValue(style.VerticalAlign)},
		{"white-space", whiteSpaceValue(style.WhiteSpace)},
		{"widows", strconv.Itoa(style.Widows)},
		{"width", style.Width.String()},
	}
	if len(style.AfterContent) > 0 {
		out = append(out, prop{"after-content", contentItems(style.AfterContent)})
	}
	// A background image is written only when a box has one, so the initial
	// "none" needs no entry and an element without it stays clean.
	if style.BackgroundImage != "" {
		out = append(out, prop{"background-image", "url(" + style.BackgroundImage + ")"})
		// The initial auto is elided like every other initial value; the
		// profile documents that it behaves as cover.
		if style.BackgroundSize != css.BackgroundSizeAuto {
			out = append(out, prop{"background-size", backgroundSizeValue(style.BackgroundSize)})
		}
	}
	return out
}

func points(mp int32) string { return css.Abs(mp).String() }

// offsetValue formats a margin, padding or text-indent length. Auto is not a
// supported value for these properties and layout resolves it to zero, so the
// dump shows the effective zero instead of an "auto" the profile rejects.
func offsetValue(l css.Length) string {
	if l.Kind == css.Auto {
		return "0pt"
	}
	return l.String()
}

func colorValue(c css.Color) string {
	switch {
	case c.Transparent:
		return "transparent"
	case !c.Set:
		return "unset"
	default:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
}

func borderValue(side css.BorderSide) string {
	if side.Style == css.BorderNone || side.Width <= 0 {
		return "none"
	}
	return points(side.Width) + " solid " + colorValue(side.Color)
}

func backgroundSizeValue(v css.BackgroundSize) string {
	if v == css.BackgroundSizeCover {
		return "cover"
	}
	return "auto"
}

func borderCollapseValue(v css.BorderCollapse) string {
	if v == css.BorderCollapseCollapse {
		return "collapse"
	}
	return "separate"
}

func displayValue(v css.Display) string {
	switch v {
	case css.DisplayBlock:
		return "block"
	case css.DisplayInlineBlock:
		return "inline-block"
	case css.DisplayListItem:
		return "list-item"
	case css.DisplayTable:
		return "table"
	case css.DisplayTableRow:
		return "table-row"
	case css.DisplayTableCell:
		return "table-cell"
	case css.DisplayNone:
		return "none"
	default:
		return "inline"
	}
}

func fontStyleValue(v css.FontStyle) string {
	if v == css.FontItalic {
		return "italic"
	}
	return "normal"
}

func lineHeightValue(style *css.Style) string {
	if style.LineHeightFactor != 0 {
		return strconv.FormatFloat(style.LineHeightFactor, 'g', -1, 64)
	}
	return style.LineHeight.String()
}

func listStyleValue(v css.ListStyleType) string {
	if v == css.ListDecimal {
		return "decimal"
	}
	return "disc"
}

func textAlignValue(v css.TextAlign) string {
	switch v {
	case css.AlignRight:
		return "right"
	case css.AlignCenter:
		return "center"
	case css.AlignJustify:
		return "justify"
	default:
		return "left"
	}
}

func verticalAlignValue(v css.VerticalAlign) string {
	switch v {
	case css.VATop:
		return "top"
	case css.VAMiddle:
		return "middle"
	case css.VABottom:
		return "bottom"
	default:
		return "baseline"
	}
}

func whiteSpaceValue(v css.WhiteSpace) string {
	if v == css.WhiteSpacePre {
		return "pre"
	}
	return "normal"
}

func breakValue(v css.Break) string {
	switch v {
	case css.BreakPage:
		return "page"
	case css.BreakAvoid:
		return "avoid"
	default:
		return "auto"
	}
}

// escText escapes text content. Newlines stay literal so a pre block reads as
// it will be rendered.
func escText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escAttr escapes an attribute value. Whitespace that XML would normalise is
// written as a character reference.
func escAttr(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\n':
			b.WriteString("&#10;")
		case '\r':
			b.WriteString("&#13;")
		case '\t':
			b.WriteString("&#9;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
