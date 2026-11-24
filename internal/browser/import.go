// Package browser lowers browser-oriented HTML and CSS to the deterministic
// profile. It is a compatibility layer, not a browser: it understands the
// constructs commonly found in HTML exported from design tools and in
// certificate-like templates, rewrites them into supported CSS and reports
// every approximation it makes.
//
// The lowering runs before profile validation, so a document that is accepted
// after Import is valid for the same strict engine that rejects it as-is.
package browser

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

// Options carries the page geometry the lowering needs: percentages and
// viewport units resolve against the printed page.
type Options struct {
	PageSize css.PageSize
	Margin   [4]int32
}

// Result reports every transformation and every construct that could not be
// lowered. An error diagnostic means the document is not supported and must be
// fixed by hand.
type Result struct {
	Diagnostics diag.List
}

// Import rewrites doc and sheets in place so that the profile accepts them.
func Import(doc *html.Document, sheets []*css.Stylesheet, opts Options) Result {
	var res Result
	for _, rule := range dropNonPrintMedia(sheets) {
		res.Diagnostics.Warnf("browser/media", rulePosition(rule), "",
			"@media %q does not apply to print and was dropped", rule.Name)
	}

	// First cascade: it sees selectors and raw declarations, but not values
	// that still need lowering. It is used for the root font size and custom
	// properties only.
	first := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	root := doc.HTML()
	rootFont := int32(12000)
	if root != nil {
		rootFont = first.Compute(root, nil).FontSize
	}
	if rootFont <= 0 {
		rootFont = 12000
	}
	vars := customProperties(first, root)
	lowerDeclarations(sheets, rootFont, opts, vars)

	// Second cascade: values are lowered, so lengths and colours are final.
	second := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	styles := computeStyles(doc, second)

	lowerDOM(doc, second, styles, &res)
	rewriteRules(sheets, &res)
	return res
}

// contentSize returns the page area the content is laid out in, in
// millipoints.
func (o Options) contentSize() (int32, int32) {
	w := o.PageSize.Width - o.Margin[1] - o.Margin[3]
	h := o.PageSize.Height - o.Margin[0] - o.Margin[2]
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return w, h
}

// dropNonPrintMedia removes @media rules that cannot apply to a printed page.
// The profile deliberately rejects them; the importer treats them as
// screen-only.
func dropNonPrintMedia(sheets []*css.Stylesheet) []css.Rule {
	var dropped []css.Rule
	for _, sh := range sheets {
		if sh != nil {
			sh.Rules, dropped = filterMedia(sh.Rules, dropped)
		}
	}
	return dropped
}

func filterMedia(rules []css.Rule, dropped []css.Rule) ([]css.Rule, []css.Rule) {
	out := rules[:0]
	for _, r := range rules {
		if r.Kind == css.MediaRule {
			if mediaAppliesToPrint(r.Name) {
				r.Rules, dropped = filterMedia(r.Rules, dropped)
				out = append(out, r)
			} else {
				dropped = append(dropped, r)
			}
			continue
		}
		out = append(out, r)
	}
	return out, dropped
}

func mediaAppliesToPrint(name string) bool {
	for _, part := range strings.Split(name, ",") {
		q := strings.ToLower(strings.TrimSpace(part))
		if q == "" || q == "all" || q == "print" {
			return true
		}
	}
	return false
}

// customProperties collects custom property declarations from the document
// element. Inheritance of custom properties from other elements is not
// modelled; a template that relies on it must be lowered by hand.
func customProperties(c *css.Cascade, root *html.Node) map[string][]css.Token {
	out := map[string][]css.Token{}
	if root == nil {
		return out
	}
	for name, decl := range c.Declarations(root) {
		if strings.HasPrefix(name, "--") {
			out[name] = decl.Value
		}
	}
	return out
}

