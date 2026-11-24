package css

import (
	"sort"

	"github.com/vyachin/go-pdf/internal/html"
)

type compiledRule struct {
	sel    Selector
	spec   [3]int
	order  int
	decls  []Declaration
	pseudo string
}

// Cascade resolves the computed style of every element.
type Cascade struct {
	rules []compiledRule
}

// NewCascade compiles the given stylesheets in document order. Later sheets
// win when specificity and order tie.
func NewCascade(sheets ...*Stylesheet) *Cascade {
	c := &Cascade{}
	for order, r := range RulesForPrint(sheets) {
		if r.Kind != StyleRule || r.Err != nil {
			continue
		}
		c.rules = append(c.rules, compiledRule{
			sel:    r.Selector,
			spec:   r.Selector.Specificity(),
			order:  order,
			decls:  r.Decls,
			pseudo: r.Selector.PseudoElement(),
		})
	}
	return c
}

type styleEntry struct {
	decl Declaration
	spec [3]int
	ord  int
}

// Compute returns the computed style of n given the parent style.
func (c *Cascade) Compute(n *html.Node, parent *Style) *Style {
	parentSize := int32(12000)
	if parent != nil {
		parentSize = parent.FontSize
	}
	st := NewStyle(parent)

	var entries []styleEntry
	for _, r := range c.rules {
		if !r.sel.Match(n) {
			continue
		}
		if r.pseudo != "" {
			// Generated content belongs to the originating element.
			for _, d := range r.decls {
				if d.Property != "content" {
					continue
				}
				if items, err := ParseContent(d.Value); err == nil {
					st.AfterContent = items
				}
			}
			continue
		}
		for _, d := range r.decls {
			entries = append(entries, styleEntry{decl: d, spec: r.spec, ord: r.order})
		}
	}
	// The style attribute participates in the cascade with a specificity above
	// any selector, and with its own !important declarations.
	if attr, ok := n.Attr("style"); ok {
		decls, _ := ParseDeclarations(attr)
		for _, d := range decls {
			entries = append(entries, styleEntry{decl: d, spec: [3]int{1 << 20, 0, 0}, ord: 1 << 30})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		// Normal declarations first, !important ones last.
		if a.decl.Important != b.decl.Important {
			return !a.decl.Important
		}
		if a.spec != b.spec {
			return lessSpec(a.spec, b.spec)
		}
		return a.ord < b.ord
	})
	for _, e := range entries {
		_ = ApplyDecl(st, e.decl.Property, e.decl.Value, parentSize)
	}

	st.resolveEm()
	return st
}

func lessSpec(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
