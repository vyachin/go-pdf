// Package profile validates an HTML document against the CSS profile and
// exposes the profile as documentation.
package profile

import (
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

// toDiag converts a stylesheet position into a diagnostic position.
func toDiag(p css.Position) diag.Position {
	return diag.Position{File: p.File, Line: p.Line, Col: p.Col}
}

// rulePos returns the source position of a rule.
func rulePos(r css.Rule) diag.Position {
	if r.Sheet == nil {
		return diag.Position{}
	}
	return toDiag(r.Sheet.Position(r.Offset))
}

// declPos returns the source position of a declaration.
func declPos(d css.Declaration) diag.Position {
	if d.Sheet == nil {
		return diag.Position{}
	}
	return toDiag(d.Sheet.Position(d.Offset))
}

var fontFaceProperties = map[string]bool{
	"font-family": true, "src": true, "font-weight": true, "font-style": true,
}

// Validate collects the document stylesheets and checks them and the document
// against the profile.
func Validate(doc *html.Document, opts Options) diag.List {
	sheets, sheetDiags := CollectStylesheets(doc, opts)
	return ValidateSheets(doc, opts, sheets, sheetDiags)
}

// ValidateSheets checks a document against the profile using stylesheets that
// were already collected, so a conversion reads each stylesheet once.
func ValidateSheets(doc *html.Document, opts Options, sheets []*css.Stylesheet, sheetDiags diag.List) diag.List {
	var out diag.List
	out = append(out, sheetDiags...)
	for _, sheet := range sheets {
		if err := opts.Budget.Add(1); err != nil {
			return out
		}
		validateSheet(doc, sheet, &out)
	}

	stopped := false
	walkElements(doc.Root, func(n *html.Node) bool {
		if err := opts.Budget.Add(1); err != nil {
			stopped = true
			return false
		}
		if n.Tag == "script" {
			out.Warnf(diag.HTMLIgnoredScript, doc.Pos(n), n.Path(), "scripts are not executed and were ignored")
			return true
		}
		if !css.SupportedElements[n.Tag] {
			out.Errorf(diag.HTMLUnsupportedElement, doc.Pos(n), n.Path(), "element <%s> is not supported", n.Tag)
		} else {
			for _, attr := range n.Attrs {
				if allowedAttribute(n.Tag, attr.Key) {
					continue
				}
				out.Warnf(diag.HTMLUnsupportedAttribute, doc.Pos(n), n.Path(), "attribute %q is not supported", attr.Key)
			}
		}
		if n.Tag == "td" || n.Tag == "th" {
			for _, name := range []string{"colspan", "rowspan"} {
				if v, ok := n.Attr(name); ok {
					if _, valid := css.ParseTableSpan(v); !valid {
						out.Errorf(diag.HTMLInvalidSpan, doc.Pos(n), n.Path(), "%s value %q is outside 1..%d", name, v, css.MaxTableSpan)
					}
				}
			}
		}
		if style, ok := n.Attr("style"); ok {
			decls, probs := css.ParseDeclarations(style)
			for _, prob := range probs {
				out.Errorf(diag.CSSSyntax, doc.Pos(n), n.Path(), "%s", prob.Message)
			}
			validateDeclarations(doc, doc.Pos(n), n.Path(), decls, &out)
		}
		return true
	})
	if stopped {
		// The caller reports the budget failure from Options.Budget; a partial
		// diagnostic list must not look like a complete validation.
		return out
	}
	out.Sort()
	if opts.Strict {
		out.EscalateWarnings()
	}
	return out
}

// allowedAttributes are accepted on every element.
var allowedAttributes = map[string]bool{
	"id": true, "class": true, "style": true, "hidden": true,
	"lang": true, "dir": true, "title": true,
}

// elementAttributes are accepted on the named element in addition to the
// global list. Keeping this table next to the profile documentation means a
// new attribute read by layout must be added in both places.
var elementAttributes = map[string]map[string]bool{
	"a":     {"href": true},
	"img":   {"src": true, "alt": true, "width": true, "height": true},
	"link":  {"rel": true, "href": true, "media": true},
	"style": {"media": true},
	"meta":  {"name": true, "content": true, "charset": true, "http-equiv": true},
	"td":    {"colspan": true, "rowspan": true},
	"th":    {"colspan": true, "rowspan": true},
	"ol":    {"start": true},
}

// allowedAttribute reports whether the profile reads the attribute.
func allowedAttribute(tag, name string) bool {
	if allowedAttributes[name] {
		return true
	}
	return elementAttributes[tag][name]
}

// walkElements calls fn for every element in document order. fn returns false
// to stop the walk.
func walkElements(n *html.Node, fn func(*html.Node) bool) bool {
	if n == nil {
		return true
	}
	if n.Type == html.ElementNode {
		if !fn(n) {
			return false
		}
	}
	for _, c := range n.Children {
		if !walkElements(c, fn) {
			return false
		}
	}
	return true
}

func validateSheet(doc *html.Document, sh *css.Stylesheet, out *diag.List) {
	for _, p := range sh.Problems {
		code := diag.Code(p.Code)
		if code == "" {
			code = diag.CSSSyntax
		}
		out.Errorf(code, toDiag(sh.Position(p.Offset)), "", "%s", p.Message)
	}
	validateRules(doc, sh.Rules, out)
}

// validateRules validates the rules in document order. It walks with an
// explicit stack: the parser bounds at-rule nesting, but validation must not
// add a second recursion that could disagree with that bound.
func validateRules(doc *html.Document, rules []css.Rule, out *diag.List) {
	type frame struct {
		rules []css.Rule
		next  int
	}
	stack := []frame{{rules: rules}}
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.next >= len(top.rules) {
			stack = stack[:len(stack)-1]
			continue
		}
		r := top.rules[top.next]
		top.next++
		switch r.Kind {
		case css.StyleRule:
			if r.Err != nil {
				out.Errorf(diag.CSSUnsupportedSelector, rulePos(r), "", "selector %q: %v", r.SelectorText, r.Err)
				continue
			}
			if pseudo := r.Selector.PseudoElement(); pseudo != "" {
				validateGeneratedContent(doc, r, pseudo, out)
				continue
			}
			validateDeclarations(doc, rulePos(r), "", r.Decls, out)
		case css.FontFaceRule:
			validateFontFace(doc, r, out)
		case css.PageRule:
			validatePageRule(doc, r, out)
		case css.PageMarginRule:
			validatePageMargin(doc, r, out)
		case css.MediaRule:
			// The query list is parsed by the same function that decides
			// whether the rules apply, so "@media print, screen" is accepted
			// for print instead of being rejected while it applies.
			if !css.MediaAppliesToPrint(r.Name) {
				out.Errorf(diag.CSSUnsupportedMedia, rulePos(r), "", "@media %q is not supported, only print and all", r.Name)
				continue
			}
			stack = append(stack, frame{rules: r.Rules})
		case css.UnknownAtRule:
			out.Errorf(diag.CSSUnsupportedAtRule, rulePos(r), "", "@%s is not supported", r.Name)
		}
	}
}