// lowerDeclarations applies the value-level lowering to every rule: custom
// properties, rem and viewport units, math functions and the background
// shorthand.
func lowerDeclarations(sheets []*css.Stylesheet, rootFont int32, opts Options, vars map[string][]css.Token) {
	width, height := opts.contentSize()
	walkDecls(sheets, func(d *css.Declaration) bool {
		if strings.HasPrefix(d.Property, "--") {
			return false
		}
		if len(vars) > 0 {
			d.Value = substituteVars(d.Value, vars)
		}
		d.Value = lowerUnits(d.Value, d.Property, rootFont, width, height)
		if d.Property == "background" {
			if _, err := css.ParseColor(d.Value); err == nil {
				d.Property = "background-color"
			}
		}
		return true
	})
}

// walkDecls calls fn for every declaration in every rule, including rules
// nested in @media. Returning false removes the declaration.
func walkDecls(sheets []*css.Stylesheet, fn func(*css.Declaration) bool) {
	for _, sh := range sheets {
		if sh == nil {
			continue
		}
		walkRuleDecls(sh.Rules, fn)
	}
}

func walkRuleDecls(rules []css.Rule, fn func(*css.Declaration) bool) {
	for i := range rules {
		r := &rules[i]
		if r.Kind == css.MediaRule {
			walkRuleDecls(r.Rules, fn)
			continue
		}
		if len(r.Decls) == 0 {
			continue
		}
		out := r.Decls[:0]
		for j := range r.Decls {
			if fn(&r.Decls[j]) {
				out = append(out, r.Decls[j])
			}
		}
		r.Decls = out
	}
}

