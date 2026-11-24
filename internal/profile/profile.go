// Package profile validates an HTML document against the CSS profile and
// exposes the profile as documentation.
package profile

import (
	"fmt"
	"sort"
	"strconv"
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
		validateSheet(doc, sheet, &out)
	}

	walkElements(doc.Root, func(n *html.Node) {
		if n.Tag == "script" {
			out.Warnf("html/ignored-script", doc.Pos(n), n.Path(), "scripts are not executed and were ignored")
			return
		}
		if !css.SupportedElements[n.Tag] {
			out.Errorf("html/unsupported-element", doc.Pos(n), n.Path(), "element <%s> is not supported", n.Tag)
		}
		if n.Tag == "td" || n.Tag == "th" {
			for _, name := range []string{"colspan", "rowspan"} {
				if v, ok := n.Attr(name); ok {
					i, convErr := strconv.Atoi(strings.TrimSpace(v))
					if convErr != nil || i < 1 || i > css.MaxTableSpan {
						out.Errorf("html/invalid-span", doc.Pos(n), n.Path(), "%s value %q is outside 1..%d", name, v, css.MaxTableSpan)
					}
				}
			}
		}
		if style, ok := n.Attr("style"); ok {
			decls, probs := css.ParseDeclarations(style)
			for _, prob := range probs {
				out.Errorf("css/syntax", doc.Pos(n), n.Path(), "%s", prob.Message)
			}
			validateDeclarations(doc, doc.Pos(n), n.Path(), decls, &out)
		}
	})

	out.Sort()
	if opts.Strict {
		out.EscalateWarnings()
	}
	return out
}

func walkElements(n *html.Node, fn func(*html.Node)) {
	if n.Type == html.ElementNode {
		fn(n)
	}
	for _, c := range n.Children {
		walkElements(c, fn)
	}
}

func validateSheet(doc *html.Document, sh *css.Stylesheet, out *diag.List) {
	for _, p := range sh.Problems {
		out.Errorf("css/syntax", toDiag(sh.Position(p.Offset)), "", "%s", p.Message)
	}
	validateRules(doc, sh.Rules, out)
}

func validateRules(doc *html.Document, rules []css.Rule, out *diag.List) {
	for _, r := range rules {
		switch r.Kind {
		case css.StyleRule:
			if r.Err != nil {
				out.Errorf("css/unsupported-selector", rulePos(r), "", "selector %q: %v", r.SelectorText, r.Err)
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
			name := strings.ToLower(strings.TrimSpace(r.Name))
			if name != "print" && name != "all" {
				out.Errorf("css/unsupported-media", rulePos(r), "", "@media %q is not supported, only print and all", r.Name)
				continue
			}
			validateRules(doc, r.Rules, out)
		case css.UnknownAtRule:
			out.Errorf("css/unsupported-at-rule", rulePos(r), "", "@%s is not supported", r.Name)
		}
	}
}

// validateGeneratedContent checks the single supported pseudo-element rule,
// a::after with a content property.
func validateGeneratedContent(doc *html.Document, r css.Rule, pseudo string, out *diag.List) {
	pos := rulePos(r)
	if pseudo != "after" {
		out.Errorf("css/unsupported-selector", pos, "", "pseudo-element ::%s is not supported, only a::after", pseudo)
		return
	}
	// Any ancestor part is allowed, but the generated content must belong to an
	// anchor: the last compound has to be <a>.
	if len(r.Selector.Compounds) == 0 || r.Selector.Compounds[len(r.Selector.Compounds)-1].Tag != "a" {
		out.Errorf("css/unsupported-selector", pos, "", "only a::after is supported, got %q", r.SelectorText)
		return
	}
	for _, d := range r.Decls {
		declPos := declPosOr(d, pos)
		if d.Property != "content" {
			out.Errorf("css/unsupported-property", declPos, "", "only content is allowed inside a::after, got %q", d.Property)
			continue
		}
		if _, err := css.ParseContent(d.Value); err != nil {
			out.Errorf("css/unsupported-value", declPos, "", "content: %v", err)
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
			out.Errorf("css/unsupported-property", pos, elementPath, "property %q is not supported", d.Property)
			continue
		}
		st := css.DefaultStyle()
		if err := css.ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
			out.Errorf("css/unsupported-value", pos, elementPath, "%s: %v", d.Property, err)
		}
	}
}

