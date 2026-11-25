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

// Orientation is the orientation keyword of the page size property.
type Orientation int

const (
	// OrientationAny leaves the page size as given.
	OrientationAny Orientation = iota
	// OrientationPortrait keeps the named size as is.
	OrientationPortrait
	// OrientationLandscape swaps the sides of the page.
	OrientationLandscape
)

// Named page sizes in millipoints.
var namedPageSizes = map[string]PageSize{
	"a4":     {Width: 595276, Height: 841890},
	"a5":     {Width: 419528, Height: 595276},
	"letter": {Width: 612000, Height: 792000},
}

// PageSizeSpec is the parsed value of the @page size property: a base size
// (named or explicit) and an orientation, resolved later against the fallback
// size from the run options.
type PageSizeSpec struct {
	Base        PageSize
	HasBase     bool
	Orientation Orientation
}

// Resolve returns the concrete page size, applying the orientation and the
// fallback size when the document does not name one.
func (s PageSizeSpec) Resolve(fallback PageSize) PageSize {
	size := s.Base
	if !s.HasBase {
		size = fallback
	}
	switch s.Orientation {
	case OrientationLandscape:
		if size.Width < size.Height {
			size.Width, size.Height = size.Height, size.Width
		}
	case OrientationPortrait:
		// The fallback size may be landscape; portrait must flip it, exactly as
		// landscape flips a portrait fallback.
		if size.Width > size.Height {
			size.Width, size.Height = size.Height, size.Width
		}
	}
	return size
}

// ParsePageSizeSpec parses the value of the @page size property. The grammar
// is a subset of CSS Paged Media: a page size name and an orientation in any
// order, or two lengths, or a single orientation applied to the fallback size.
func ParsePageSizeSpec(toks []Token) (PageSizeSpec, error) {
	f := filterWhitespace(toks)
	if len(f) == 0 {
		return PageSizeSpec{}, fmt.Errorf("page size is empty")
	}
	hasIdent := false
	for _, t := range f {
		if t.Kind == IdentToken {
			hasIdent = true
			break
		}
	}
	if !hasIdent {
		if len(f) != 2 {
			return PageSizeSpec{}, fmt.Errorf("page size expects two lengths")
		}
		w, err := ParseLengthToken(f[0])
		if err != nil {
			return PageSizeSpec{}, err
		}
		h, err := ParseLengthToken(f[1])
		if err != nil {
			return PageSizeSpec{}, err
		}
		if w.Kind != MP || h.Kind != MP || w.Value <= 0 || h.Value <= 0 {
			return PageSizeSpec{}, fmt.Errorf("page size expects two positive lengths")
		}
		return PageSizeSpec{Base: PageSize{Width: w.Value, Height: h.Value}, HasBase: true}, nil
	}
	if len(f) > 2 {
		return PageSizeSpec{}, fmt.Errorf("page size expects a name and an optional orientation")
	}
	var spec PageSizeSpec
	for _, t := range f {
		if t.Kind != IdentToken {
			return PageSizeSpec{}, fmt.Errorf("orientation can only be combined with a page size name, not with lengths")
		}
		name := strings.ToLower(t.Text)
		switch name {
		case "portrait", "landscape":
			if spec.Orientation != OrientationAny {
				return PageSizeSpec{}, fmt.Errorf("page size has more than one orientation")
			}
			if name == "portrait" {
				spec.Orientation = OrientationPortrait
			} else {
				spec.Orientation = OrientationLandscape
			}
		case "a4", "a5", "letter":
			if spec.HasBase {
				return PageSizeSpec{}, fmt.Errorf("page size has more than one size name")
			}
			spec.Base = namedPageSizes[name]
			spec.HasBase = true
		default:
			return PageSizeSpec{}, fmt.Errorf("unsupported page size %q", t.Text)
		}
	}
	return spec, nil
}

// ParsePageMargin parses the margin shorthand of an @page rule. A page margin is
// a non-negative length: auto and percentages have no meaning on the page box,
// and an em resolves against the initial font size (12 pt), not the page width.
func ParsePageMargin(toks []Token) ([4]Length, error) {
	v, err := parseBoxShorthand(toks)
	if err != nil {
		return [4]Length{}, err
	}
	const initialFontSize = 12000 // 12 pt in millipoints
	var out [4]Length
	for i, l := range v.Box {
		switch l.Kind {
		case MP:
			if l.Value < 0 {
				return [4]Length{}, fmt.Errorf("page margin must not be negative")
			}
			out[i] = l
		case Em:
			if l.Value < 0 {
				return [4]Length{}, fmt.Errorf("page margin must not be negative")
			}
			out[i] = Abs(l.Resolve(initialFontSize))
		case Auto:
			return [4]Length{}, fmt.Errorf("page margin must be a length, not auto")
		case Percent:
			return [4]Length{}, fmt.Errorf("page margin must be a length, not a percentage")
		default:
			return [4]Length{}, fmt.Errorf("page margin must be a length")
		}
	}
	return out, nil
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