// substituteVars replaces var(--name[, fallback]) in a declaration value.
func substituteVars(toks []css.Token, vars map[string][]css.Token) []css.Token {
	var out []css.Token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Kind != css.IdentToken || !strings.EqualFold(t.Text, "var") || i+1 >= len(toks) || toks[i+1].Kind != css.LParenToken {
			out = append(out, t)
			continue
		}
		depth := 0
		end := -1
		for j := i + 1; j < len(toks); j++ {
			switch toks[j].Kind {
			case css.LParenToken:
				depth++
			case css.RParenToken:
				depth--
				if depth == 0 {
					end = j
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			out = append(out, t)
			continue
		}
		name, fallback := splitVarArgs(toks[i+2 : end])
		if value, ok := vars[name]; ok {
			out = append(out, value...)
		} else if len(fallback) > 0 {
			out = append(out, fallback...)
		} else {
			// Leave the call untouched; validation reports the invalid value.
			out = append(out, toks[i:end+1]...)
		}
		i = end
	}
	return out
}

func splitVarArgs(toks []css.Token) (string, []css.Token) {
	trimmed := trimSpace(toks)
	if len(trimmed) == 0 || trimmed[0].Kind != css.IdentToken {
		return "", nil
	}
	name := trimmed[0].Text
	rest := trimSpace(trimmed[1:])
	if len(rest) > 0 && rest[0].Kind == css.CommaToken {
		return name, trimSpace(rest[1:])
	}
	return name, nil
}

func trimSpace(toks []css.Token) []css.Token {
	for len(toks) > 0 && toks[0].Kind == css.WhitespaceToken {
		toks = toks[1:]
	}
	for len(toks) > 0 && toks[len(toks)-1].Kind == css.WhitespaceToken {
		toks = toks[:len(toks)-1]
	}
	return toks
}

// lowerUnits rewrites rem and viewport units to px and evaluates a whole-value
// min()/max()/clamp() expression against the page area.
func lowerUnits(toks []css.Token, property string, rootFont, width, height int32) []css.Token {
	out := make([]css.Token, 0, len(toks))
	for _, t := range toks {
		if t.Kind == css.DimensionToken {
			if px, ok := unitToPixels(t.Unit, t.Num, rootFont, width, height); ok {
				out = append(out, pxToken(px))
				continue
			}
		}
		out = append(out, t)
	}
	basis := width
	switch property {
	case "height", "top", "bottom", "min-height", "max-height", "line-height":
		basis = height
	}
	if v, ok := evalMathFunction(out, basis); ok {
		return []css.Token{ptToken(v)}
	}
	return out
}

func unitToPixels(unit string, n float64, rootFont, width, height int32) (float64, bool) {
	switch unit {
	case "rem":
		return n * float64(rootFont) / 750, true
	case "vw":
		return n * float64(width) / 750, true
	case "vh", "svh", "lvh", "dvh":
		return n * float64(height) / 750, true
	case "vmin":
		m := width
		if height < m {
			m = height
		}
		return n * float64(m) / 750, true
	case "vmax":
		m := width
		if height > m {
			m = height
		}
		return n * float64(m) / 750, true
	}
	return 0, false
}

func pxToken(px float64) css.Token {
	text := strconv.FormatFloat(px, 'f', 3, 64)
	return css.Token{Kind: css.DimensionToken, Text: text + "px", Num: px, Unit: "px"}
}

func ptToken(mp int32) css.Token {
	text := strconv.FormatFloat(float64(mp)/1000, 'f', 3, 64)
	return css.Token{Kind: css.DimensionToken, Text: text + "pt", Num: float64(mp) / 1000, Unit: "pt"}
}

// evalMathFunction evaluates min(), max() and clamp() when the whole value is
// one such function with absolute or percentage operands.
func evalMathFunction(toks []css.Token, basis int32) (int32, bool) {
	f := trimSpace(toks)
	if len(f) < 3 || f[0].Kind != css.IdentToken || f[1].Kind != css.LParenToken || f[len(f)-1].Kind != css.RParenToken {
		return 0, false
	}
	name := strings.ToLower(f[0].Text)
	if name != "min" && name != "max" && name != "clamp" {
		return 0, false
	}
	inner := f[2 : len(f)-1]
	var values []int32
	depth := 0
	start := 0
	for i := 0; i <= len(inner); i++ {
		if i < len(inner) {
			switch inner[i].Kind {
			case css.LParenToken:
				depth++
			case css.RParenToken:
				depth--
			}
			if depth != 0 || inner[i].Kind != css.CommaToken {
				continue
			}
		}
		part := trimSpace(inner[start:i])
		if len(part) != 1 {
			return 0, false
		}
		v, ok := operand(part[0], basis)
		if !ok {
			return 0, false
		}
		values = append(values, v)
		start = i + 1
	}
	switch name {
	case "min":
		if len(values) == 0 {
			return 0, false
		}
		v := values[0]
		for _, x := range values[1:] {
			if x < v {
				v = x
			}
		}
		return v, true
	case "max":
		if len(values) == 0 {
			return 0, false
		}
		v := values[0]
		for _, x := range values[1:] {
			if x > v {
				v = x
			}
		}
		return v, true
	default:
		if len(values) != 3 {
			return 0, false
		}
		v := values[1]
		if v < values[0] {
			v = values[0]
		}
		if v > values[2] {
			v = values[2]
		}
		return v, true
	}
}

// operand resolves one math function argument.
func operand(t css.Token, basis int32) (int32, bool) {
	switch t.Kind {
	case css.NumberToken:
		if t.Num == 0 {
			return 0, true
		}
		return 0, false
	case css.PercentageToken:
		return int32(math.Round(float64(basis) * t.Num / 100)), true
	case css.DimensionToken:
		switch t.Unit {
		case "pt":
			return int32(math.Round(t.Num * 1000)), true
		case "mm":
			return int32(math.Round(t.Num * 72000 / 25.4)), true
		case "cm":
			return int32(math.Round(t.Num * 720000 / 25.4)), true
		case "in":
			return int32(math.Round(t.Num * 72000)), true
		case "px":
			return int32(math.Round(t.Num * 750)), true
		}
	}
	return 0, false
}

// computeStyles computes the style of every element, starting at the document
// element.
func computeStyles(doc *html.Document, c *css.Cascade) map[*html.Node]*css.Style {
	out := map[*html.Node]*css.Style{}
	var walk func(n *html.Node, parent *css.Style)
	walk = func(n *html.Node, parent *css.Style) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			st := c.Compute(n, parent)
			out[n] = st
			parent = st
		}
		for _, child := range n.Children {
			walk(child, parent)
		}
	}
	walk(doc.Root, nil)
	return out
}

