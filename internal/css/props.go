package css

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// PropSpec describes one supported CSS property: how its value is parsed and
// how it is written into a computed style.
type PropSpec struct {
	Name      string
	Inherited bool
	Parse     func(toks []Token) (Value, error)
	Set       func(s *Style, v Value, parentFontSize int32) error
}

// PropOrder lists supported properties in a stable order, used by the CLI
// profile listing.
var PropOrder = []string{
	"display",
	"width", "height",
	"margin", "padding", "border", "border-collapse",
	"background-color", "color",
	"font-family", "font-size", "font-weight", "font-style", "line-height",
	"text-align", "text-indent", "white-space", "vertical-align",
	"break-before", "break-after", "break-inside",
	"page-break-before", "page-break-after", "page-break-inside",
	"orphans", "widows",
	"list-style-type",
}

// Table maps a property name to its specification. It is the single source of
// truth for parsing, validation and documentation.
var Table = map[string]PropSpec{
	"display": {
		Name: "display",
		Parse: parseKeyword(map[string]Display{
			"block": DisplayBlock, "inline": DisplayInline, "inline-block": DisplayInlineBlock,
			"list-item": DisplayListItem, "table": DisplayTable, "table-row": DisplayTableRow,
			"table-cell": DisplayTableCell, "none": DisplayNone,
		}, "display"),
		Set: func(s *Style, v Value, _ int32) error {
			s.Display = v.Enum.(Display)
			return nil
		},
	},
	"width":  {Name: "width", Parse: parseNonNegativeLength, Set: setLength(func(s *Style, l Length) { s.Width = l })},
	"height": {Name: "height", Parse: parseNonNegativeLength, Set: setLength(func(s *Style, l Length) { s.Height = l })},
	"margin": {Name: "margin", Parse: parseMarginShorthand, Set: func(s *Style, v Value, _ int32) error {
		s.Margin = v.Box
		return nil
	}},
	"padding": {Name: "padding", Parse: parsePaddingShorthand, Set: func(s *Style, v Value, _ int32) error {
		s.Padding = v.Box
		return nil
	}},
	"border": {Name: "border", Parse: parseBorderShorthand, Set: func(s *Style, v Value, _ int32) error {
		s.Border = v.Borders
		return nil
	}},
	"border-collapse": {
		Name:  "border-collapse",
		Parse: parseKeyword(map[string]BorderCollapse{"collapse": BorderCollapseCollapse, "separate": BorderCollapseSeparate}, "border-collapse"),
		Set: func(s *Style, v Value, _ int32) error {
			s.BorderCollapse = v.Enum.(BorderCollapse)
			return nil
		},
	},
	"background-color": {
		Name: "background-color",
		Parse: func(toks []Token) (Value, error) {
			c, err := ParseColor(toks)
			return Value{Kind: "color", Color: c}, err
		},
		Set: func(s *Style, v Value, _ int32) error {
			s.Background = v.Color
			return nil
		},
	},
	"color": {
		Name:      "color",
		Inherited: true,
		Parse: func(toks []Token) (Value, error) {
			c, err := ParseColor(toks)
			return Value{Kind: "color", Color: c}, err
		},
		Set: func(s *Style, v Value, _ int32) error {
			if v.Color.Transparent {
				return fmt.Errorf("color: transparent is not supported")
			}
			s.Color = v.Color
			return nil
		},
	},
	"font-family": {
		Name:      "font-family",
		Inherited: true,
		Parse:     parseFontFamily,
		Set: func(s *Style, v Value, _ int32) error {
			s.FontFamily = v.Families
			return nil
		},
	},
	"font-size": {
		Name:      "font-size",
		Inherited: true,
		Parse:     parseSingleLength,
		Set: func(s *Style, v Value, parentFontSize int32) error {
			l := v.Length
			switch l.Kind {
			case MP:
				if l.Value <= 0 {
					return fmt.Errorf("font-size must be positive")
				}
				s.FontSize = l.Value
			case Percent, Em:
				size := l.Resolve(parentFontSize)
				if size <= 0 {
					return fmt.Errorf("font-size must be positive")
				}
				s.FontSize = size
			default:
				return fmt.Errorf("font-size must be a length")
			}
			return nil
		},
	},
	"font-weight": {
		Name:      "font-weight",
		Inherited: true,
		Parse: func(toks []Token) (Value, error) {
			f := filterWhitespace(toks)
			if len(f) != 1 {
				return Value{}, fmt.Errorf("font-weight expects one value")
			}
			switch f[0].Kind {
			case NumberToken:
				if f[0].Bad || !finiteInRange(f[0].Num) {
					return Value{}, fmt.Errorf("font-weight %q is out of range", f[0].Text)
				}
				return Value{Kind: "number", Number: int(f[0].Num)}, nil
			case IdentToken:
				switch strings.ToLower(f[0].Text) {
				case "normal":
					return Value{Kind: "number", Number: 400}, nil
				case "bold":
					return Value{Kind: "number", Number: 700}, nil
				}
			}
			return Value{}, fmt.Errorf("unsupported font-weight %q", f[0].Text)
		},
		Set: func(s *Style, v Value, _ int32) error {
			if v.Number < 100 || v.Number > 900 {
				return fmt.Errorf("font-weight %d is out of range", v.Number)
			}
			s.FontWeight = v.Number
			return nil
		},
	},
	"font-style": {
		Name:      "font-style",
		Inherited: true,
		Parse:     parseKeyword(map[string]FontStyle{"normal": FontNormal, "italic": FontItalic}, "font-style"),
		Set: func(s *Style, v Value, _ int32) error {
			s.FontStyle = v.Enum.(FontStyle)
			return nil
		},
	},
	"line-height": {
		Name:      "line-height",
		Inherited: true,
		Parse: func(toks []Token) (Value, error) {
			f := filterWhitespace(toks)
			if len(f) != 1 {
				return Value{}, fmt.Errorf("line-height expects one value")
			}
			if f[0].Kind == IdentToken {
				if strings.EqualFold(f[0].Text, "normal") {
					return Value{Kind: "length", Length: Length{Kind: Auto}}, nil
				}
				return Value{}, fmt.Errorf("line-height expects normal, a number or a length, not %q", f[0].Text)
			}
			if f[0].Kind == NumberToken {
				if f[0].Bad || !finiteInRange(f[0].Num) || f[0].Num > maxLineHeightFactor {
					return Value{}, fmt.Errorf("line-height factor %q is out of range", f[0].Text)
				}
				if f[0].Num <= 0 {
					return Value{}, fmt.Errorf("line-height must be positive")
				}
				// A unitless number is a factor, not an em length: it must
				// follow the font size of the element that inherits it.
				return Value{Kind: "length", Number: int(f[0].Num), Float: f[0].Num}, nil
			}
			l, err := ParseLengthToken(f[0])
			if err != nil {
				return Value{}, err
			}
			if l.Kind == Percent {
				// Length percentages are stored as 100000 per 100%; an em
				// factor of the same value uses the same scale.
				l = EmLen(float64(l.Value) / 100000)
			}
			if l.Kind == MP && l.Value <= 0 {
				return Value{}, fmt.Errorf("line-height must be positive")
			}
			return Value{Kind: "length", Length: l}, nil
		},
		Set: func(s *Style, v Value, _ int32) error {
			if v.Float > 0 {
				s.LineHeightFactor = v.Float
				s.LineHeight = Length{Kind: Auto}
				return nil
			}
			s.LineHeightFactor = 0
			s.LineHeight = v.Length
			return nil
		},
	},
	"text-align": {
		Name:      "text-align",
		Inherited: true,
		Parse:     parseKeyword(map[string]TextAlign{"left": AlignLeft, "right": AlignRight, "center": AlignCenter, "justify": AlignJustify}, "text-align"),
		Set: func(s *Style, v Value, _ int32) error {
			s.TextAlign = v.Enum.(TextAlign)
			return nil
		},
	},
	"text-indent": {
		Name:      "text-indent",
		Inherited: true,
		Parse:     parseTextIndent,
		Set: setLength(func(s *Style, l Length) {
			s.TextIndent = l
		}),
	},
	"white-space": {
		Name:      "white-space",
		Inherited: true,
		Parse:     parseKeyword(map[string]WhiteSpace{"normal": WhiteSpaceNormal, "pre": WhiteSpacePre}, "white-space"),
		Set: func(s *Style, v Value, _ int32) error {
			s.WhiteSpace = v.Enum.(WhiteSpace)
			return nil
		},
	},
	"vertical-align": {
		Name:  "vertical-align",
		Parse: parseKeyword(map[string]VerticalAlign{"baseline": VABaseline, "top": VATop, "middle": VAMiddle, "bottom": VABottom}, "vertical-align"),
		Set: func(s *Style, v Value, _ int32) error {
			s.VerticalAlign = v.Enum.(VerticalAlign)
			return nil
		},
	},
	"break-before": {
		Name:  "break-before",
		Parse: parseBreak,
		Set: func(s *Style, v Value, _ int32) error {
			s.BreakBefore = v.Enum.(Break)
			return nil
		},
	},
	"page-break-before": {
		Name:  "page-break-before",
		Parse: parseBreak,
		Set: func(s *Style, v Value, _ int32) error {
			s.BreakBefore = v.Enum.(Break)
			return nil
		},
	},
	"break-after": {
		Name:  "break-after",
		Parse: parseBreak,
		Set: func(s *Style, v Value, _ int32) error {
			s.BreakAfter = v.Enum.(Break)
			return nil
		},
	},
	"page-break-after": {
		Name:  "page-break-after",
		Parse: parseBreak,
		Set: func(s *Style, v Value, _ int32) error {
			s.BreakAfter = v.Enum.(Break)
			return nil
		},
	},
	"break-inside": {
		Name:  "break-inside",
		Parse: parseBreak,
		Set: func(s *Style, v Value, _ int32) error {
			s.BreakInside = v.Enum.(Break)
			return nil
		},
	},
	"page-break-inside": {
		Name:  "page-break-inside",
		Parse: parseBreak,
		Set: func(s *Style, v Value, _ int32) error {
			s.BreakInside = v.Enum.(Break)
			return nil
		},
	},
	"orphans": {
		Name:      "orphans",
		Inherited: true,
		Parse:     parseInteger,
		Set: func(s *Style, v Value, _ int32) error {
			if v.Number < 1 {
				return fmt.Errorf("orphans must be at least 1")
			}
			s.Orphans = v.Number
			return nil
		},
	},
	"widows": {
		Name:      "widows",
		Inherited: true,
		Parse:     parseInteger,
		Set: func(s *Style, v Value, _ int32) error {
			if v.Number < 1 {
				return fmt.Errorf("widows must be at least 1")
			}
			s.Widows = v.Number
			return nil
		},
	},
	"list-style-type": {
		Name:      "list-style-type",
		Inherited: true,
		Parse:     parseKeyword(map[string]ListStyleType{"disc": ListDisc, "decimal": ListDecimal}, "list-style-type"),
		Set: func(s *Style, v Value, _ int32) error {
			s.ListStyleType = v.Enum.(ListStyleType)
			return nil
		},
	},
}

