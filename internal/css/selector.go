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
	// Adjacent is the "+" combinator: the element immediately after the
	// previous compound among its siblings.
	Adjacent
)

// Compound is a simple selector: tag, id and classes.
type Compound struct {
	Tag       string
	ID        string
	Classes   []string
	Universal bool
	// PseudoElement is set for the single supported pseudo-element, "after".
	PseudoElement string
	// Root marks the :root pseudo-class, which matches the document element.
	Root bool
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
		if c.Root {
			sp[1]++
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
func (s Selector) Match(n *html.Node) bool { return s.match(n, nil) }

// matcher counts compound comparisons, so a test can pin the bounded cost of
// matching with a step bound instead of a wall-clock threshold. The methods are
// nil-safe: production matching passes nil and pays only a nil check.
type matcher struct{ steps int }

func (m *matcher) matches(c Compound, n *html.Node) bool {
	if m != nil {
		m.steps++
	}
	return c.Match(n)
}

// match is Match with an optional step counter.
func (s Selector) match(n *html.Node, m *matcher) bool {
	if n == nil || n.Type != html.ElementNode || len(s.Compounds) == 0 {
		return false
	}
	// One and two compounds are a single bounded walk. Longer selectors are
	// split at every adjacent combinator: a descendant/child segment is matched
	// by the ancestor-chain dynamic program, and "+" is a hop to the previous
	// element. Without the split, the recursive matcher re-walks the ancestor
	// chain at every step, so a deep tree takes combinatorial time.
	if len(s.Compounds) <= 2 {
		return matchFromM(s, len(s.Compounds)-1, n, m)
	}
	starts := s.segmentStarts()
	if len(starts) == 1 {
		return s.matchSegment(0, len(s.Compounds)-1, n, m, nil)
	}
	return s.matchAdjacent(n, m)
}

func matchFrom(s Selector, idx int, n *html.Node) bool { return matchFromM(s, idx, n, nil) }

// matchFromM is the reference right-to-left matcher. It is the oracle the
// differential test compares against, and the bounded matcher reuses its
// result for one- and two-compound selectors.
func matchFromM(s Selector, idx int, n *html.Node, m *matcher) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	if !m.matches(s.Compounds[idx], n) {
		return false
	}
	if idx == 0 {
		return true
	}
	switch s.Combinators[idx-1] {
	case Child:
		return matchFromM(s, idx-1, parentElement(n), m)
	case Adjacent:
		return matchFromM(s, idx-1, previousElement(n), m)
	default:
		for p := parentElement(n); p != nil; p = parentElement(p) {
			if matchFromM(s, idx-1, p, m) {
				return true
			}
		}
		return false
	}
}

// segmentStarts returns the first compound index of every segment between
// adjacent combinators. The combinators inside one segment are only descendant
// and child, which the dynamic program handles in one pass.
func (s Selector) segmentStarts() []int {
	starts := []int{0}
	for i, c := range s.Combinators {
		if c == Adjacent {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// matchChainSize is the ancestor depth matched without allocating. The parser
// bounds DOM depth, so most documents stay on the stack path; deeper chains
// fall back to slices.
const matchChainSize = 64

// matchSegment matches compounds[lo..hi], whose combinators are descendant or
// child, with compounds[hi] at right. It reports whether the segment matches
// and, when starts is non-nil, appends every node where compounds[lo] matched.
// The work is O((hi-lo+1) × depth), independent of the number of ways the
// compounds could be distributed over the ancestors.
func (s Selector) matchSegment(lo, hi int, right *html.Node, m *matcher, starts *[]*html.Node) bool {
	var chainBuf [matchChainSize]*html.Node
	chain := chainBuf[:0]
	for p := right; p != nil; p = parentElement(p) {
		chain = append(chain, p)
	}
	depth := len(chain)
	var prevBuf, nextBuf [matchChainSize]bool
	var lowBuf [matchChainSize + 1]bool
	var prev, next, low []bool
	if depth <= matchChainSize {
		prev, next, low = prevBuf[:depth], nextBuf[:depth], lowBuf[:depth+1]
	} else {
		prev, next = make([]bool, depth), make([]bool, depth)
		low = make([]bool, depth+1)
	}
	// The rightmost compound of the segment must be right itself, so only the
	// first ancestor position starts as a candidate.
	prev[0] = m.matches(s.Compounds[hi], chain[0])
	for idx := hi - 1; idx >= lo; idx-- {
		// low[j] reports whether compounds[idx+1] matched at one of the j
		// elements below chain[j].
		low[0] = false
		for j := 0; j < depth; j++ {
			low[j+1] = low[j] || prev[j]
		}
		child := s.Combinators[idx] == Child
		for j := 0; j < depth; j++ {
			if !m.matches(s.Compounds[idx], chain[j]) {
				next[j] = false
				continue
			}
			if child {
				next[j] = j >= 1 && prev[j-1]
			} else {
				next[j] = low[j]
			}
		}
		prev, next = next, prev
	}
	matched := false
	for j := 0; j < depth; j++ {
		if !prev[j] {
			continue
		}
		matched = true
		if starts != nil {
			*starts = append(*starts, chain[j])
		}
	}
	return matched
}

// segmentKey identifies one subproblem of matchAdjacent.
type segmentKey struct {
	segment int
	right   *html.Node
}

// matchAdjacent matches a selector that uses "+". Segments are matched from the
// right; every element the leftmost compound of a segment can occupy is tried
// as the anchor whose previous sibling must end the segment to its left.
func (s Selector) matchAdjacent(n *html.Node, m *matcher) bool {
	starts := s.segmentStarts()
	memo := make(map[segmentKey]bool, len(starts)*2)
	var rec func(k int, right *html.Node) bool
	rec = func(k int, right *html.Node) bool {
		key := segmentKey{segment: k, right: right}
		if v, ok := memo[key]; ok {
			return v
		}
		lo := starts[k]
		hi := len(s.Compounds) - 1
		if k+1 < len(starts) {
			hi = starts[k+1] - 1
		}
		var anchors []*html.Node
		res := false
		if s.matchSegment(lo, hi, right, m, &anchors) {
			if k == 0 {
				res = true
			} else {
				for _, anchor := range anchors {
					if prev := previousElement(anchor); prev != nil && rec(k-1, prev) {
						res = true
						break
					}
				}
			}
		}
		memo[key] = res
		return res
	}
	return rec(len(starts)-1, n)
}

// previousElement returns the sibling immediately before n, skipping text.
func previousElement(n *html.Node) *html.Node {
	if n == nil || n.Parent == nil {
		return nil
	}
	var prev *html.Node
	for _, c := range n.Parent.Children {
		if c == n {
			return prev
		}
		if c.Type == html.ElementNode {
			prev = c
		}
	}
	return nil
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
	if c.Root && parentElement(n) != nil {
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
		if s[i] == '>' || s[i] == '+' {
			if expectCompound {
				return Selector{}, fmt.Errorf("unexpected %q in selector %q", string(s[i]), s)
			}
			if s[i] == '>' {
				pending = Child
			} else {
				pending = Adjacent
			}
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
		case ' ', '>', '+', ',':
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
			name, n := readIdent(s[i+1:])
			if strings.EqualFold(name, "root") {
				c.Root = true
				started = true
				i += 1 + n
				continue
			}
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
