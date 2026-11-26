package css

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/diag"
)

// MaxProblems bounds the problems one stylesheet records. A stray brace costs
// one byte and one problem, so without a cap a megabyte of them would produce a
// million diagnostics and an error text of tens of megabytes. The cap is
// reported as one resource/limit and parsing stops there.
const MaxProblems = 100

// MaxAtRuleNesting bounds how deeply at-rules may nest inside one another:
// @media inside @media, a margin box inside @page, and every other nested
// at-rule. The parser rejects a stylesheet that goes one level deeper than the
// limit, before the body of the offending at-rule is parsed, so no later walk
// over the rules can recurse without a bound.
const MaxAtRuleNesting = 64

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
// It walks with an explicit stack: the parser bounds at-rule nesting, but a
// walk that recursed could still be handed a deeper tree constructed by a
// caller or a future parser change.
func RulesForPrint(sheets []*Stylesheet) []Rule {
	var out []Rule
	type frame struct {
		rules []Rule
		next  int
	}
	for _, sh := range sheets {
		if sh == nil {
			continue
		}
		stack := []frame{{rules: sh.Rules}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.next >= len(top.rules) {
				stack = stack[:len(stack)-1]
				continue
			}
			r := top.rules[top.next]
			top.next++
			if r.Kind == MediaRule {
				if MediaAppliesToPrint(r.Name) {
					stack = append(stack, frame{rules: r.Rules})
				}
				continue
			}
			out = append(out, r)
		}
	}
	return out
}

// MediaAppliesToPrint reports whether a comma separated media query list
// applies to print. It is the one media-list parser: stylesheet collection,
// rule filtering and profile validation all call it, so a list cannot be
// accepted by one of them and rejected by another.
func MediaAppliesToPrint(name string) bool {
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
	// Code is the registered diagnostic code. It is empty for a plain syntax
	// problem, which validation reports as css/syntax, and set when the parser
	// hits a resource limit.
	Code string
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

	src []byte
	// lines is the bounded line index of an external stylesheet. It is nil
	// when locate answers positions, because an inline sheet uses the HTML
	// document's index and must not build a second one.
	lines  *budget.Lines
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
	// Guard charges tokenizing and parsing to the conversion's work budget and
	// reports cancellation. Nil means unlimited work.
	Guard *budget.Guard
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
	line, start := s.lines.Resolve(offset)
	return Position{File: s.File, Line: line + 1, Col: charset.Column(s.src, start, offset)}
}

// tokenReader supplies the parser's tokens. The lexer path reads the source on
// demand, so a pathological stylesheet is never materialised as a token slice
// with one entry per input byte.
type tokenReader interface {
	peek() (Token, error)
	next() (Token, error)
}

// lexerReader adapts the streaming lexer to the parser's one-token lookahead.
type lexerReader struct {
	lex  *lexer
	tok  Token
	err  error
	have bool
}

func (r *lexerReader) peek() (Token, error) {
	if r.have {
		return r.tok, r.err
	}
	t, err := r.lex.next()
	r.tok, r.err, r.have = t, err, true
	return t, err
}

func (r *lexerReader) next() (Token, error) {
	t, err := r.peek()
	if err != nil || t.Kind == EOFToken {
		return t, err
	}
	r.have = false
	return t, nil
}

// sliceReader serves tokens that are already materialised, which is what a
// style attribute needs: it is short and ends at the end of the input.
type sliceReader struct {
	toks []Token
	pos  int
}

func (r *sliceReader) peek() (Token, error) {
	if r.pos >= len(r.toks) {
		return Token{Kind: EOFToken}, nil
	}
	return r.toks[r.pos], nil
}

func (r *sliceReader) next() (Token, error) {
	t, err := r.peek()
	if err != nil || t.Kind == EOFToken {
		return t, err
	}
	r.pos++
	return t, nil
}

type parser struct {
	reader tokenReader
	tok    Token
	have   bool
	fail   error
	failed bool
	sheet  *Stylesheet
	// depth is the current at-rule nesting level, so the parser can reject a
	// stylesheet before it builds a tree no later walk could handle.
	depth int
	// limit records that the nesting or problem limit was reported. Parsing
	// stops there: the stylesheet is rejected, and the EOF that follows must
	// not add one "unclosed block" problem per enclosing level.
	limit bool
	// problems counts the problems recorded for the sheet, so the cap is
	// enforced before the list grows with the input.
	problems int
	// declList parses a style attribute: a declaration list that ends only at
	// end of input. A brace or an at-rule is a reported problem instead of a
	// block terminator or a nested rule.
	declList bool
	// guard bounds parsing work and carries cancellation. Nil means no limit.
	guard *budget.Guard
}

