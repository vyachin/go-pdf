package css

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
	Color          Color
	FontFamily     []string
	FontSize       int32 // absolute millipoints
	FontWeight     int
	FontStyle      FontStyle
	LineHeight     Length // Auto or absolute millipoints
	TextAlign      TextAlign
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
	s.FontFamily = append([]string(nil), parent.FontFamily...)
	s.FontSize = parent.FontSize
	s.FontWeight = parent.FontWeight
	s.FontStyle = parent.FontStyle
	s.LineHeight = parent.LineHeight
	s.TextAlign = parent.TextAlign
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
	s.TextIndent = s.TextIndent.ResolveFont(s.FontSize)
	s.LineHeight = s.LineHeight.ResolveFont(s.FontSize)
}

// LineHeightMP returns the used line height in millipoints, falling back to
// 1.2 times the font size for "auto".
func (s *Style) LineHeightMP() int32 {
	if s.LineHeight.IsAuto() {
		return s.FontSize * 12 / 10
	}
	return s.LineHeight.Value
}
