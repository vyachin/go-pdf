package css

import (
	"fmt"
	"strings"
)

// MarginBoxNames lists the supported page margin boxes.
var MarginBoxNames = map[string]bool{
	"top-left": true, "top-center": true, "top-right": true,
	"bottom-left": true, "bottom-center": true, "bottom-right": true,
}

// PageSize is an absolute page size in millipoints.
type PageSize struct {
	Width  int32
	Height int32
}

// Named page sizes in millipoints.
var namedPageSizes = map[string]PageSize{
	"a4":     {Width: 595276, Height: 841890},
	"a5":     {Width: 419528, Height: 595276},
	"letter": {Width: 612000, Height: 792000},
}

// ParsePageSize parses the value of the @page size property.
func ParsePageSize(toks []Token) (PageSize, error) {
	f := filterWhitespace(toks)
	if len(f) == 1 && f[0].Kind == IdentToken {
		if size, ok := namedPageSizes[strings.ToLower(f[0].Text)]; ok {
			return size, nil
		}
		return PageSize{}, fmt.Errorf("unsupported page size %q", f[0].Text)
	}
	if len(f) == 2 {
		w, err := ParseLengthToken(f[0])
		if err != nil {
			return PageSize{}, err
		}
		h, err := ParseLengthToken(f[1])
		if err != nil {
			return PageSize{}, err
		}
		if w.Kind != MP || h.Kind != MP || w.Value <= 0 || h.Value <= 0 {
			return PageSize{}, fmt.Errorf("page size expects two positive lengths")
		}
		return PageSize{Width: w.Value, Height: h.Value}, nil
	}
	return PageSize{}, fmt.Errorf("page size expects a name or two lengths")
}

// ParsePageMargin parses the margin shorthand of an @page rule.
func ParsePageMargin(toks []Token) ([4]Length, error) {
	v, err := parseBoxShorthand(toks)
	if err != nil {
		return [4]Length{}, err
	}
	return v.Box, nil
}

// ContentKind classifies a generated content item.
type ContentKind int

const (
	// ContentText is a literal string.
	ContentText ContentKind = iota
	// ContentPageNumber is counter(page).
	ContentPageNumber
	// ContentPageCount is counter(pages).
	ContentPageCount
	// ContentTarget is target-counter(attr(href), page): the page number of
	// the element referenced by an anchor.
	ContentTarget
)

// ContentItem is one piece of generated content in a margin box.
type ContentItem struct {
	Kind ContentKind
	Text string
}

// ParseContent parses the value of the content property.
func ParseContent(toks []Token) ([]ContentItem, error) {
	f := filterWhitespace(toks)
	var items []ContentItem
	for i := 0; i < len(f); {
		t := f[i]
		switch {
		case t.Kind == StringToken:
			items = append(items, ContentItem{Kind: ContentText, Text: unquote(t.Text)})
			i++
		case t.Kind == IdentToken && strings.EqualFold(t.Text, "counter"):
			if i+3 >= len(f) || f[i+1].Kind != LParenToken || f[i+3].Kind != RParenToken {
				return nil, fmt.Errorf("malformed counter() in content")
			}
			switch strings.ToLower(f[i+2].Text) {
			case "page":
				items = append(items, ContentItem{Kind: ContentPageNumber})
			case "pages":
				items = append(items, ContentItem{Kind: ContentPageCount})
			default:
				return nil, fmt.Errorf("unsupported counter %q", f[i+2].Text)
			}
			i += 4
		case t.Kind == IdentToken && strings.EqualFold(t.Text, "target-counter"):
			if i+8 >= len(f) {
				return nil, fmt.Errorf("malformed target-counter() in content")
			}
			ok := f[i+1].Kind == LParenToken &&
				f[i+2].Kind == IdentToken && strings.EqualFold(f[i+2].Text, "attr") &&
				f[i+3].Kind == LParenToken && f[i+4].Kind == IdentToken &&
				f[i+5].Kind == RParenToken && f[i+6].Kind == CommaToken &&
				f[i+7].Kind == IdentToken && strings.EqualFold(f[i+7].Text, "page") &&
				f[i+8].Kind == RParenToken
			if !ok {
				return nil, fmt.Errorf("malformed target-counter() in content")
			}
			items = append(items, ContentItem{Kind: ContentTarget, Text: strings.ToLower(f[i+4].Text)})
			i += 9
		default:
			return nil, fmt.Errorf("unsupported content value %q", t.Text)
		}
	}
	return items, nil
}