// lowerDOM applies the structural lowering: flex columns, absolutely
// positioned cover images, border-box sizes and text-transform.
func lowerDOM(doc *html.Document, c *css.Cascade, styles map[*html.Node]*css.Style, res *Result) {
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type != html.ElementNode {
			for _, child := range n.Children {
				walk(child)
			}
			return
		}
		raw := c.Declarations(n)
		if declIdent(raw, "position") == "absolute" {
			lowerAbsolute(doc, n, raw, styles, c, res)
			return
		}
		switch declIdent(raw, "display") {
		case "flex", "inline-flex":
			lowerFlex(doc, n, raw, styles, c, res)
		}
		if declIdent(raw, "box-sizing") == "border-box" {
			if lowerBorderBox(n, raw, styles) {
				res.Diagnostics.Warnf("browser/box-sizing", doc.Pos(n), n.Path(),
					"border-box size lowered to the content-box model")
			}
		}
		switch transform := declIdent(raw, "text-transform"); transform {
		case "", "none":
		case "uppercase":
			transformText(n, strings.ToUpper)
			res.Diagnostics.Warnf("browser/text-transform", doc.Pos(n), n.Path(),
				"text-transform: uppercase applied to the text")
		case "lowercase":
			transformText(n, strings.ToLower)
			res.Diagnostics.Warnf("browser/text-transform", doc.Pos(n), n.Path(),
				"text-transform: lowercase applied to the text")
		case "capitalize":
			transformText(n, capitalize)
			res.Diagnostics.Warnf("browser/text-transform", doc.Pos(n), n.Path(),
				"text-transform: capitalize applied to the text")
		default:
			res.Diagnostics.Errorf("browser/unsupported-text-transform", doc.Pos(n), n.Path(),
				"text-transform: %s is not supported", transform)
		}
		for _, child := range append([]*html.Node(nil), n.Children...) {
			walk(child)
		}
	}
	walk(doc.Root)
}

// declIdent returns the first identifier of a declaration in the raw map.
func declIdent(raw map[string]css.Declaration, prop string) string {
	d, ok := raw[prop]
	if !ok {
		return ""
	}
	return identValue(d)
}

// identValue returns the first identifier of a declaration value.
func identValue(d css.Declaration) string {
	for _, t := range d.Value {
		if t.Kind == css.WhitespaceToken {
			continue
		}
		if t.Kind == css.IdentToken {
			return strings.ToLower(t.Text)
		}
		return ""
	}
	return ""
}