func setLength(fn func(*Style, Length)) func(*Style, Value, int32) error {
	return func(s *Style, v Value, _ int32) error {
		fn(s, v.Length)
		return nil
	}
}

var errMultipleValues = fmt.Errorf("expected a single value")

func parseSingleLength(toks []Token) (Value, error) {
	f := filterWhitespace(toks)
	if len(f) != 1 {
		return Value{}, errMultipleValues
	}
	l, err := ParseLengthToken(f[0])
	if err != nil {
		return Value{}, err
	}
	return Value{Kind: "length", Length: l}, nil
}

// parseNonNegativeLength parses width or height: auto or a non-negative length.
func parseNonNegativeLength(toks []Token) (Value, error) {
	v, err := parseSingleLength(toks)
	if err != nil {
		return Value{}, err
	}
	if v.Length.Kind != Auto && v.Length.Value < 0 {
		return Value{}, fmt.Errorf("a box dimension must not be negative")
	}
	return v, nil
}

// parsePaddingShorthand parses padding: one to four non-negative lengths. The
// generic box parser accepts negatives for margins, which padding does not.
// parseMarginShorthand parses the margin shorthand. Auto margins are not part
// of the profile: they used to be accepted and then resolved to zero, which
// silently turned "margin: 0 auto" into a left-aligned block. An explicit error
// is better than a wrong layout.
func parseMarginShorthand(toks []Token) (Value, error) {
	v, err := parseBoxShorthand(toks)
	if err != nil {
		return Value{}, err
	}
	for _, l := range v.Box {
		if l.Kind == Auto {
			return Value{}, fmt.Errorf("margin must not be auto, use an explicit length")
		}
	}
	return v, nil
}

