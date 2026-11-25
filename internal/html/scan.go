package html

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"github.com/vyachin/go-pdf/internal/budget"
)

// spanKind classifies a recorded source range.
type spanKind uint8

const (
	// spanText is character data.
	spanText spanKind = iota
	// spanStart is a start tag, including a self-closing one.
	spanStart
	// spanOther is a token the conversion never matches against the tree: an end
	// tag, a comment or a doctype. It is recorded so the ranges stay a partition
	// of the input.
	spanOther
	// spanGap is input the tokenizer could not turn into a token, such as an
	// unterminated tag at the end of the document. It keeps the recorded ranges
	// a partition of the input even for truncated documents.
	spanGap
)

// span is the byte range of one token. Offsets are int32 because the engine
// rejects a document larger than MaxInputBytes (8 MiB) before parsing.
type span struct {
	start int32
	end   int32
	kind  spanKind
	name  uint16
}

// scanResult is the token pass: every token's range in document order, plus the
// start tags indexed by name so conversion can find an element's position.
type scanResult struct {
	spans []span
	index map[string]uint16
	// startSpans holds, per interned name, the span indices of its start tags in
	// document order.
	startSpans map[uint16][]int
	// truncated is the offset where an incomplete tag begins, or -1 when the
	// input ends between tokens.
	truncated int
}

// voidElements never have children and therefore never deepen the open stack.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "keygen": true, "link": true,
	"meta": true, "param": true, "source": true, "track": true, "wbr": true,
}

// scan tokenizes src once, recording one span per token, charging one work unit
// per span, enforcing the nesting limit and reporting cancellation. It runs
// before the tree is built, so a document over a limit fails without the tree
// being allocated.
func scan(src []byte, opts ParseOptions) (*scanResult, error) {
	res := &scanResult{index: map[string]uint16{}, startSpans: map[uint16][]int{}, truncated: -1}
	z := html.NewTokenizer(bytes.NewReader(src))
	var stack []uint16
	sawHTML, sawBody := false, false
	off := int32(0)
	for {
		tt := z.Next()
		raw := z.Raw()
		start := off
		off += int32(len(raw))
		if tt == html.ErrorToken {
			if len(raw) == 0 {
				return res, nil
			}
			// Input the tokenizer could not turn into a token still costs one
			// unit, so a unit always corresponds to one recorded span.
			if err := opts.Budget.Add(1); err != nil {
				return nil, err
			}
			res.spans = append(res.spans, span{start: start, end: off, kind: spanGap})
			res.truncated = int(start)
			return res, nil
		}
		if err := opts.Budget.Add(1); err != nil {
			return nil, err
		}
		switch tt {
		case html.TextToken:
			res.spans = append(res.spans, span{start: start, end: off, kind: spanText})
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := normalTagName(string(name))
			idx := res.intern(tag)
			res.spans = append(res.spans, span{start: start, end: off, kind: spanStart, name: idx})
			res.startSpans[idx] = append(res.startSpans[idx], len(res.spans)-1)
			if tag == "html" {
				sawHTML = true
			}
			if tag == "body" {
				sawBody = true
			}
			if voidElements[tag] {
				continue
			}
			stack = append(stack, idx)
			if opts.MaxDepth > 0 && len(stack)+implicitDepth(sawHTML, sawBody) > opts.MaxDepth {
				return nil, fmt.Errorf("%w: element nesting exceeds %d levels", budget.ErrExceeded, opts.MaxDepth)
			}
		case html.EndTagToken:
			res.spans = append(res.spans, span{start: start, end: off, kind: spanOther})
			name, _ := z.TagName()
			idx, ok := res.index[normalTagName(string(name))]
			if !ok {
				continue
			}
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i] == idx {
					stack = stack[:i]
					break
				}
			}
		default:
			// Comments, doctypes and processing instructions: recorded so the
			// ranges stay a partition, never matched against the tree.
			res.spans = append(res.spans, span{start: start, end: off, kind: spanOther})
		}
	}
}

// implicitDepth is how many of html and body the tree will hold without the
// document spelling them out. The tree always has both, so the nesting limit
// counts them even when the stack does not.
func implicitDepth(sawHTML, sawBody bool) int {
	n := 0
	if !sawHTML {
		n++
	}
	if !sawBody {
		n++
	}
	return n
}

// normalTagName maps the deprecated HTML aliases the tree construction rewrites,
// so a token still matches the element it becomes. Names are compared in lower
// case because the tokenizer keeps the case of the source.
func normalTagName(name string) string {
	name = strings.ToLower(name)
	if name == "image" {
		return "img"
	}
	return name
}

// intern returns the index of name, adding it on first use.
func (s *scanResult) intern(name string) uint16 {
	if i, ok := s.index[name]; ok {
		return i
	}
	if len(s.index) > 0xffff {
		// Unreachable for a document under MaxInputBytes; a distinct name needs
		// at least two bytes of input.
		return 0
	}
	i := uint16(len(s.index))
	s.index[name] = i
	return i
}
