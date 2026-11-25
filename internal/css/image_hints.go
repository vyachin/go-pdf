package css

import (
	"math"
	"strings"

	"github.com/vyachin/go-pdf/internal/html"
)

// cssPixelMP is one CSS pixel in millipoints: 0.75 pt at 96 dpi.
const cssPixelMP = 750

// imageHintMP parses a non-negative integer HTML length attribute as CSS
// pixels. It reports false for anything HTML does not accept as a pixel
// dimension — a percentage, a unit, a sign, an empty value — and for a value
// whose millipoint form would not fit int32, so an absurd attribute is an
// absent hint rather than a wrapped geometry.
func imageHintMP(v string) (int32, bool) {
	v = strings.Trim(v, " \t\r\n\f")
	if v == "" {
		return 0, false
	}
	var pixels int64
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		pixels = pixels*10 + int64(c-'0')
		// The check runs on every digit, in 64 bits, so a long attribute cannot
		// overflow before the limit is noticed.
		if pixels > math.MaxInt32/cssPixelMP {
			return 0, false
		}
	}
	return int32(pixels * cssPixelMP), true
}

// applyImageHints applies the width and height attributes of an image as
// presentation hints. CSS gives presentation hints a lower priority than author
// declarations, so they are applied before the cascade runs: an author rule —
// including an explicit `auto` or an `!important` declaration — overrides them,
// and an unspecified or invalid attribute leaves the property at its initial
// value.
func applyImageHints(st *Style, n *html.Node) {
	if n == nil || n.Type != html.ElementNode || !strings.EqualFold(n.Tag, "img") {
		return
	}
	if v, ok := n.Attr("width"); ok {
		if mp, valid := imageHintMP(v); valid {
			st.Width = Abs(mp)
		}
	}
	if v, ok := n.Attr("height"); ok {
		if mp, valid := imageHintMP(v); valid {
			st.Height = Abs(mp)
		}
	}
}