// parseTextIndent parses text-indent: one length, and no keyword.
func parseTextIndent(toks []Token) (Value, error) {
	f := filterWhitespace(toks)
	if len(f) == 1 && f[0].Kind == IdentToken {
		return Value{}, fmt.Errorf("text-indent expects a length, not %q", f[0].Text)
	}
	return parseSingleLength(toks)
}

func parsePaddingShorthand(toks []Token) (Value, error) {
	v, err := parseBoxShorthand(toks)
	if err != nil {
		return Value{}, err
	}
	for _, l := range v.Box {
		if l.Kind == Auto {
			return Value{}, fmt.Errorf("padding must not be auto")
		}
		if l.Value < 0 {
			return Value{}, fmt.Errorf("padding must not be negative")
		}
	}
	return v, nil
}

func parseBoxShorthand(toks []Token) (Value, error) {
	f := filterWhitespace(toks)
	if len(f) < 1 || len(f) > 4 {
		return Value{}, fmt.Errorf("expected one to four lengths")
	}
	var parts [4]Length
	for i, t := range f {
		l, err := ParseLengthToken(t)
		if err != nil {
			return Value{}, err
		}
		parts[i] = l
	}
	var box [4]Length
	switch len(f) {
	case 1:
		box = [4]Length{parts[0], parts[0], parts[0], parts[0]}
	case 2:
		box = [4]Length{parts[0], parts[1], parts[0], parts[1]}
	case 3:
		box = [4]Length{parts[0], parts[1], parts[2], parts[1]}
	case 4:
		box = parts
	}
	return Value{Kind: "box", Box: box}, nil
}