// lowerFlex lowers a single-line flex column to block layout with wrappers
// that preserve the flex items' vertical margins.
func lowerFlex(doc *html.Document, n *html.Node, raw map[string]css.Declaration, styles map[*html.Node]*css.Style, c *css.Cascade, res *Result) {
	direction := declIdent(raw, "flex-direction")
	if direction == "" {
		direction = "row"
	}
	if direction != "column" {
		res.Diagnostics.Errorf("browser/unsupported-flex", doc.Pos(n), n.Path(),
			"only a flex column can be lowered, got flex-direction: %s", direction)
		return
	}
	if justify := declIdent(raw, "justify-content"); justify != "" && justify != "flex-start" && justify != "start" {
		res.Diagnostics.Errorf("browser/unsupported-flex", doc.Pos(n), n.Path(),
			"only justify-content: flex-start can be lowered, got %s", justify)
		return
	}
	for _, prop := range []string{"flex-wrap", "flex-flow", "gap", "row-gap", "column-gap"} {
		if _, ok := raw[prop]; ok {
			res.Diagnostics.Errorf("browser/unsupported-flex", doc.Pos(n), n.Path(),
				"%s is not supported by the flex lowering", prop)
			return
		}
	}
	align := declIdent(raw, "align-items")
	center := align == "center"
	if align != "" && align != "center" && align != "flex-start" && align != "start" && align != "stretch" {
		res.Diagnostics.Errorf("browser/unsupported-flex", doc.Pos(n), n.Path(),
			"align-items: %s is not supported", align)
		return
	}
	children := append([]*html.Node(nil), n.Children...)
	for _, child := range children {
		if child.Type != html.ElementNode {
			continue
		}
		childRaw := c.Declarations(child)
		if declIdent(childRaw, "position") == "absolute" {
			continue // out of flow; handled on its own visit
		}
		for _, prop := range []string{"flex", "flex-grow", "flex-shrink", "flex-basis", "align-self", "order"} {
			if _, ok := childRaw[prop]; ok {
				res.Diagnostics.Errorf("browser/unsupported-flex", doc.Pos(child), child.Path(),
					"%s on a flex item is not supported by the flex lowering", prop)
				return
			}
		}
		fontSize := int32(12000)
		if st := styles[child]; st != nil {
			fontSize = st.FontSize
		}
		top, bottom, ok := margins(childRaw, fontSize)
		if !ok {
			res.Diagnostics.Errorf("browser/unsupported-flex", doc.Pos(child), child.Path(),
				"a percentage margin on a flex item is not supported")
			continue
		}
		var style []string
		if center {
			style = append(style, "text-align:center")
		}
		if top > 0 {
			style = append(style, "padding-top:"+formatMP(top))
		}
		if bottom > 0 {
			style = append(style, "padding-bottom:"+formatMP(bottom))
		}
		wrapper := &html.Node{
			Type:   html.ElementNode,
			Tag:    "div",
			Parent: n,
			Start:  child.Start,
			End:    child.End,
		}
		if len(style) > 0 {
			wrapper.Attrs = []html.Attr{{Key: "style", Value: strings.Join(style, ";")}}
		}
		appendStyle(child, "margin:0")
		wrapper.Children = []*html.Node{child}
		child.Parent = wrapper
		replaceChild(n, child, wrapper)
	}
	appendStyle(n, "display:block")
	if center {
		appendStyle(n, "text-align:center")
	}
	res.Diagnostics.Warnf("browser/flex", doc.Pos(n), n.Path(),
		"flex column lowered to block layout")
}

// lowerAbsolute turns an absolutely positioned, box-covering image into the
// background of its containing block.
func lowerAbsolute(doc *html.Document, n *html.Node, raw map[string]css.Declaration, styles map[*html.Node]*css.Style, c *css.Cascade, res *Result) {
	if n.Tag != "img" {
		res.Diagnostics.Errorf("browser/unsupported-position", doc.Pos(n), n.Path(),
			"only an absolutely positioned <img> can be lowered")
		return
	}
	parent := positionedAncestor(c, n)
	if parent == nil {
		res.Diagnostics.Errorf("browser/unsupported-position", doc.Pos(n), n.Path(),
			"an absolutely positioned image needs a positioned ancestor")
		return
	}
	if !coversContainingBlock(raw) {
		res.Diagnostics.Errorf("browser/unsupported-position", doc.Pos(n), n.Path(),
			"an absolutely positioned image must cover its containing block")
		return
	}
	if fit := declIdent(raw, "object-fit"); fit != "cover" {
		res.Diagnostics.Errorf("browser/unsupported-position", doc.Pos(n), n.Path(),
			"only object-fit: cover can be lowered, got %s", fit)
		return
	}
	if st := styles[parent]; st == nil || st.Height.Kind != css.MP {
		res.Diagnostics.Errorf("browser/unsupported-position", doc.Pos(n), n.Path(),
			"the containing block of the background image needs an explicit height")
		return
	}
	src, _ := n.Attr("src")
	if strings.TrimSpace(src) == "" {
		res.Diagnostics.Errorf("browser/unsupported-position", doc.Pos(n), n.Path(),
			"an absolutely positioned image has no src")
		return
	}
	appendStyle(parent, `background-image:url("`+src+`");background-size:cover`)
	removeChild(n.Parent, n)
	res.Diagnostics.Warnf("browser/background", doc.Pos(parent), parent.Path(),
		"absolutely positioned cover image lowered to a background image")
}