func validateFontFace(doc *html.Document, r css.Rule, out *diag.List) {
	hasFamily, hasSrc := false, false
	for _, d := range r.Decls {
		pos := declPosOr(d, rulePos(r))
		if !fontFaceProperties[d.Property] {
			out.Errorf("css/unsupported-property", pos, "", "property %q is not supported inside @font-face", d.Property)
			continue
		}
		switch d.Property {
		case "font-family":
			hasFamily = true
			st := css.DefaultStyle()
			if err := css.ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
				out.Errorf("css/unsupported-value", pos, "", "font-family: %v", err)
			}
		case "src":
			hasSrc = true
			if _, err := ParseFontFaceSrc(d.Value); err != nil {
				out.Errorf("css/unsupported-value", pos, "", "src: %v", err)
			}
		case "font-weight", "font-style":
			st := css.DefaultStyle()
			if err := css.ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
				out.Errorf("css/unsupported-value", pos, "", "%s: %v", d.Property, err)
			}
		}
	}
	if !hasFamily || !hasSrc {
		out.Errorf("css/incomplete-font-face", rulePos(r), "", "@font-face requires font-family and src")
	}
}

func validatePageRule(doc *html.Document, r css.Rule, out *diag.List) {
	if name := strings.TrimSpace(r.Name); name != "" {
		out.Errorf("css/unsupported-page-selector", rulePos(r), "", "@page %q selectors are not supported", name)
	}
	for _, d := range r.Decls {
		pos := declPosOr(d, rulePos(r))
		switch d.Property {
		case "size":
			if _, err := css.ParsePageSizeSpec(d.Value); err != nil {
				out.Errorf("css/unsupported-value", pos, "", "size: %v", err)
			}
		case "margin":
			if _, err := css.ParsePageMargin(d.Value); err != nil {
				out.Errorf("css/unsupported-value", pos, "", "margin: %v", err)
			}
		default:
			out.Errorf("css/unsupported-property", pos, "", "property %q is only allowed inside a margin box, not in @page", d.Property)
		}
	}
}

func validatePageMargin(doc *html.Document, r css.Rule, out *diag.List) {
	if !css.MarginBoxNames[r.Name] {
		out.Errorf("css/unsupported-margin-box", rulePos(r), "", "margin box @%s is not supported", r.Name)
		return
	}
	for _, d := range r.Decls {
		pos := declPosOr(d, rulePos(r))
		if d.Property != "content" {
			out.Errorf("css/unsupported-property", pos, "", "property %q is not supported inside @%s", d.Property, r.Name)
			continue
		}
		if _, err := css.ParseContent(d.Value); err != nil {
			out.Errorf("css/unsupported-value", pos, "", "content: %v", err)
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

// Text renders the profile as human readable documentation, used by the CLI.
func Text() string {
	var b strings.Builder
	b.WriteString("Supported elements:\n")
	for _, name := range css.ElementOrder() {
		fmt.Fprintf(&b, "  %s\n", name)
	}
	b.WriteString("\nSupported properties:\n")
	for _, name := range css.PropOrder {
		spec := css.Table[name]
		inherited := ""
		if spec.Inherited {
			inherited = " (inherited)"
		}
		fmt.Fprintf(&b, "  %s%s\n", name, inherited)
	}
	b.WriteString("\nPage rules:\n")
	b.WriteString("  @page { size; margin }\n")
	b.WriteString("  margin boxes @top-left, @top-center, @top-right,\n")
	b.WriteString("               @bottom-left, @bottom-center, @bottom-right\n")
	b.WriteString("  content values: strings, counter(page), counter(pages), target-counter(attr(href), page)\n")
	b.WriteString("\nEverything outside this list is rejected by validation.\n")
	return b.String()
}

// Elements returns the supported element names, sorted.
func Elements() []string { return css.ElementOrder() }

// Properties returns the supported property names in profile order.
func Properties() []string {
	out := make([]string, len(css.PropOrder))
	copy(out, css.PropOrder)
	sort.Strings(out)
	return out
}
