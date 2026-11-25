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
	// name is the interned tag name for spanStart, and the decoded text's id
	// plus one for spanText. Zero on a text span means the token's bytes are
	// already its decoded text.
	name int32
}

// scanResult is the token pass: every token's range in document order, plus the
// start tags indexed by name so conversion can find an element's position.
type scanResult struct {
	spans []span
	index map[string]int32
	// startSpans holds, per interned name, the span indices of its start tags in
	// document order.
	startSpans map[int32][]int
	// truncated is the offset where an incomplete tag begins, or -1 when the
	// input ends between tokens.
	truncated int
	// decoded holds the text of the text tokens whose decoded bytes differ from
	// the source, interned through decodedIndex; a text span refers to an entry
	// by name-1. A token that decodes to itself stores nothing.
	decoded      []string
	decodedIndex map[string]int32
}

// neverImplicitlyClosed are elements a following start tag cannot close
// implicitly in the standard tree: a run of them is a lower bound on nesting.
var neverImplicitlyClosed = map[string]bool{
	"div": true, "ul": true, "ol": true, "span": true,
	"b": true, "strong": true, "em": true, "i": true,
}

// earlyDepthOff are constructs whose implied elements or foreign content make
// the token-stream run an unreliable lower bound, so the early rule gives up.
var earlyDepthOff = map[string]bool{
	"table": true, "tbody": true, "thead": true, "tfoot": true, "tr": true,
	"td": true, "th": true, "svg": true, "math": true, "select": true,
	"template": true,
}

// scan tokenizes src once, recording one span per token, charging one work unit
// per span, enforcing the nesting limit and reporting cancellation. It runs
// before the tree is built, so a document over a limit fails without the tree
// being allocated.
func scan(src []byte, opts ParseOptions) (*scanResult, error) {
	res := &scanResult{
		index:        map[string]int32{},
		startSpans:   map[int32][]int{},
		decodedIndex: map[string]int32{},
		truncated:    -1,
	}
	z := html.NewTokenizer(bytes.NewReader(src))
	// earlyRun counts the current run of never-implicitly-closed start tags.
	// It is a lower bound on the tree depth, so exceeding MaxDepth proves the
	// document over the limit without building the tree; it never over-rejects.
	earlyRun := 0
	earlyDepth := true
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
			sp := span{start: start, end: off, kind: spanText}
			// Only a trigger byte can make the decoded text differ from the
			// source: an entity, a carriage return or a NUL in raw text. A
			// raw-text token keeps its ampersand, so the tokenizer returns the
			// source unchanged and no entry is stored.
			if bytes.IndexAny(src[start:off], "&\r\x00") >= 0 {
				if decoded := z.Text(); !bytes.Equal(decoded, src[start:off]) {
					sp.name = res.internText(string(decoded)) + 1
				}
			}
			res.spans = append(res.spans, sp)
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := normalTagName(string(name))
			idx := res.intern(tag)
			res.spans = append(res.spans, span{start: start, end: off, kind: spanStart, name: idx})
			res.startSpans[idx] = append(res.startSpans[idx], len(res.spans)-1)
			if earlyDepth {
				switch {
				case earlyDepthOff[tag]:
					earlyDepth = false
				case neverImplicitlyClosed[tag]:
					earlyRun++
					if opts.MaxDepth > 0 && earlyRun > opts.MaxDepth {
						return nil, fmt.Errorf("%w: element nesting exceeds %d levels", budget.ErrExceeded, opts.MaxDepth)
					}
				}
			}
		case html.EndTagToken:
			res.spans = append(res.spans, span{start: start, end: off, kind: spanOther})
			// Any end tag can close one of the open elements, so the proven run
			// starts again from zero.
			earlyRun = 0
		default:
			// Comments, doctypes and processing instructions: recorded so the
			// ranges stay a partition, never matched against the tree.
			res.spans = append(res.spans, span{start: start, end: off, kind: spanOther})
		}
	}
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

// intern returns the index of name, adding it on first use. The table grows
// with the document: a flat document may use more than 65,536 distinct names,
// and a shared zero index would append a start tag to the wrong span.
func (s *scanResult) intern(name string) int32 {
	if i, ok := s.index[name]; ok {
		return i
	}
	i := int32(len(s.index))
	s.index[name] = i
	return i
}

// internText returns the id of a decoded text, adding it on first use. Repeated
// references such as &amp; share one entry.
func (s *scanResult) internText(text string) int32 {
	if i, ok := s.decodedIndex[text]; ok {
		return i
	}
	i := int32(len(s.decoded))
	s.decoded = append(s.decoded, text)
	s.decodedIndex[text] = i
	return i
}

// textOf returns the decoded text of a text span. A span whose bytes decode to
// themselves has no entry, so its raw slice is its decoded text.
func (s *scanResult) textOf(src []byte, sp span) string {
	if sp.name > 0 {
		return s.decoded[sp.name-1]
	}
	return string(src[sp.start:sp.end])
}
