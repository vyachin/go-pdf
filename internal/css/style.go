package css

import "math"

// Direction is the base direction of a block, taken from the HTML dir
// attribute of the element or of its nearest ancestor that sets one. DirAuto
// leaves the direction to the first strong character of the paragraph, which
// is also the behaviour when no element declares the attribute.
type Direction int

const (
	// DirAuto selects the direction from the paragraph's first strong
	// character, then left-to-right.
	DirAuto Direction = iota
	// DirLTR is an explicit left-to-right base direction.
	DirLTR
	// DirRTL is an explicit right-to-left base direction.
	DirRTL
)

// Style is a computed style. Geometry values are absolute millipoints unless
// they carry Percent or Auto, which need a layout-time basis.
type Style struct {
	Display        Display
	Width, Height  Length
	Margin         [4]Length // top, right, bottom, left
	Padding        [4]Length
	Border         [4]BorderSide
	BorderCollapse BorderCollapse
	Background     Color
	// BackgroundImage is the local source of a background image, or "" when
	// the box has none. BackgroundSize keeps the declared keyword.
	BackgroundImage string
	BackgroundSize  BackgroundSize
	Color           Color
	// inheritedColor is the parent's computed color. A currentColor value on
	// the color property computes to it even after an earlier declaration in
	// the same rule changed Color.
	inheritedColor Color
	FontFamily     []string
	FontSize       int32 // absolute millipoints
	FontWeight     int
	FontStyle      FontStyle
	LineHeight     Length // Auto or absolute millipoints
	// LineHeightFactor is set for a unitless line-height and follows the
	// font size of the element that inherits it.
	LineHeightFactor float64
	TextAlign        TextAlign
	// Dir is the base direction inherited from the nearest dir attribute.
	// Layout reads it to order the runs of a line and to choose the paragraph
	// level when the attribute is absent or auto.
	Dir Direction
	// TextDecoration is the set of lines painted under or through the text. It
	// is inherited because CSS propagates a decoration from the decorated box
	// to its descendants, which is the painted result the profile promises.
	TextDecoration Decoration
	TextIndent     Length
	WhiteSpace     WhiteSpace
	VerticalAlign  VerticalAlign
	BreakBefore    Break
	BreakAfter     Break
	BreakInside    Break
	Orphans        int
	Widows         int
	ListStyleType  ListStyleType
	// AfterContent is the generated content of a::after, if any.
	AfterContent []ContentItem
}

// DefaultStyle returns the initial style values, before any author rules.
func DefaultStyle() *Style {
	return &Style{
		Display:        DisplayInline,
		Width:          Length{Kind: Auto},
		Height:         Length{Kind: Auto},
		FontFamily:     []string{"serif"},
		FontSize:       12000, // 12pt, overridden by the UA sheet for documents
		FontWeight:     400,
		LineHeight:     Length{Kind: Auto},
		TextAlign:      AlignLeft,
		WhiteSpace:     WhiteSpaceNormal,
		VerticalAlign:  VABaseline,
		BreakBefore:    BreakAuto,
		BreakAfter:     BreakAuto,
		BreakInside:    BreakAuto,
		Orphans:        2,
		Widows:         2,
		ListStyleType:  ListDisc,
		BorderCollapse: BorderCollapseSeparate,
	}
}

// NewStyle builds the starting style of an element: initial values plus the
// inherited properties of the parent.
func NewStyle(parent *Style) *Style {
	s := DefaultStyle()
	if parent == nil {
		return s
	}
	s.Color = parent.Color
	s.inheritedColor = parent.Color
	s.FontFamily = append([]string(nil), parent.FontFamily...)
	s.FontSize = parent.FontSize
	s.FontWeight = parent.FontWeight
	s.FontStyle = parent.FontStyle
	s.LineHeight = parent.LineHeight
	s.TextIndent = parent.TextIndent
	s.LineHeightFactor = parent.LineHeightFactor
	s.TextAlign = parent.TextAlign
	s.Dir = parent.Dir
	s.TextDecoration = parent.TextDecoration
	s.WhiteSpace = parent.WhiteSpace
	s.ListStyleType = parent.ListStyleType
	s.Orphans = parent.Orphans
	s.Widows = parent.Widows
	return s
}

// resolveEm converts em-based lengths to absolute values using the element's
// own font size. It must run after font-size has been computed.
func (s *Style) resolveEm() {
	s.Width = s.Width.ResolveFont(s.FontSize)
	s.Height = s.Height.ResolveFont(s.FontSize)
	for i := range s.Margin {
		s.Margin[i] = s.Margin[i].ResolveFont(s.FontSize)
	}
	for i := range s.Padding {
		s.Padding[i] = s.Padding[i].ResolveFont(s.FontSize)
	}
	for i := range s.Border {
		if s.Border[i].WidthLen.Kind != Auto {
			s.Border[i].Width = s.Border[i].WidthLen.ResolveFont(s.FontSize).Value
		}
	}
	s.TextIndent = s.TextIndent.ResolveFont(s.FontSize)
	if s.LineHeightFactor > 0 {
		s.LineHeight = Length{Kind: Auto}
	} else {
		s.LineHeight = s.LineHeight.ResolveFont(s.FontSize)
	}
}

// resolveCurrentColor replaces every remaining currentColor value with the
// element's computed color. It runs after all declarations have been applied,
// so a border declared before the color in the same rule still follows the
// final color. A computed color that is unset paints as black, which is the
// initial color value.
func (s *Style) resolveCurrentColor() {
	current := s.Color
	if current.CurrentColor || (!current.Set && !current.Transparent) {
		current = Color{Set: true}
	}
	if s.Background.CurrentColor {
		s.Background = current
	}
	for i := range s.Border {
		if s.Border[i].Color.CurrentColor {
			s.Border[i].Color = current
		}
	}
}

// LineHeightMP returns the used line height in millipoints, falling back to
// 1.2 times the font size for "auto".
func (s *Style) LineHeightMP() int32 {
	var v int64
	switch {
	case s.LineHeightFactor > 0:
		v = int64(math.Round(s.LineHeightFactor * float64(s.FontSize)))
	case s.LineHeight.IsAuto():
		v = int64(s.FontSize) * 12 / 10
	default:
		return s.LineHeight.Value
	}
	// Saturate: a large factor or font size must not narrow the product into a
	// negative line height.
	if v > int64(maxMillipoints) {
		return maxMillipoints
	}
	if v < -int64(maxMillipoints) {
		return -maxMillipoints
	}
	return int32(v)
}