func positionedAncestor(c *css.Cascade, n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type != html.ElementNode {
			continue
		}
		switch declIdent(c.Declarations(p), "position") {
		case "relative", "absolute", "fixed", "sticky":
			return p
		}
	}
	return nil
}

func coversContainingBlock(raw map[string]css.Declaration) bool {
	if inset, ok := raw["inset"]; ok && allZero(inset.Value) {
		return true
	}
	for _, side := range []string{"top", "right", "bottom", "left"} {
		d, ok := raw[side]
		if !ok || !allZero(d.Value) {
			return false
		}
	}
	return true
}

func allZero(toks []css.Token) bool {
	n := 0
	for _, t := range toks {
		if t.Kind == css.WhitespaceToken {
			continue
		}
		if t.Kind == css.NumberToken && t.Num == 0 {
			n++
			continue
		}
		return false
	}
	return n >= 1 && n <= 4
}

// lowerBorderBox moves the padding and border of a border-box element out of
// its content size, which is what the engine's content-box model expects.
func lowerBorderBox(n *html.Node, raw map[string]css.Declaration, styles map[*html.Node]*css.Style) bool {
	changed := false
	st := css.DefaultStyle()
	if s := styles[n]; s != nil {
		st.FontSize = s.FontSize
	}
	for _, prop := range []string{"width", "height", "padding", "padding-top", "padding-right", "padding-bottom", "padding-left", "border"} {
		if d, ok := raw[prop]; ok {
			_ = css.ApplyDecl(st, prop, d.Value, st.FontSize)
		}
	}
	if st.Height.Kind == css.MP {
		reduce := st.Padding[0].Resolve(st.FontSize) + st.Padding[2].Resolve(st.FontSize) +
			borderWidth(st.Border[0]) + borderWidth(st.Border[2])
		if reduce > 0 && st.Height.Value > reduce {
			appendStyle(n, "height:"+formatMP(st.Height.Value-reduce))
			changed = true
		}
	}
	if st.Width.Kind == css.MP {
		reduce := st.Padding[3].Resolve(st.FontSize) + st.Padding[1].Resolve(st.FontSize) +
			borderWidth(st.Border[3]) + borderWidth(st.Border[1])
		if reduce > 0 && st.Width.Value > reduce {
			appendStyle(n, "width:"+formatMP(st.Width.Value-reduce))
			changed = true
		}
	}
	return changed
}

func borderWidth(side css.BorderSide) int32 {
	if side.Style == css.BorderNone {
		return 0
	}
	return side.Width
}

// margins returns the resolved top and bottom margins of a flex item.
func margins(raw map[string]css.Declaration, fontSize int32) (int32, int32, bool) {
	st := css.DefaultStyle()
	st.FontSize = fontSize
	for _, prop := range []string{"margin", "margin-top", "margin-right", "margin-bottom", "margin-left"} {
		if d, ok := raw[prop]; ok {
			_ = css.ApplyDecl(st, prop, d.Value, fontSize)
		}
	}
	top, okTop := resolveMargin(st.Margin[0], fontSize)
	bottom, okBottom := resolveMargin(st.Margin[2], fontSize)
	return top, bottom, okTop && okBottom
}

func resolveMargin(l css.Length, fontSize int32) (int32, bool) {
	switch l.Kind {
	case css.MP:
		return l.Value, true
	case css.Em:
		return l.Resolve(fontSize), true
	case css.Auto:
		return 0, true
	default:
		return 0, false
	}
}