func parseBorderShorthand(toks []Token) (Value, error) {
	f := filterWhitespace(toks)
	if len(f) == 0 {
		return Value{}, fmt.Errorf("border expects a width, style and colour")
	}
	side := BorderSide{Style: BorderNone, Color: Color{R: 0, G: 0, B: 0, Set: true}}
	var widthSet bool
	for _, t := range f {
		switch t.Kind {
		case IdentToken:
			switch strings.ToLower(t.Text) {
			case "solid":
				side.Style = BorderSolid
			case "none":
				side.Style = BorderNone
			case "thin":
				side.Width, widthSet = 750, true
			case "medium":
				side.Width, widthSet = 2250, true
			case "thick":
				side.Width, widthSet = 3750, true
			default:
				c, err := ParseColor([]Token{t})
				if err != nil {
					return Value{}, fmt.Errorf("unsupported border value %q", t.Text)
				}
				side.Color = c
			}
		case HashToken:
			c, err := ParseColor([]Token{t})
			if err != nil {
				return Value{}, err
			}
			side.Color = c
		default:
			l, err := ParseLengthToken(t)
			if err != nil {
				return Value{}, err
			}
			if l.Kind == Percent || l.Kind == Auto {
				return Value{}, fmt.Errorf("border width must be a length, not a percentage or auto")
			}
			if l.Value < 0 {
				return Value{}, fmt.Errorf("border width must not be negative")
			}
			// Keep the unit: an em width must be resolved against the
			// element font size, not treated as absolute millipoints.
			side.WidthLen = l
			side.Width = l.Value
			widthSet = true
		}
	}
	if side.Style == BorderNone {
		side.Width = 0
	} else if !widthSet {
		side.Width = 2250
	}
	return Value{Kind: "borders", Borders: [4]BorderSide{side, side, side, side}}, nil
}