// Parse parses CSS from src with positions relative to the stylesheet itself.
func Parse(src []byte) *Stylesheet { return ParseSheet(SheetOptions{}, src) }

// ParseSheet parses CSS and remembers how to map offsets to positions.
func ParseSheet(opts SheetOptions, src []byte) *Stylesheet {
	sh := &Stylesheet{File: opts.File, src: src, locate: opts.Locate}
	if opts.Locate == nil {
		// An external sheet maps its own offsets to positions, so it needs a
		// bounded index. The index is charged before it grows and stops at
		// MaxLineStarts, which rejects a sheet made of newlines instead of
		// allocating one int per line.
		lines, err := budget.NewLines(src, opts.Guard)
		sh.lines = lines
		if err == nil && lines.Capped() {
			// A stylesheet with more lines than the bounded index stores is a
			// resource limit: rejecting it avoids the hundreds of megabytes an
			// entry per input newline would allocate.
			err = fmt.Errorf("%w: the stylesheet has more than %d lines", budget.ErrExceeded, budget.MaxLineStarts)
		}
		if err != nil {
			sh.Problems = guardProblem(err)
			return sh
		}
	}
	p := &parser{
		reader: &lexerReader{lex: &lexer{src: src, base: opts.Base, guard: opts.Guard}},
		sheet:  sh,
		guard:  opts.Guard,
	}
	sh.Rules, sh.Problems = p.parseRules(false)
	if p.fail != nil {
		// The guard stopped the lexer: the rules read so far are not a
		// stylesheet, and the partial diagnostics would misdescribe the file.
		sh.Rules = nil
		sh.Problems = guardProblem(p.fail)
	}
	return sh
}

// guardProblem turns a guard failure into the one problem a stopped sheet
// records. A cancellation is left to the caller, which reports the context
// error itself.
func guardProblem(err error) []Problem {
	if errors.Is(err, budget.ErrExceeded) {
		return []Problem{{
			Code:    string(diag.ResourceLimit),
			Message: fmt.Sprintf("the stylesheet exceeds the work budget: %v", err),
		}}
	}
	var syn *syntaxError
	if errors.As(err, &syn) {
		return []Problem{{Message: syn.Msg, Offset: syn.Offset}}
	}
	return nil
}

// charge records parsing work. A nil guard is unlimited work.
func (p *parser) charge(n uint64) error { return p.guard.Add(n) }

// stop reports a guard failure as one resource/limit problem and stops the
// parse. A cancellation adds no problem: the caller reports the context error.
func (p *parser) stop(dst *[]Problem, err error) {
	p.limit = true
	if p.failed {
		return
	}
	p.failed = true
	*dst = append(*dst, guardProblem(err)...)
}

// addProblem records one problem unless the sheet has reached its cap. The cap
// is reported once as resource/limit and stops parsing.
func (p *parser) addProblem(dst *[]Problem, pr Problem) {
	if p.fail != nil && pr.Code != "resource/limit" {
		// A guard failure is reported once by stop; the problems the stopped
		// parse would have found are not evidence about the document.
		return
	}
	if p.limit {
		// The problem that stopped the parse is still recorded, because the
		// caller merges it after the limit was reported.
		if pr.Code == "resource/limit" {
			*dst = append(*dst, pr)
		}
		return
	}
	p.problems++
	if p.problems > MaxProblems {
		p.limit = true
		*dst = append(*dst, Problem{
			Code:    string(diag.ResourceLimit),
			Message: fmt.Sprintf("a stylesheet records more than %d problems", MaxProblems),
			Offset:  pr.Offset,
		})
		return
	}
	*dst = append(*dst, pr)
}

// mergeProblems records problems the parser produced in a nested block.
func (p *parser) mergeProblems(dst *[]Problem, src []Problem) {
	for _, pr := range src {
		p.addProblem(dst, pr)
	}
}

// ParseDeclarations parses a declaration list such as a style attribute with no
// work limit.
func ParseDeclarations(src string) ([]Declaration, []Problem) {
	return ParseDeclarationsWithGuard(src, nil)
}

// ParseDeclarationsWithGuard parses a declaration list and charges the guard for
// every token it consumes. A syntax error or an exhausted budget returns the one
// problem it caused; a cancellation leaves the reason on the guard for the caller
// to report.
func ParseDeclarationsWithGuard(src string, guard *budget.Guard) ([]Declaration, []Problem) {
	if err := guard.Err(); err != nil {
		return nil, guardProblem(err)
	}
	toks, err := TokenizeWithGuard([]byte(src), guard)
	if err != nil {
		return nil, guardProblem(err)
	}
	p := &parser{reader: &sliceReader{toks: toks}, guard: guard, declList: true}
	decls, _, probs := p.parseBlockContents()
	if p.fail != nil {
		return nil, guardProblem(p.fail)
	}
	return decls, probs
}

