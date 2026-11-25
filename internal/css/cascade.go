package css

import (
	"cmp"
	"slices"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/html"
)

type compiledRule struct {
	sel    Selector
	spec   [3]int
	order  int
	origin int
	decls  []Declaration
	pseudo string
}

// Cascade origins. Origin is compared before specificity, so any author
// declaration beats any user-agent declaration, even a more specific one.
const (
	originUserAgent = iota
	originAuthor
)

// Cascade resolves the computed style of every element.
type Cascade struct {
	rules []compiledRule
	// guard bounds the work of matching every rule against every element and
	// carries cancellation. Nil means no limit, which is what the browser
	// importer and the document dump use.
	guard *budget.Guard
}

// SetGuard attaches the conversion's work guard to the cascade and returns it
// so construction can stay fluent. A nil guard leaves the cascade unlimited.
func (c *Cascade) SetGuard(g *budget.Guard) *Cascade {
	c.guard = g
	return c
}

// NewCascade compiles the given stylesheets as author rules in document order.
// Later sheets win when specificity and order tie.
func NewCascade(sheets ...*Stylesheet) *Cascade {
	return newCascade(nil, sheets)
}

// NewCascadeForDocument compiles the user-agent stylesheet as its own origin
// followed by the author stylesheets. An author reset such as "* { padding: 0 }"
// therefore wins over the user-agent "td { padding: 1mm }" instead of losing on
// selector specificity.
func NewCascadeForDocument(ua *Stylesheet, sheets ...*Stylesheet) *Cascade {
	return newCascade(ua, sheets)
}

func newCascade(ua *Stylesheet, sheets []*Stylesheet) *Cascade {
	c := &Cascade{}
	add := func(list []*Stylesheet, origin int) {
		for _, r := range RulesForPrint(list) {
			if r.Kind != StyleRule || r.Err != nil {
				continue
			}
			c.rules = append(c.rules, compiledRule{
				sel:    r.Selector,
				spec:   r.Selector.Specificity(),
				order:  len(c.rules),
				origin: origin,
				decls:  r.Decls,
				pseudo: r.Selector.PseudoElement(),
			})
		}
	}
	if ua != nil {
		add([]*Stylesheet{ua}, originUserAgent)
	}
	add(sheets, originAuthor)
	return c
}

type styleEntry struct {
	decl   Declaration
	spec   [3]int
	ord    int
	origin int
	pseudo bool // content of a::after, cascaded like any other declaration
}

// Compute returns the computed style of n given the parent style.
func (c *Cascade) Compute(n *html.Node, parent *Style) *Style {
	parentSize := int32(12000)
	if parent != nil {
		parentSize = parent.FontSize
	}
	st := NewStyle(parent)
	for _, e := range c.cascadeEntries(n) {
		if e.pseudo {
			if items, err := ParseContent(e.decl.Value); err == nil {
				st.AfterContent = items
			}
			continue
		}
		_ = ApplyDecl(st, e.decl.Property, e.decl.Value, parentSize)
	}

	st.resolveEm()
	return st
}

// cascadeEntries collects and orders every declaration that participates in
// the cascade of n.
func (c *Cascade) cascadeEntries(n *html.Node) []styleEntry {
	var entries []styleEntry
	for _, r := range c.rules {
		// One unit per compound keeps the charge proportional to the matcher's
		// work. An exhausted budget or a cancelled context stops the scan; the
		// engine reports the reason from the shared guard.
		if c.guard != nil {
			if err := c.guard.Add(uint64(len(r.sel.Compounds))); err != nil {
				return entries
			}
		}
		if !r.sel.Match(n) {
			continue
		}
		if r.pseudo != "" {
			// Generated content belongs to the originating element but must
			// follow the same importance, specificity and order rules as any
			// other property.
			for _, d := range r.decls {
				if d.Property != "content" {
					continue
				}
				entries = append(entries, styleEntry{decl: d, spec: r.spec, ord: r.order, origin: r.origin, pseudo: true})
			}
			continue
		}
		for _, d := range r.decls {
			entries = append(entries, styleEntry{decl: d, spec: r.spec, ord: r.order, origin: r.origin})
		}
	}
	// The style attribute participates in the cascade with a specificity above
	// any selector, and with its own !important declarations.
	if attr, ok := n.Attr("style"); ok {
		decls, _ := ParseDeclarations(attr)
		for _, d := range decls {
			entries = append(entries, styleEntry{decl: d, spec: [3]int{1 << 20, 0, 0}, ord: 1 << 30, origin: originAuthor})
		}
	}
	slices.SortStableFunc(entries, func(a, b styleEntry) int {
		// Normal declarations first, !important ones last.
		if a.decl.Important != b.decl.Important {
			if a.decl.Important {
				return 1
			}
			return -1
		}
		if a.origin != b.origin {
			if !a.decl.Important {
				// Normal declarations: user agent first, author last.
				return cmp.Compare(a.origin, b.origin)
			}
			// Important declarations: author first, user agent last, which
			// makes a user-agent !important rule win, as CSS requires.
			return cmp.Compare(b.origin, a.origin)
		}
		if a.spec != b.spec {
			return compareSpec(a.spec, b.spec)
		}
		return cmp.Compare(a.ord, b.ord)
	})
	return entries
}

// compareSpec orders two specificities component by component.
func compareSpec(a, b [3]int) int {
	for i := range a {
		if c := cmp.Compare(a[i], b[i]); c != 0 {
			return c
		}
	}
	return 0
}
