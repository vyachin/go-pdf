package css

import (
	"testing"

	"github.com/vyachin/go-pdf/internal/html"
)

// styleOf computes the style of the first element with the given tag under the
// given stylesheet.
func styleOf(t *testing.T, sheetCSS, tag string) *Style {
	t.Helper()
	sheet := ParseSheet(SheetOptions{File: "test.css"}, []byte(sheetCSS))
	doc, err := html.Parse([]byte("<"+tag+">x</"+tag+">"), "test.html")
	if err != nil {
		t.Fatal(err)
	}
	c := NewCascade(sheet)
	var node *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if node != nil {
			return
		}
		if n.Type == html.ElementNode && n.Tag == tag {
			node = n
			return
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(doc.Root)
	if node == nil {
		t.Fatalf("no <%s> element", tag)
	}
	return c.Compute(node, nil)
}

func TestPercentageLineHeight(t *testing.T) {
	cases := []struct {
		css  string
		want int32
	}{
		{"p { line-height: 100% }", 12000},
		{"p { line-height: 120% }", 14400},
		{"p { line-height: 150%; font-size: 20pt }", 30000},
	}
	for _, tc := range cases {
		st := styleOf(t, tc.css, "p")
		if got := st.LineHeightMP(); got != tc.want {
			t.Fatalf("%s = %d, want %d", tc.css, got, tc.want)
		}
	}
}

func TestBorderEmWidthUsesFontSize(t *testing.T) {
	st := styleOf(t, "p { border: 1em solid #000000 }", "p")
	if got := st.Border[0].Width; got != 12000 {
		t.Fatalf("1em border at 12pt = %d, want 12000", got)
	}
	st = styleOf(t, "p { border: 2em solid #000000; font-size: 20pt }", "p")
	if got := st.Border[0].Width; got != 40000 {
		t.Fatalf("2em border at 20pt = %d, want 40000", got)
	}
}

func TestBorderRejectsPercentWidth(t *testing.T) {
	st := styleOf(t, "p { border: 10% solid #000000 }", "p")
	if st.Border[0].Style != BorderNone || st.Border[0].Width != 0 {
		t.Fatalf("a percentage border width must be rejected, got %+v", st.Border[0])
	}
}
