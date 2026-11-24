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

func TestBackgroundImageAndCover(t *testing.T) {
	st := DefaultStyle()
	if err := ApplyDecl(st, "background-image", Tokenize([]byte("url(cover.jpg)")), 12000); err != nil {
		t.Fatalf("url(cover.jpg): %v", err)
	}
	if st.BackgroundImage != "cover.jpg" {
		t.Fatalf("background-image = %q, want cover.jpg", st.BackgroundImage)
	}
	if st.BackgroundSize != BackgroundSizeAuto {
		t.Fatalf("background-size initial = %v, want auto", st.BackgroundSize)
	}
	if err := ApplyDecl(st, "background-size", Tokenize([]byte("cover")), 12000); err != nil {
		t.Fatalf("background-size: cover: %v", err)
	}
	if st.BackgroundSize != BackgroundSizeCover {
		t.Fatalf("background-size = %v, want cover", st.BackgroundSize)
	}
	quoted := DefaultStyle()
	if err := ApplyDecl(quoted, "background-image", Tokenize([]byte(`url("photos/cover shot.jpg")`)), 12000); err != nil {
		t.Fatalf("quoted url: %v", err)
	}
	if quoted.BackgroundImage != "photos/cover shot.jpg" {
		t.Fatalf("quoted background-image = %q", quoted.BackgroundImage)
	}
}

func TestBackgroundImageRejectsUnsupportedSources(t *testing.T) {
	invalid := []struct{ property, value string }{
		{"background-image", "linear-gradient(#000, #fff)"},
		{"background-image", "url(https://example.com/cover.jpg)"},
		{"background-image", `url("data:text/plain;base64,AAAA")`},
		{"background-image", "none"},
		{"background-image", "url()"},
		{"background-size", "contain"},
		{"background-size", "auto"},
		{"background-size", "20mm 20mm"},
		{"background-size", "100% 100%"},
	}
	for _, tc := range invalid {
		st := DefaultStyle()
		if err := ApplyDecl(st, tc.property, Tokenize([]byte(tc.value)), 12000); err == nil {
			t.Errorf("%s: %s was accepted", tc.property, tc.value)
		}
	}
}

func TestDataURIBackgroundIsAccepted(t *testing.T) {
	for _, value := range []string{
		`url("data:image/png;base64,AAAA")`,
		"url('data:image/jpeg;base64,AAAA')",
	} {
		st := DefaultStyle()
		if err := ApplyDecl(st, "background-image", Tokenize([]byte(value)), 12000); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if st.BackgroundImage == "" {
			t.Fatalf("%s: the URL was not stored", value)
		}
	}
}