func parseFontFamily(toks []Token) (Value, error) {
	var families []string
	var current []string
	flush := func() {
		if len(current) > 0 {
			families = append(families, strings.Join(current, " "))
			current = nil
		}
	}
	for _, t := range toks {
		switch t.Kind {
		case WhitespaceToken, EOFToken:
			continue
		case IdentToken:
			current = append(current, t.Text)
		case StringToken:
			flush()
			families = append(families, unquote(t.Text))
		case CommaToken:
			flush()
		default:
			return Value{}, fmt.Errorf("unsupported font-family syntax %q", t.Text)
		}
	}
	flush()
	if len(families) == 0 {
		return Value{}, fmt.Errorf("font-family expects at least one family")
	}
	return Value{Kind: "families", Families: families}, nil
}

func parseInteger(toks []Token) (Value, error) {
	f := filterWhitespace(toks)
	if len(f) != 1 || f[0].Kind != NumberToken {
		return Value{}, fmt.Errorf("expected an integer")
	}
	if f[0].Bad || !finiteInRange(f[0].Num) {
		return Value{}, fmt.Errorf("number %q is out of range", f[0].Text)
	}
	if f[0].Num != math.Trunc(f[0].Num) {
		return Value{}, fmt.Errorf("%q is not an integer", f[0].Text)
	}
	return Value{Kind: "number", Number: int(f[0].Num)}, nil
}

// maxLineHeightFactor bounds a unitless line-height in the profile. The factor
// is multiplied by the font size at use time, and a corrupt value would
// otherwise narrow that product.
const maxLineHeightFactor = 1000

// finiteInRange reports whether v is a finite number that fits an int32 when
// used as a count or a millipoint value.
func finiteInRange(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v <= float64(maxMillipoints) && v >= -float64(maxMillipoints)
}

func parseBreak(toks []Token) (Value, error) {
	f := filterWhitespace(toks)
	if len(f) != 1 || f[0].Kind != IdentToken {
		return Value{}, fmt.Errorf("expected auto, page or avoid")
	}
	switch strings.ToLower(f[0].Text) {
	case "auto":
		return Value{Kind: "break", Enum: BreakAuto}, nil
	case "page", "always":
		return Value{Kind: "break", Enum: BreakPage}, nil
	case "avoid":
		return Value{Kind: "break", Enum: BreakAvoid}, nil
	}
	return Value{}, fmt.Errorf("expected auto, page or avoid, got %q", f[0].Text)
}

func parseKeyword[T any](m map[string]T, what string) func([]Token) (Value, error) {
	return func(toks []Token) (Value, error) {
		f := filterWhitespace(toks)
		if len(f) != 1 || f[0].Kind != IdentToken {
			return Value{}, fmt.Errorf("expected one keyword for %s", what)
		}
		v, ok := m[strings.ToLower(f[0].Text)]
		if !ok {
			return Value{}, fmt.Errorf("unsupported %s value %q", what, f[0].Text)
		}
		return Value{Kind: "enum", Enum: v}, nil
	}
}

// ApplyDecl applies one declaration to a style, reporting unsupported
// properties and values as errors.
func ApplyDecl(s *Style, name string, toks []Token, parentFontSize int32) error {
	spec, ok := Table[name]
	if !ok {
		return fmt.Errorf("property %q is not supported", name)
	}
	v, err := spec.Parse(toks)
	if err != nil {
		return err
	}
	if err := spec.Set(s, v, parentFontSize); err != nil {
		return err
	}
	return nil
}

// SupportedElements is the white list of element names in the profile.
var SupportedElements = map[string]bool{
	"html": true, "head": true, "body": true, "div": true, "p": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true,
	"tr": true, "td": true, "th": true,
	"img": true, "span": true, "br": true, "hr": true,
	"strong": true, "b": true, "em": true, "i": true, "a": true,
	"title": true, "meta": true, "style": true, "link": true,
}

// ElementOrder returns supported elements in a stable order.
func ElementOrder() []string {
	out := make([]string, 0, len(SupportedElements))
	for k := range SupportedElements {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