func (p *parser) peek() Token {
	if p.have {
		return p.tok
	}
	t, err := p.reader.peek()
	if err != nil {
		// A budget or context failure stops the parse: the remaining loops see
		// EOF, and stop records the reason once.
		p.fail = err
		p.tok = Token{Kind: EOFToken}
		p.have = true
		return p.tok
	}
	p.tok, p.have = t, true
	return t
}

func (p *parser) next() Token {
	t := p.peek()
	if p.fail != nil || t.Kind == EOFToken {
		return t
	}
	p.have = false
	_, _ = p.reader.next()
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
		if p.fail != nil {
			p.stop(&problems, p.fail)
			return rules, problems
		}
		if p.limit {
			return rules, problems
		}
		if err := p.charge(1); err != nil {
			p.stop(&problems, err)
			return rules, problems
		}
		t := p.peek()
		if t.Kind == EOFToken {
			if nested {
				p.addProblem(&problems, Problem{Message: "unclosed block", Offset: t.Offset})
			}
			return rules, problems
		}
		if t.Kind == RBraceToken {
			p.next()
			if !nested {
				p.addProblem(&problems, Problem{Message: "unexpected '}'", Offset: t.Offset})
				continue
			}
			return rules, problems
		}
		if t.Kind == AtKeywordToken {
			r, pr := p.parseAtRule()
			rules = append(rules, r...)
			p.mergeProblems(&problems, pr)
			continue
		}
		r, pr := p.parseQualifiedRule()
		rules = append(rules, r...)
		p.mergeProblems(&problems, pr)
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
		var probs []Problem
		p.addProblem(&probs, Problem{Message: "malformed @" + name, Offset: at.Offset})
		return nil, probs
	}
	p.next() // {
	// Reject the level that would exceed the limit before its body is parsed:
	// the returned tree then never nests deeper than MaxAtRuleNesting, so the
	// walks in this package and in profile stay bounded.
	if p.depth >= MaxAtRuleNesting {
		p.limit = true
		p.skipBlockBody()
		return nil, []Problem{{
			Code:    string(diag.ResourceLimit),
			Message: fmt.Sprintf("at-rules are nested deeper than %d levels", MaxAtRuleNesting),
			Offset:  at.Offset,
		}}
	}
	p.depth++
	defer func() { p.depth-- }()
	switch name {
	case "page":
		decls, nestedRules, probs := p.parseBlockContents()
		rules := []Rule{{Kind: PageRule, Name: stringify(prelude), Decls: decls, Offset: at.Offset, Sheet: p.sheet}}
		for _, nr := range nestedRules {
			switch nr.Kind {
			case UnknownAtRule:
				// An unknown nested at-rule inside @page is reported as an
				// unsupported margin box.
				nr.Kind = PageMarginRule
				rules = append(rules, nr)
			case PageMarginRule:
				rules = append(rules, nr)
			default:
				// @page inside @page was hoisted and applied, and @media or
				// @font-face inside a page rule cannot be represented.
				p.addProblem(&probs, Problem{
					Code:    "css/unsupported-at-rule",
					Message: fmt.Sprintf("an at-rule nested inside @page is not supported: %s", ruleName(nr)),
					Offset:  nr.Offset,
				})
			}
		}
		return rules, probs
	case "font-face":
		decls, nested, probs := p.parseBlockContents()
		for _, nr := range nested {
			p.addProblem(&probs, Problem{
				Code:    "css/unsupported-at-rule",
				Message: fmt.Sprintf("an at-rule cannot appear inside @font-face: %s", ruleName(nr)),
				Offset:  nr.Offset,
			})
		}
		return []Rule{{Kind: FontFaceRule, Decls: decls, Offset: at.Offset, Sheet: p.sheet}}, probs
	case "top-left", "top-center", "top-right", "bottom-left", "bottom-center", "bottom-right":
		decls, nested, probs := p.parseBlockContents()
		for _, nr := range nested {
			p.addProblem(&probs, Problem{
				Code:    "css/unsupported-at-rule",
				Message: fmt.Sprintf("an at-rule cannot appear inside a page margin box: %s", ruleName(nr)),
				Offset:  nr.Offset,
			})
		}
		return []Rule{{Kind: PageMarginRule, Name: name, Decls: decls, Offset: at.Offset, Sheet: p.sheet}}, probs
	case "media":
		inner, probs := p.parseRules(true)
		return []Rule{{Kind: MediaRule, Name: strings.TrimSpace(stringify(prelude)), Rules: inner, Offset: at.Offset, Sheet: p.sheet}}, probs
	default:
		p.skipBlockBody()
		return []Rule{{Kind: UnknownAtRule, Name: name, Offset: at.Offset, Sheet: p.sheet}}, nil
	}
}