func formatMP(mp int32) string {
	return strconv.FormatFloat(float64(mp)/1000, 'f', 3, 64) + "pt"
}

// transformText applies a text-transform function to the text below n.
func transformText(n *html.Node, fn func(string) string) {
	if n.Tag == "script" || n.Tag == "style" {
		return
	}
	for _, child := range n.Children {
		if child.Type == html.TextNode {
			child.Data = fn(child.Data)
			continue
		}
		transformText(child, fn)
	}
}

// capitalize uppercases the first letter of every word.
func capitalize(s string) string {
	var b strings.Builder
	atWordStart := true
	for _, r := range s {
		if atWordStart && unicode.IsLetter(r) {
			r = unicode.ToUpper(r)
			atWordStart = false
		} else if unicode.IsSpace(r) {
			atWordStart = true
		}
		b.WriteRune(r)
	}
	return b.String()
}

// rewriteRules removes properties that the lowering replaced and turns
// display:flex into display:block.
func rewriteRules(sheets []*css.Stylesheet, res *Result) {
	walkDecls(sheets, func(d *css.Declaration) bool {
		if strings.HasPrefix(d.Property, "--") {
			return false
		}
		if droppedProperties[d.Property] {
			res.Diagnostics.Warnf("browser/dropped-property", positionOf(d), "",
				"property %q is not part of the profile and was dropped by the browser import", d.Property)
			return false
		}
		if d.Property == "display" {
			switch identValue(*d) {
			case "flex":
				d.Value = []css.Token{{Kind: css.IdentToken, Text: "block"}}
			case "inline-flex":
				d.Value = []css.Token{{Kind: css.IdentToken, Text: "inline"}}
			}
		}
		return true
	})
}

var droppedProperties = map[string]bool{
	"position": true, "inset": true, "z-index": true, "object-fit": true,
	"overflow": true, "overflow-x": true, "overflow-y": true,
	"flex-direction": true, "justify-content": true, "align-items": true,
	"align-self": true, "flex": true, "flex-wrap": true, "flex-grow": true,
	"flex-shrink": true, "flex-basis": true, "gap": true,
	"box-sizing": true, "box-shadow": true, "isolation": true,
	"aspect-ratio": true, "min-height": true, "text-transform": true,
	"print-color-adjust": true, "-webkit-print-color-adjust": true,
	"color-scheme": true,
}

// rulePosition maps a rule to its source position, if the sheet kept it.
func rulePosition(r css.Rule) diag.Position {
	if r.Sheet == nil {
		return diag.Position{}
	}
	p := r.Sheet.Position(r.Offset)
	return diag.Position{File: p.File, Line: p.Line, Col: p.Col}
}

// positionOf maps a declaration to its source position, if the sheet kept it.
func positionOf(d *css.Declaration) diag.Position {
	if d.Sheet == nil {
		return diag.Position{}
	}
	p := d.Sheet.Position(d.Offset)
	return diag.Position{File: p.File, Line: p.Line, Col: p.Col}
}

// --- DOM helpers -----------------------------------------------------------

func appendStyle(n *html.Node, decls string) {
	if n == nil {
		return
	}
	existing, ok := n.Attr("style")
	if !ok || strings.TrimSpace(existing) == "" {
		setAttr(n, "style", decls)
		return
	}
	setAttr(n, "style", strings.TrimSuffix(strings.TrimSpace(existing), ";")+";"+decls)
}

func setAttr(n *html.Node, key, value string) {
	for i := range n.Attrs {
		if n.Attrs[i].Key == key {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, html.Attr{Key: key, Value: value})
}

func replaceChild(parent, old, replacement *html.Node) {
	for i, c := range parent.Children {
		if c == old {
			parent.Children[i] = replacement
			return
		}
	}
}

func removeChild(parent, n *html.Node) {
	if parent == nil {
		return
	}
	for i, c := range parent.Children {
		if c == n {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			return
		}
	}
}
