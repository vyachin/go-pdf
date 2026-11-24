// Package profile validates an HTML document against the CSS profile and
// exposes the profile as documentation.
package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

// ExtractStyleSheets collects every <style> element of the document into one
// stylesheet. Token offsets are shifted so diagnostics point into the original
// HTML file.
func ExtractStyleSheets(doc *html.Document) *css.Stylesheet {
	sh := &css.Stylesheet{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Tag == "style" {
			base := n.Start
			if len(n.Children) > 0 && n.Children[0].Type == html.TextNode {
				base = n.Children[0].Start
			}
			sheet := css.ParseWithBase([]byte(n.TextContent()), base)
			sh.Rules = append(sh.Rules, sheet.Rules...)
			for _, p := range sheet.Problems {
				sh.Problems = append(sh.Problems, css.Problem{Message: p.Message, Offset: p.Offset})
			}
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if doc.Root != nil {
		walk(doc.Root)
	}
	return sh
}

var fontFaceProperties = map[string]bool{
	"font-family": true, "src": true, "font-weight": true, "font-style": true,
}

// Validate checks the document and its stylesheets against the profile.
func Validate(doc *html.Document) diag.List {
	var out diag.List
	sheet := ExtractStyleSheets(doc)
	validateSheet(doc, sheet, &out)

	walkElements(doc.Root, func(n *html.Node) {
		if !css.SupportedElements[n.Tag] {
			out.Errorf("html/unsupported-element", doc.Pos(n), n.Path(), "element <%s> is not supported", n.Tag)
		}
		if style, ok := n.Attr("style"); ok {
			validateDeclarations(doc, doc.Pos(n), n.Path(), css.ParseDeclarations(style), &out)
		}
	})

	out.Sort()
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
		out.Errorf("css/syntax", doc.Position(p.Offset), "", "%s", p.Message)
	}
	validateRules(doc, sh.Rules, out)
}

func validateRules(doc *html.Document, rules []css.Rule, out *diag.List) {
	for _, r := range rules {
		switch r.Kind {
		case css.StyleRule:
			if r.Err != nil {
				out.Errorf("css/unsupported-selector", doc.Position(r.Offset), "", "selector %q: %v", r.SelectorText, r.Err)
				continue
			}
			if pseudo := r.Selector.PseudoElement(); pseudo != "" {
				validateGeneratedContent(doc, r, pseudo, out)
				continue
			}
			validateDeclarations(doc, doc.Position(r.Offset), "", r.Decls, out)
		case css.FontFaceRule:
			validateFontFace(doc, r, out)
		case css.PageRule:
			validatePageRule(doc, r, out)
		case css.PageMarginRule:
			validatePageMargin(doc, r, out)
		case css.MediaRule:
			name := strings.ToLower(strings.TrimSpace(r.Name))
			if name != "print" && name != "all" {
				out.Errorf("css/unsupported-media", doc.Position(r.Offset), "", "@media %q is not supported, only print and all", r.Name)
				continue
			}
			validateRules(doc, r.Rules, out)
		case css.UnknownAtRule:
			out.Errorf("css/unsupported-at-rule", doc.Position(r.Offset), "", "@%s is not supported", r.Name)
		}
	}
}

// validateGeneratedContent checks the single supported pseudo-element rule,
// a::after with a content property.
func validateGeneratedContent(doc *html.Document, r css.Rule, pseudo string, out *diag.List) {
	pos := doc.Position(r.Offset)
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
		declPos := doc.Position(d.Offset)
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
		declPos := doc.Position(d.Offset)
		if declPos.File == "" {
			declPos = pos
		}
		if _, ok := css.Table[d.Property]; !ok {
			out.Errorf("css/unsupported-property", declPos, elementPath, "property %q is not supported", d.Property)
			continue
		}
		st := css.DefaultStyle()
		if err := css.ApplyDecl(st, d.Property, d.Value, 12000); err != nil {
			out.Errorf("css/unsupported-value", declPos, elementPath, "%s: %v", d.Property, err)
		}
	}
}

func validateFontFace(doc *html.Document, r css.Rule, out *diag.List) {
	hasFamily, hasSrc := false, false
	for _, d := range r.Decls {
		pos := doc.Position(d.Offset)
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
		out.Errorf("css/incomplete-font-face", doc.Position(r.Offset), "", "@font-face requires font-family and src")
	}
}

func validatePageRule(doc *html.Document, r css.Rule, out *diag.List) {
	if name := strings.TrimSpace(r.Name); name != "" {
		out.Errorf("css/unsupported-page-selector", doc.Position(r.Offset), "", "@page %q selectors are not supported", name)
	}
	for _, d := range r.Decls {
		pos := doc.Position(d.Offset)
		switch d.Property {
		case "size":
			if _, err := css.ParsePageSize(d.Value); err != nil {
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
		out.Errorf("css/unsupported-margin-box", doc.Position(r.Offset), "", "margin box @%s is not supported", r.Name)
		return
	}
	for _, d := range r.Decls {
		pos := doc.Position(d.Offset)
		if d.Property != "content" {
			out.Errorf("css/unsupported-property", pos, "", "property %q is not supported inside @%s", d.Property, r.Name)
			continue
		}
		if _, err := css.ParseContent(d.Value); err != nil {
			out.Errorf("css/unsupported-value", pos, "", "content: %v", err)
		}
	}
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
