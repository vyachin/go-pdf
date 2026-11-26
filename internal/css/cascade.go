package css

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/diag"
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
	// diags receives a declaration that validation accepted but that cannot be
	// applied with the resolved parent font size. Nil means the caller does not
	// collect them, which is what a standalone style computation wants.
	diags *diag.List
	// reported deduplicates those diagnostics: one declaration applied to a
	// thousand elements is one problem, not a thousand.
	reported map[string]bool
	// nodePos maps an element to its source position. The cascade parses a
	// style attribute itself, so without it a declaration rejected here would
	// report :0:0 although the owning element's position is known. Nil means
	// positions are unknown, which is what a standalone style computation
	// wants.
	nodePos func(*html.Node) diag.Position
}

// SetGuard attaches the conversion's work guard to the cascade and returns it
// so construction can stay fluent. A nil guard leaves the cascade unlimited.
func (c *Cascade) SetGuard(g *budget.Guard) *Cascade {
	c.guard = g
	return c
}

// SetDiagnostics attaches the list that receives a declaration accepted by
// validation but rejected here against the resolved parent font size, and
// returns the cascade so construction can stay fluent. Without it the failure
// would be silent, which is how a paragraph was rendered at its parent's size.
func (c *Cascade) SetDiagnostics(list *diag.List) *Cascade {
	c.diags = list
	return c
}

// SetNodePosition attaches the document's position lookup to the cascade and
// returns it so construction can stay fluent. It makes an inline declaration
// rejected during style computation name the element that owns it. A nil
// lookup leaves inline positions unknown.
func (c *Cascade) SetNodePosition(lookup func(*html.Node) diag.Position) *Cascade {
	c.nodePos = lookup
	return c
}

// report records one declaration that could not be applied.
func (c *Cascade) report(d Declaration, err error) {
	if c.diags == nil {
		return
	}
	pos := d.Position()
	if pos.File == "" && d.Element != nil && c.nodePos != nil {
		pos = c.nodePos(d.Element)
	}
	if c.reported == nil {
		c.reported = map[string]bool{}
	}
	key := fmt.Sprintf("%s:%d:%d:%s", pos.File, pos.Line, pos.Col, d.Property)
	if c.reported[key] {
		return
	}
	c.reported[key] = true
	c.diags.Errorf(diag.CSSUnsupportedValue, pos, "", "%s: %v", d.Property, err)
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
	// HTML width and height attributes are presentation hints: they enter below
	// author declarations, so author CSS — including an explicit auto — wins.
	applyImageHints(st, n)
	for _, e := range c.cascadeEntries(n) {
		if e.pseudo {
			items, err := ParseContent(e.decl.Value)
			if err != nil {
				c.report(e.decl, err)
				continue
			}
			pos := e.decl.Position()
			for i := range items {
				items[i].Pos = pos
			}
			st.AfterContent = items
			continue
		}
		if err := ApplyDecl(st, e.decl.Property, e.decl.Value, parentSize); err != nil {
			// Validation checked the declaration with its default parent size;
			// here the real parent size decides. A value that rounds to zero is
			// reported instead of being dropped and rendered at the parent size.
			c.report(e.decl, err)
		}
	}

	st.resolveEm()
	st.resolveCurrentColor()
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
		decls, _ := ParseDeclarationsWithGuard(attr, c.guard)
		for _, d := range decls {
			d.Element = n
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
