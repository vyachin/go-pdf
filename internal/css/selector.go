package css

import (
	"fmt"
	"strings"

	"github.com/vyachin/go-pdf/internal/html"
)

// Combinator connects two compound selectors.
type Combinator int

const (
	// Descendant is the whitespace combinator.
	Descendant Combinator = iota
	// Child is the ">" combinator.
	Child
)

// Compound is a simple selector: tag, id and classes.
type Compound struct {
	Tag       string
	ID        string
	Classes   []string
	Universal bool
	// PseudoElement is set for the single supported pseudo-element, "after".
	PseudoElement string
}

// Selector is a parsed complex selector.
type Selector struct {
	Compounds   []Compound
	Combinators []Combinator
	Text        string
}

// Specificity returns the (ids, classes, tags) triple used by the cascade.
func (s Selector) Specificity() [3]int {
	var sp [3]int
	for _, c := range s.Compounds {
		if c.ID != "" {
			sp[0]++
		}
		sp[1] += len(c.Classes)
		if c.Tag != "" {
			sp[2]++
		}
		if c.PseudoElement != "" {
			sp[2]++
		}
	}
	return sp
}

func (s Selector) String() string { return s.Text }

// PseudoElement returns the pseudo-element of the last compound, if any.
func (s Selector) PseudoElement() string {
	if len(s.Compounds) == 0 {
		return ""
	}
	return s.Compounds[len(s.Compounds)-1].PseudoElement
}

// Match reports whether the selector matches an element node.
func (s Selector) Match(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode || len(s.Compounds) == 0 {
		return false
	}
	// One and two compounds are a single ancestor walk. Longer selectors are
	// matched by dynamic programming over the ancestor chain: the naive
	// recursive matcher re-walks the same ancestors for every combination of
	// positions, so a deep tree and a long descendant selector take
	// combinatorial time even for a document of a few hundred bytes.
	if len(s.Compounds) <= 2 {
		return matchFrom(s, len(s.Compounds)-1, n)
	}
	return s.matchAncestors(n)
}

func matchFrom(s Selector, idx int, n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	if !s.Compounds[idx].Match(n) {
		return false
	}
	if idx == 0 {
		return true
	}
	switch s.Combinators[idx-1] {
	case Child:
		return matchFrom(s, idx-1, parentElement(n))
	default:
		for p := parentElement(n); p != nil; p = parentElement(p) {
			if matchFrom(s, idx-1, p) {
				return true
			}
		}
		return false
	}
}

// matchChainSize is the ancestor depth matchAncestors handles without
// allocating. The parser bounds DOM depth, so most documents stay on the stack
// path; deeper chains fall back to slices.
const matchChainSize = 64

// matchAncestors matches a selector with three or more compounds in
// O(compounds × depth): prev[j] reports whether the compounds before the
// current one match with the previous compound on the j-th ancestor of n,
// counting n as ancestor 0.
func (s Selector) matchAncestors(n *html.Node) bool {
	var chainBuf [matchChainSize]*html.Node
	chain := chainBuf[:0]
	for p := n; p != nil; p = parentElement(p) {
		chain = append(chain, p)
	}
	depth := len(chain)

	var prevBuf, curBuf [matchChainSize]bool
	var suffixBuf [matchChainSize + 1]bool
	var prev, cur, suffix []bool
	if depth <= matchChainSize {
		prev, cur, suffix = prevBuf[:depth], curBuf[:depth], suffixBuf[:depth+1]
	} else {
		prev, cur = make([]bool, depth), make([]bool, depth)
		suffix = make([]bool, depth+1)
	}
	for j := 0; j < depth; j++ {
		prev[j] = s.Compounds[0].Match(chain[j])
	}
	for idx := 1; idx < len(s.Compounds); idx++ {
		child := s.Combinators[idx-1] == Child
		suffix[depth] = false
		for j := depth - 1; j >= 0; j-- {
			suffix[j] = prev[j] || suffix[j+1]
		}
		for j := 0; j < depth; j++ {
			if !s.Compounds[idx].Match(chain[j]) {
				cur[j] = false
				continue
			}
			if child {
				cur[j] = j+1 < depth && prev[j+1]
			} else {
				cur[j] = suffix[j+1]
			}
		}
		prev, cur = cur, prev
	}
	return prev[0]
}

func parentElement(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode {
			return p
		}
	}
	return nil
}

// Match reports whether the compound selector matches an element.
func (c Compound) Match(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	if c.Tag != "" && c.Tag != n.Tag {
		return false
	}
	if c.ID != "" && c.ID != n.ID() {
		return false
	}
	for _, cl := range c.Classes {
		if !n.HasClass(cl) {
			return false
		}
	}
	return true
}

