package css

import (
	"fmt"
	"math"
	"strings"
)

// LengthKind distinguishes the ways a length can be expressed.
type LengthKind int

const (
	// Auto is the "auto" keyword.
	Auto LengthKind = iota
	// MP is an absolute length stored in thousandths of a point.
	MP
	// Percent is relative to the containing block, stored in thousandths of a
	// percent (100000 == 100%).
	Percent
	// Em is relative to the element font size, stored in thousandths of a
	// percent of the font size (100000 == 1em).
	Em
)

// Length is a CSS length value.
type Length struct {
	Kind  LengthKind
	Value int32
}

// Abs builds an absolute length from millipoints.
func Abs(mp int32) Length { return Length{Kind: MP, Value: mp} }

// Pct builds a percentage length, for example Pct(50) is 50%.
func Pct(v float64) Length { return Length{Kind: Percent, Value: int32(math.Round(v * 1000))} }

// EmLen builds an em length, for example EmLen(1.5) is 1.5em.
func EmLen(v float64) Length { return Length{Kind: Em, Value: int32(math.Round(v * 100000))} }

// IsAuto reports whether the length is "auto".
func (l Length) IsAuto() bool { return l.Kind == Auto }

// Resolve returns the absolute value for a resolved basis. Em and Percent are
// resolved against basis; absolute values are returned unchanged.
func (l Length) Resolve(basis int32) int32 {
	switch l.Kind {
	case MP:
		return l.Value
	case Percent, Em:
		return int32(int64(basis) * int64(l.Value) / 100000)
	default:
		return 0
	}
}

// ResolveFont converts an em length into an absolute length using the element
// font size. Other kinds are returned unchanged.
func (l Length) ResolveFont(fontSize int32) Length {
	if l.Kind == Em {
		return Abs(l.Resolve(fontSize))
	}
	return l
}

func (l Length) String() string {
	switch l.Kind {
	case Auto:
		return "auto"
	case MP:
		return fmt.Sprintf("%gpt", float64(l.Value)/1000)
	case Percent:
		return fmt.Sprintf("%g%%", float64(l.Value)/1000)
	case Em:
		return fmt.Sprintf("%gem", float64(l.Value)/100000)
	}
	return "0"
}

var mpPerUnit = map[string]float64{
	"pt": 1000,
	"px": 750,
	"mm": 72000 / 25.4,
	"cm": 720000 / 25.4,
	"in": 72000,
}

// ParseLengthToken parses a single length token.
func ParseLengthToken(t Token) (Length, error) {
	switch t.Kind {
	case DimensionToken:
		if v, ok := mpPerUnit[t.Unit]; ok {
			return Abs(int32(math.Round(t.Num * v))), nil
		}
		if t.Unit == "em" {
			return EmLen(t.Num), nil
		}
		if t.Unit == "rem" {
			return Length{}, fmt.Errorf("unit %q is not supported", t.Unit)
		}
		return Length{}, fmt.Errorf("unit %q is not supported", t.Unit)
	case PercentageToken:
		return Pct(t.Num), nil
	case NumberToken:
		if t.Num == 0 {
			return Abs(0), nil
		}
		return Length{}, fmt.Errorf("length %q needs a unit", t.Text)
	case IdentToken:
		if strings.EqualFold(t.Text, "auto") {
			return Length{Kind: Auto}, nil
		}
		return Length{}, fmt.Errorf("expected a length, got %q", t.Text)
	default:
		return Length{}, fmt.Errorf("expected a length, got %q", t.Text)
	}
}

// ParseLengthString parses a single length such as "20mm".
func ParseLengthString(s string) (Length, error) {
	f := filterWhitespace(Tokenize([]byte(s)))
	if len(f) != 1 {
		return Length{}, fmt.Errorf("expected a single length, got %q", s)
	}
	return ParseLengthToken(f[0])
}

// Color is a parsed colour. Set is false for "transparent" and for an absent
// value.
type Color struct {
	R, G, B uint8
	Set     bool
}

var namedColors = map[string][3]uint8{
	"black": {0, 0, 0}, "white": {255, 255, 255}, "red": {255, 0, 0},
	"green": {0, 128, 0}, "blue": {0, 0, 255}, "yellow": {255, 255, 0},
	"gray": {128, 128, 128}, "grey": {128, 128, 128}, "silver": {192, 192, 192},
	"maroon": {128, 0, 0}, "navy": {0, 0, 128}, "teal": {0, 128, 128},
	"purple": {128, 0, 128}, "orange": {255, 165, 0}, "olive": {128, 128, 0},
	"lime": {0, 255, 0}, "aqua": {0, 255, 255}, "fuchsia": {255, 0, 255},
	"darkgray": {169, 169, 169}, "darkgrey": {169, 169, 169},
	"lightgray": {211, 211, 211}, "lightgrey": {211, 211, 211},
}