// validateGeneratedContent checks the single supported pseudo-element rule,
// a::after with a content property.
func validateGeneratedContent(doc *html.Document, r css.Rule, pseudo string, out *diag.List) {
	pos := rulePos(r)
	if pseudo != "after" {
		out.Errorf(diag.CSSUnsupportedSelector, pos, "", "pseudo-element ::%s is not supported, only a::after", pseudo)
		return
	}
	// Any ancestor part is allowed, but the generated content must belong to an
	// anchor: the last compound has to be <a>.
	if len(r.Selector.Compounds) == 0 || r.Selector.Compounds[len(r.Selector.Compounds)-1].Tag != "a" {
		out.Errorf(diag.CSSUnsupportedSelector, pos, "", "only a::after is supported, got %q", r.SelectorText)
		return
	}
	for _, d := range r.Decls {
		declPos := declPosOr(d, pos)
		if d.Property != "content" {
			out.Errorf(diag.CSSUnsupportedProperty, declPos, "", "only content is allowed inside a::after, got %q", d.Property)
			continue
		}
		items, err := css.ParseContent(d.Value)
		if err != nil {
			out.Errorf(diag.CSSUnsupportedValue, declPos, "", "content: %v", err)
			continue
		}
		for _, item := range items {
			if item.Kind == css.ContentPageNumber || item.Kind == css.ContentPageCount {
				out.Errorf(diag.CSSUnsupportedValue, declPos, "", "counter(page) is not available in a::after; use target-counter")
			}
		}
	}
}