// ParseSelector parses a CSS selector. Unsupported syntax (pseudo classes,
// pseudo elements, attribute selectors) is rejected with a descriptive error,
// because the profile is a closed white list.
func ParseSelector(text string) (Selector, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return Selector{}, fmt.Errorf("empty selector")
	}
	sel := Selector{Text: s}
	i := 0
	expectCompound := true
	pending := Descendant
	pendingSet := false
	sawSpace := false
	for i < len(s) {
		sp := false
		for i < len(s) && s[i] == ' ' {
			i++
			sp = true
		}
		if sp {
			sawSpace = true
		}
		if i >= len(s) {
			break
		}
		if s[i] == '>' {
			if expectCompound {
				return Selector{}, fmt.Errorf("unexpected %q in selector %q", ">", s)
			}
			pending = Child
			pendingSet = true
			expectCompound = true
			i++
			continue
		}
		if !expectCompound {
			if !sawSpace {
				return Selector{}, fmt.Errorf("missing combinator in selector %q", s)
			}
			pending = Descendant
			pendingSet = true
		}
		compound, n, err := parseCompound(s[i:])
		if err != nil {
			return Selector{}, err
		}
		if n == 0 {
			return Selector{}, fmt.Errorf("unexpected %q in selector %q", string(s[i]), s)
		}
		i += n
		if len(sel.Compounds) > 0 {
			if !pendingSet {
				pending = Descendant
			}
			sel.Combinators = append(sel.Combinators, pending)
		}
		sel.Compounds = append(sel.Compounds, compound)
		expectCompound = false
		pending = Descendant
		pendingSet = false
		sawSpace = false
	}
	if expectCompound || len(sel.Compounds) == 0 {
		return Selector{}, fmt.Errorf("incomplete selector %q", s)
	}
	return sel, nil
}

func parseCompound(s string) (Compound, int, error) {
	var c Compound
	i := 0
	started := false
	for i < len(s) {
		ch := s[i]
		switch ch {
		case ' ', '>', ',':
			if !started {
				return Compound{}, 0, fmt.Errorf("empty compound selector")
			}
			return c, i, nil
		case '*':
			if started {
				return Compound{}, 0, fmt.Errorf("unexpected %q", "*")
			}
			c.Universal = true
			started = true
			i++
		case '.':
			name, n := readIdent(s[i+1:])
			if n == 0 {
				return Compound{}, 0, fmt.Errorf("class selector without a name")
			}
			c.Classes = append(c.Classes, name)
			i += 1 + n
			started = true
		case '#':
			name, n := readIdent(s[i+1:])
			if n == 0 {
				return Compound{}, 0, fmt.Errorf("id selector without a name")
			}
			c.ID = name
			i += 1 + n
			started = true
		case ':':
			if strings.HasPrefix(s[i:], "::") {
				name, n := readIdent(s[i+2:])
				if n == 0 {
					return Compound{}, 0, fmt.Errorf("pseudo-element without a name")
				}
				if c.PseudoElement != "" {
					return Compound{}, 0, fmt.Errorf("more than one pseudo-element in a selector")
				}
				c.PseudoElement = strings.ToLower(name)
				i += 2 + n
				started = true
				// A pseudo-element must end the compound selector.
				if i < len(s) && s[i] != ' ' && s[i] != '>' && s[i] != ',' {
					return Compound{}, 0, fmt.Errorf("unexpected %q after a pseudo-element", string(s[i]))
				}
				continue
			}
			name, _ := readIdent(s[i+1:])
			return Compound{}, 0, fmt.Errorf("pseudo-class %q is not supported", ":"+name)
		case '[':
			return Compound{}, 0, fmt.Errorf("attribute selectors are not supported")
		default:
			if !isNameStart(ch) && ch != '-' {
				return Compound{}, 0, fmt.Errorf("unexpected %q in selector", string(ch))
			}
			if started {
				return Compound{}, 0, fmt.Errorf("unexpected %q after a compound selector", string(ch))
			}
			name, n := readIdent(s[i:])
			if n == 0 {
				return Compound{}, 0, fmt.Errorf("empty tag name")
			}
			c.Tag = strings.ToLower(name)
			i += n
			started = true
		}
	}
	if !started {
		return Compound{}, 0, fmt.Errorf("empty compound selector")
	}
	return c, i, nil
}

func readIdent(s string) (string, int) {
	i := 0
	for i < len(s) && isNameChar(s[i]) {
		i++
	}
	return s[:i], i
}