func hexNibble(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// ParseColor parses a colour token sequence.
func ParseColor(toks []Token) (Color, error) {
	filtered := filterWhitespace(toks)
	if len(filtered) == 1 {
		t := filtered[0]
		if t.Kind == IdentToken {
			name := strings.ToLower(t.Text)
			if name == "transparent" {
				return Color{}, nil
			}
			if c, ok := namedColors[name]; ok {
				return Color{R: c[0], G: c[1], B: c[2], Set: true}, nil
			}
			return Color{}, fmt.Errorf("unknown colour %q", t.Text)
		}
		if t.Kind == HashToken {
			hex := strings.TrimPrefix(t.Text, "#")
			switch len(hex) {
			case 3:
				r, ok1 := hexNibble(hex[0])
				g, ok2 := hexNibble(hex[1])
				b, ok3 := hexNibble(hex[2])
				if ok1 && ok2 && ok3 {
					return Color{R: r*17, G: g*17, B: b*17, Set: true}, nil
				}
			case 6:
				parse := func(s string) (uint8, bool) {
					var v uint8
					for i := 0; i < len(s); i++ {
						n, ok := hexNibble(s[i])
						if !ok {
							return 0, false
						}
						v = v*16 + n
					}
					return v, true
				}
				r, ok1 := parse(hex[0:2])
				g, ok2 := parse(hex[2:4])
				b, ok3 := parse(hex[4:6])
				if ok1 && ok2 && ok3 {
					return Color{R: r, G: g, B: b, Set: true}, nil
				}
			}
			return Color{}, fmt.Errorf("invalid colour %q", t.Text)
		}
	}
	// rgb(r, g, b)
	if len(filtered) >= 5 && filtered[0].Kind == IdentToken && strings.EqualFold(filtered[0].Text, "rgb") &&
		filtered[1].Kind == LParenToken && filtered[len(filtered)-1].Kind == RParenToken {
		var parts []float64
		for _, t := range filtered[2 : len(filtered)-1] {
			if t.Kind == CommaToken {
				continue
			}
			if t.Kind != NumberToken {
				return Color{}, fmt.Errorf("rgb() expects numbers")
			}
			parts = append(parts, t.Num)
		}
		if len(parts) != 3 {
			return Color{}, fmt.Errorf("rgb() expects three numbers")
		}
		clamp := func(v float64) uint8 {
			if v < 0 {
				return 0
			}
			if v > 255 {
				return 255
			}
			return uint8(math.Round(v))
		}
		return Color{R: clamp(parts[0]), G: clamp(parts[1]), B: clamp(parts[2]), Set: true}, nil
	}
	return Color{}, fmt.Errorf("invalid colour %q", stringify(toks))
}

// BorderStyle is the supported set of border styles.
type BorderStyle int

const (
	// BorderNone draws nothing.
	BorderNone BorderStyle = iota
	// BorderSolid draws a solid line.
	BorderSolid
)

// BorderSide is one side of a box border.
type BorderSide struct {
	Width int32
	Style BorderStyle
	Color Color
}

// IsVisible reports whether the side should be drawn.
func (b BorderSide) IsVisible() bool { return b.Style != BorderNone && b.Width > 0 }

// Display is the computed value of the display property.
type Display int

const (
	DisplayInline Display = iota
	DisplayBlock
	DisplayInlineBlock
	DisplayListItem
	DisplayTable
	DisplayTableRow
	DisplayTableCell
	DisplayNone
)

// TextAlign is the computed value of the text-align property.
type TextAlign int

const (
	AlignLeft TextAlign = iota
	AlignRight
	AlignCenter
	AlignJustify
)

// WhiteSpace is the computed value of the white-space property.
type WhiteSpace int

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpacePre
)

// VerticalAlign is the computed value of the vertical-align property.
type VerticalAlign int

const (
	VABaseline VerticalAlign = iota
	VATop
	VAMiddle
	VABottom
)

// Break is the computed value of a break-* property.
type Break int

const (
	BreakAuto Break = iota
	BreakPage
	BreakAvoid
)

// BorderCollapse is the computed value of border-collapse.
type BorderCollapse int

const (
	BorderCollapseSeparate BorderCollapse = iota
	BorderCollapseCollapse
)

// ListStyleType is the computed value of list-style-type.
type ListStyleType int

const (
	ListDisc ListStyleType = iota
	ListDecimal
)

// FontStyle is the computed value of font-style.
type FontStyle int

const (
	FontNormal FontStyle = iota
	FontItalic
)

func parseInt(t Token) (int, error) {
	if t.Kind != NumberToken {
		return 0, fmt.Errorf("expected a number, got %q", t.Text)
	}
	return int(t.Num), nil
}

func parseIdent(t Token) (string, error) {
	if t.Kind != IdentToken {
		return "", fmt.Errorf("expected a keyword, got %q", t.Text)
	}
	return strings.ToLower(t.Text), nil
}

func filterWhitespace(toks []Token) []Token {
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		if t.Kind == WhitespaceToken || t.Kind == EOFToken {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Value is a parsed CSS value. Only the field matching Kind is meaningful.
type Value struct {
	Kind     string
	Length   Length
	Color    Color
	Number   int
	Keyword  string
	Families []string
	Box      [4]Length
	Borders  [4]BorderSide
	// Enum holds an enumerated value (Display, TextAlign, Break, ...).
	Enum any
}
