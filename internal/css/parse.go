package css

import (
	"fmt"
	"sort"
	"strings"
)

// RuleKind classifies a parsed rule.
type RuleKind int

const (
	// StyleRule is a normal selector-and-declarations rule.
	StyleRule RuleKind = iota
	// PageRule is an @page rule.
	PageRule
	// PageMarginRule is a margin box inside @page, for example @top-center.
	PageMarginRule
	// FontFaceRule is an @font-face rule.
	FontFaceRule
	// MediaRule is an @media rule.
	MediaRule
	// UnknownAtRule is an at-rule outside the profile.
	UnknownAtRule
)

// Declaration is a single "property: value" pair.
type Declaration struct {
	Property  string
	Value     []Token
	Important bool
	Offset    int
	// Sheet is the stylesheet this declaration came from, used for positions.
	Sheet *Stylesheet
}

// Rule is a parsed CSS rule. A selector list is stored as one rule per
// selector, all sharing the same declarations.
// RulesForPrint returns the rules that apply to print, descending into @media
// blocks whose query is empty, "all" or "print". Document order is preserved.
func RulesForPrint(sheets []*Stylesheet) []Rule {
	var out []Rule
	var walk func([]Rule)
	walk = func(rules []Rule) {
		for _, r := range rules {
			if r.Kind == MediaRule {
				if mediaAppliesToPrint(r.Name) {
					walk(r.Rules)
				}
				continue
			}
			out = append(out, r)
		}
	}
	for _, sh := range sheets {
		if sh != nil {
			walk(sh.Rules)
		}
	}
	return out
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

type Rule struct {
	Kind         RuleKind
	Name         string
	Selector     Selector
	SelectorText string
	Err          error
	Decls        []Declaration
	Rules        []Rule
	Offset       int
	// Sheet is the stylesheet this rule came from, used for positions.
	Sheet *Stylesheet
}

// Problem is a problem found while parsing CSS.
type Problem struct {
	Message string
	Offset  int
}

// Position is a location inside a stylesheet.
type Position struct {
	File string
	Line int
	Col  int
}

// Stylesheet is a parsed CSS source together with enough information to map a
// token offset back to a file position.
type Stylesheet struct {
	Rules    []Rule
	Problems []Problem
	// File is the name of the stylesheet source, empty for an inline <style>.
	File string

	src    []byte
	lines  []int
	locate func(offset int) Position
}

// SheetOptions configures parsing of a stylesheet.
type SheetOptions struct {
	// File names the source for diagnostics.
	File string
	// Base is the offset of the stylesheet inside the surrounding document.
	Base int
	// Locate maps an absolute offset to a position. Inline styles use the
	// position function of the HTML document.
	Locate func(offset int) Position
}

// Position converts a token offset into a source position.
func (s *Stylesheet) Position(offset int) Position {
	if s == nil {
		return Position{}
	}
	if s.locate != nil {
		return s.locate(offset)
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(s.src) {
		offset = len(s.src)
	}
	i := sort.Search(len(s.lines), func(i int) bool { return s.lines[i] > offset }) - 1
	if i < 0 {
		i = 0
	}
	return Position{File: s.File, Line: i + 1, Col: offset - s.lines[i] + 1}
}

func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

type parser struct {
	toks  []Token
	pos   int
	sheet *Stylesheet
}

// Parse parses CSS from src with positions relative to the stylesheet itself.
func Parse(src []byte) *Stylesheet { return ParseSheet(SheetOptions{}, src) }

// ParseSheet parses CSS and remembers how to map offsets to positions.
func ParseSheet(opts SheetOptions, src []byte) *Stylesheet {
	toks := Tokenize(src)
	if opts.Base != 0 {
		for i := range toks {
			toks[i].Offset += opts.Base
		}
	}
	sh := &Stylesheet{File: opts.File, src: src, lines: lineStarts(src), locate: opts.Locate}
	p := &parser{toks: toks, sheet: sh}
	sh.Rules, sh.Problems = p.parseRules(false)
	return sh
}

// ParseDeclarations parses a declaration list such as a style attribute.
func ParseDeclarations(src string) []Declaration {
	toks := Tokenize([]byte(src))
	if len(toks) > 0 {
		toks[len(toks)-1] = Token{Kind: RBraceToken, Offset: len(src)}
	}
	p := &parser{toks: toks}
	decls, _, _ := p.parseBlockContents()
	return decls
}

func (p *parser) peek() Token {
	if p.pos >= len(p.toks) {
		return Token{Kind: EOFToken}
	}
	return p.toks[p.pos]
}

func (p *parser) next() Token {
	t := p.peek()
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) skipTrivia() {
	for p.peek().Kind == WhitespaceToken {
		p.next()
	}
}

func (p *parser) parseRules(nested bool) ([]Rule, []Problem) {
	var rules []Rule
	var problems []Problem
	for {
		p.skipTrivia()
		t := p.peek()
		if t.Kind == EOFToken {
			if nested {
				problems = append(problems, Problem{Message: "unclosed block", Offset: t.Offset})
			}
			return rules, problems
		}
		if t.Kind == RBraceToken {
			p.next()
			if !nested {
				problems = append(problems, Problem{Message: "unexpected '}'", Offset: t.Offset})
				continue
			}
			return rules, problems
		}
		if t.Kind == AtKeywordToken {
			r, pr := p.parseAtRule()
			rules = append(rules, r...)
			problems = append(problems, pr...)
			continue
		}
		r, pr := p.parseQualifiedRule()
		rules = append(rules, r...)
		problems = append(problems, pr...)
	}
}

func (p *parser) parseAtRule() ([]Rule, []Problem) {
	at := p.next()
	name := strings.ToLower(strings.TrimPrefix(at.Text, "@"))
	var prelude []Token
	for {
		t := p.peek()
		if t.Kind == EOFToken || t.Kind == SemicolonToken || t.Kind == LBraceToken {
			break
		}
		prelude = append(prelude, p.next())
	}
	if p.peek().Kind == SemicolonToken {
		p.next()
		return []Rule{{Kind: UnknownAtRule, Name: name, Offset: at.Offset, Sheet: p.sheet}}, nil
	}
	if p.peek().Kind != LBraceToken {
		return nil, []Problem{{Message: "malformed @" + name, Offset: at.Offset}}
	}
	p.next() // {
	switch name {
	case "page":
		decls, nestedRules, probs := p.parseBlockContents()
		rules := []Rule{{Kind: PageRule, Name: stringify(prelude), Decls: decls, Offset: at.Offset, Sheet: p.sheet}}
		for _, nr := range nestedRules {
			// An unknown nested at-rule inside @page is reported as an
			// unsupported margin box.
			if nr.Kind == UnknownAtRule {
				nr.Kind = PageMarginRule
			}
			rules = append(rules, nr)
		}
		return rules, probs
	case "font-face":
		decls, _, probs := p.parseBlockContents()
		return []Rule{{Kind: FontFaceRule, Decls: decls, Offset: at.Offset, Sheet: p.sheet}}, probs
	case "top-left", "top-center", "top-right", "bottom-left", "bottom-center", "bottom-right":
		decls, _, probs := p.parseBlockContents()
		return []Rule{{Kind: PageMarginRule, Name: name, Decls: decls, Offset: at.Offset, Sheet: p.sheet}}, probs
	case "media":
		inner, probs := p.parseRules(true)
		return []Rule{{Kind: MediaRule, Name: strings.TrimSpace(stringify(prelude)), Rules: inner, Offset: at.Offset, Sheet: p.sheet}}, probs
	default:
		p.skipBlockBody()
		return []Rule{{Kind: UnknownAtRule, Name: name, Offset: at.Offset, Sheet: p.sheet}}, nil
	}
}

// skipBlockBody consumes a block whose opening brace has already been read.
func (p *parser) skipBlockBody() {
	depth := 1
	for depth > 0 {
		t := p.next()
		switch t.Kind {
		case EOFToken:
			return
		case LBraceToken:
			depth++
		case RBraceToken:
			depth--
		}
	}
}

func (p *parser) parseQualifiedRule() ([]Rule, []Problem) {
	start := p.peek().Offset
	var prelude []Token
	for {
		t := p.peek()
		if t.Kind == EOFToken || t.Kind == LBraceToken {
			break
		}
		prelude = append(prelude, p.next())
	}
	if p.peek().Kind != LBraceToken {
		p.skipBlock()
		return nil, []Problem{{Message: "malformed rule", Offset: start}}
	}
	p.next()
	decls, nested, probs := p.parseBlockContents()
	for _, nr := range nested {
		probs = append(probs, Problem{Message: "nested at-rules are not supported here", Offset: nr.Offset})
	}
	selText := stringify(prelude)
	parts := splitTopLevelComma(selText)
	if len(parts) == 0 {
		return nil, append(probs, Problem{Message: "rule without a selector", Offset: start})
	}
	var rules []Rule
	for _, part := range parts {
		sel, err := ParseSelector(part)
		rules = append(rules, Rule{
			Kind:         StyleRule,
			Selector:     sel,
			SelectorText: part,
			Err:          err,
			Decls:        decls,
			Offset:       start,
			Sheet:        p.sheet,
		})
	}
	return rules, probs
}

// parseBlockContents reads declarations and nested at-rules until the closing
// brace (which it consumes).
func (p *parser) parseBlockContents() ([]Declaration, []Rule, []Problem) {
	var decls []Declaration
	var nested []Rule
	var probs []Problem
	for {
		p.skipTrivia()
		t := p.peek()
		if t.Kind == EOFToken {
			probs = append(probs, Problem{Message: "unclosed block", Offset: t.Offset})
			return decls, nested, probs
		}
		if t.Kind == RBraceToken {
			p.next()
			return decls, nested, probs
		}
		if t.Kind == AtKeywordToken {
			r, pr := p.parseAtRule()
			nested = append(nested, r...)
			probs = append(probs, pr...)
			continue
		}
		d, ok := p.parseDeclaration()
		if !ok {
			p.skipToDeclarationEnd()
			continue
		}
		decls = append(decls, d)
	}
}

func (p *parser) parseDeclaration() (Declaration, bool) {
	p.skipTrivia()
	t := p.peek()
	if t.Kind != IdentToken {
		return Declaration{}, false
	}
	prop := strings.ToLower(t.Text)
	p.next()
	p.skipTrivia()
	if p.peek().Kind != ColonToken {
		return Declaration{}, false
	}
	p.next()
	start := p.peek().Offset
	var value []Token
	depth := 0
	for {
		t := p.peek()
		if t.Kind == EOFToken {
			break
		}
		if depth == 0 && (t.Kind == SemicolonToken || t.Kind == RBraceToken) {
			break
		}
		switch t.Kind {
		case LParenToken, LBracketToken:
			depth++
		case RParenToken, RBracketToken:
			if depth > 0 {
				depth--
			}
		}
		value = append(value, p.next())
	}
	if p.peek().Kind == SemicolonToken {
		p.next()
	}
	important := false
	if idx := lastNonSpace(value); idx >= 1 {
		if value[idx].Kind == IdentToken && strings.EqualFold(value[idx].Text, "important") {
			if prev := lastNonSpace(value[:idx]); prev >= 0 && value[prev].Kind == DelimToken && value[prev].Text == "!" {
				important = true
				value = value[:prev]
			}
		}
	}
	return Declaration{Property: prop, Value: value, Important: important, Offset: start, Sheet: p.sheet}, true
}

func lastNonSpace(toks []Token) int {
	for i := len(toks) - 1; i >= 0; i-- {
		if toks[i].Kind != WhitespaceToken {
			return i
		}
	}
	return -1
}

func (p *parser) skipToDeclarationEnd() {
	depth := 0
	for {
		t := p.peek()
		switch t.Kind {
		case EOFToken:
			return
		case SemicolonToken:
			if depth == 0 {
				p.next()
				return
			}
			p.next()
		case RBraceToken:
			if depth == 0 {
				return
			}
			depth--
			p.next()
		case LBraceToken:
			depth++
			p.next()
		default:
			p.next()
		}
	}
}

func (p *parser) skipBlock() {
	for p.peek().Kind != LBraceToken && p.peek().Kind != EOFToken {
		p.next()
	}
	if p.peek().Kind == EOFToken {
		return
	}
	p.next()
	depth := 1
	for depth > 0 {
		t := p.next()
		switch t.Kind {
		case EOFToken:
			return
		case LBraceToken:
			depth++
		case RBraceToken:
			depth--
		}
	}
}

// splitTopLevelComma splits a selector list on commas that are not nested in
// parentheses or brackets.
func splitTopLevelComma(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// String renders a declaration for messages.
func (d Declaration) String() string {
	return fmt.Sprintf("%s: %s", d.Property, stringify(d.Value))
}