// ruleName names an at-rule for a diagnostic: "@page", "@media" and so on.
func ruleName(r Rule) string {
	if r.Kind == PageRule {
		return "@page"
	}
	if r.Name != "" {
		return "@" + r.Name
	}
	return "an at-rule"
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
		var probs []Problem
		p.addProblem(&probs, Problem{Message: "malformed rule", Offset: start})
		return nil, probs
	}
	p.next()
	decls, nested, probs := p.parseBlockContents()
	for _, nr := range nested {
		p.addProblem(&probs, Problem{Message: "nested at-rules are not supported here", Offset: nr.Offset})
	}
	selText := stringifySelector(prelude)
	parts := splitTopLevelComma(selText)
	if len(parts) == 0 {
		p.addProblem(&probs, Problem{Message: "rule without a selector", Offset: start})
		return nil, probs
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
// brace (which it consumes). In declaration-list mode, used for a style
// attribute, the list ends at end of input instead: a brace is a reported
// problem that is skipped, and an at-rule is reported once and skipped whole.
func (p *parser) parseBlockContents() ([]Declaration, []Rule, []Problem) {
	var decls []Declaration
	var nested []Rule
	var probs []Problem
	for {
		p.skipTrivia()
		if p.fail != nil {
			p.stop(&probs, p.fail)
			return decls, nested, probs
		}
		if p.limit {
			return decls, nested, probs
		}
		if err := p.charge(1); err != nil {
			p.stop(&probs, err)
			return decls, nested, probs
		}
		t := p.peek()
		if t.Kind == EOFToken {
			if p.declList {
				// End of input ends the list; there is no enclosing block to
				// be unclosed.
				return decls, nested, probs
			}
			p.addProblem(&probs, Problem{Message: "unclosed block", Offset: t.Offset})
			return decls, nested, probs
		}
		if t.Kind == RBraceToken {
			p.next()
			if p.declList {
				// A style attribute has no enclosing block, so a real '}' is
				// a problem rather than an end sentinel. Skip it and keep
				// parsing so no declaration after it is lost.
				p.addProblem(&probs, Problem{Message: "unexpected '}' in a style attribute", Offset: t.Offset})
				continue
			}
			return decls, nested, probs
		}
		if p.declList && t.Kind == LBraceToken {
			p.next()
			p.addProblem(&probs, Problem{Message: "unexpected '{' in a style attribute", Offset: t.Offset})
			continue
		}
		if t.Kind == AtKeywordToken {
			if p.declList {
				// An attribute has no rule structure to attach an at-rule to,
				// so it is unsupported as a whole: report it once and skip its
				// prelude and block instead of descending into it.
				at := p.next()
				p.addProblem(&probs, Problem{
					Code:    string(diag.CSSUnsupportedAtRule),
					Message: fmt.Sprintf("%s is not supported in a style attribute", strings.ToLower(at.Text)),
					Offset:  at.Offset,
				})
				p.skipAtRule()
				continue
			}
			r, pr := p.parseAtRule()
			nested = append(nested, r...)
			p.mergeProblems(&probs, pr)
			continue
		}
		d, ok := p.parseDeclaration()
		if !ok {
			// A skipped declaration is a syntax error, not a silent success:
			// the closed profile must report it.
			p.addProblem(&probs, Problem{Message: "skipping an invalid declaration", Offset: p.peek().Offset})
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
	// A declaration is reported at its first value token, not at the
	// whitespace between the colon and the value: the whitespace is not part
	// of what the author reads as the declaration.
	p.skipTrivia()
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
		if p.declList && depth == 0 && t.Kind == LBraceToken {
			// A '{' in a style attribute is a reported problem, not part of a
			// value: ending the declaration here keeps the preceding value and
			// lets the list parser report the brace and resume after it.
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

// skipAtRule consumes an at-rule in a declaration list without descending into
// it: its prelude up to the terminating semicolon or block, and the whole block
// including nested braces. The parser reports the at-rule once; the
// declarations it wraps belong to a rule an attribute cannot express.
func (p *parser) skipAtRule() {
	for {
		t := p.peek()
		if t.Kind == EOFToken {
			return
		}
		p.next()
		switch t.Kind {
		case SemicolonToken:
			return
		case LBraceToken:
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
			return
		}
	}
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
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			// A comma or a bracket inside a string is data, not syntax.
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
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