func validateDeclarations(doc *html.Document, pos diag.Position, elementPath string, decls []css.Declaration, out *diag.List) {
	for _, d := range decls {
		pos := pos
		if p := declPos(d); p.File != "" {
			pos = p
		}
		if _, ok := css.Table[d.Property]; !ok {
			out.Errorf(diag.CSSUnsupportedProperty, pos, elementPath, "property %q is not supported", d.Property)
			continue
		}
		st := css.DefaultStyle()
		if err := css.ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
			out.Errorf(diag.CSSUnsupportedValue, pos, elementPath, "%s: %v", d.Property, err)
			continue
		}
		if d.Property == "display" && st.Display == css.DisplayInlineBlock {
			// Layout has no atomic inline-block box: it would silently lose the
			// width, height and decorations, so the profile rejects it.
			out.Errorf(diag.CSSUnsupportedValue, pos, elementPath, "display: inline-block is not supported")
		}
	}
}

func validateFontFace(doc *html.Document, r css.Rule, out *diag.List) {
	hasFamily, hasSrc := false, false
	for _, d := range r.Decls {
		pos := declPosOr(d, rulePos(r))
		if !fontFaceProperties[d.Property] {
			out.Errorf(diag.CSSUnsupportedProperty, pos, "", "property %q is not supported inside @font-face", d.Property)
			continue
		}
		switch d.Property {
		case "font-family":
			hasFamily = true
			st := css.DefaultStyle()
			if err := css.ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
				out.Errorf(diag.CSSUnsupportedValue, pos, "", "font-family: %v", err)
			}
		case "src":
			hasSrc = true
			if _, err := ParseFontFaceSrc(d.Value); err != nil {
				out.Errorf(diag.CSSUnsupportedValue, pos, "", "src: %v", err)
			}
		case "font-weight", "font-style":
			st := css.DefaultStyle()
			if err := css.ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
				out.Errorf(diag.CSSUnsupportedValue, pos, "", "%s: %v", d.Property, err)
			}
		}
	}
	if !hasFamily || !hasSrc {
		out.Errorf(diag.CSSIncompleteFontFace, rulePos(r), "", "@font-face requires font-family and src")
	}
}

func validatePageRule(doc *html.Document, r css.Rule, out *diag.List) {
	if name := strings.TrimSpace(r.Name); name != "" {
		out.Errorf(diag.CSSUnsupportedPageSelector, rulePos(r), "", "@page %q selectors are not supported", name)
	}
	for _, d := range r.Decls {
		pos := declPosOr(d, rulePos(r))
		switch d.Property {
		case "size":
			if _, err := css.ParsePageSizeSpec(d.Value); err != nil {
				out.Errorf(diag.CSSUnsupportedValue, pos, "", "size: %v", err)
			}
		case "margin":
			if _, err := css.ParsePageMargin(d.Value); err != nil {
				out.Errorf(diag.CSSUnsupportedValue, pos, "", "margin: %v", err)
			}
		default:
			out.Errorf(diag.CSSUnsupportedProperty, pos, "", "property %q is only allowed inside a margin box, not in @page", d.Property)
		}
	}
}

func validatePageMargin(doc *html.Document, r css.Rule, out *diag.List) {
	if !css.MarginBoxNames[r.Name] {
		out.Errorf(diag.CSSUnsupportedMarginBox, rulePos(r), "", "margin box @%s is not supported", r.Name)
		return
	}
	for _, d := range r.Decls {
		pos := declPosOr(d, rulePos(r))
		if d.Property != "content" {
			out.Errorf(diag.CSSUnsupportedProperty, pos, "", "property %q is not supported inside @%s", d.Property, r.Name)
			continue
		}
		items, err := css.ParseContent(d.Value)
		if err != nil {
			out.Errorf(diag.CSSUnsupportedValue, pos, "", "content: %v", err)
			continue
		}
		for _, item := range items {
			if item.Kind == css.ContentTarget {
				out.Errorf(diag.CSSUnsupportedValue, pos, "", "target-counter is not available in a margin box; use counter(page)")
			}
		}
	}
}

// declPosOr prefers the declaration's own position and falls back to the
// position of the enclosing rule or element.
func declPosOr(d css.Declaration, fallback diag.Position) diag.Position {
	if p := declPos(d); p.File != "" {
		return p
	}
	return fallback
}
