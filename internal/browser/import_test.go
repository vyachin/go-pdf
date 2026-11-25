package browser

import (
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/profile"
)

func importSource(t *testing.T, src string) (*html.Document, Result) {
	t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		t.Fatal(err)
	}
	sheets, diags := profile.CollectStylesheets(doc, profile.Options{})
	if diags.HasErrors() {
		t.Fatalf("stylesheets: %v", diags.Lines())
	}
	res := Import(doc, sheets, Options{
		PageSize: css.PageSize{Width: 595276, Height: 841890},
		Margin:   [4]int32{56693, 56693, 56693, 56693},
	})
	return doc, res
}

func findTag(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Tag == tag {
		return n
	}
	for _, c := range n.Children {
		if found := findTag(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func styleOf(t *testing.T, n *html.Node) string {
	t.Helper()
	if n == nil {
		t.Fatal("node not found")
	}
	v, _ := n.Attr("style")
	return v
}

// TestBorderBoxResolvesEmBorder guards RV2-18: a border-box height used to
// subtract an unresolved em border, so 500 pt became 300 pt instead of 476 pt.
func TestBorderBoxResolvesEmBorder(t *testing.T) {
	doc, _ := importSource(t, `<div style="box-sizing: border-box; width: 100pt; height: 500pt; font-size: 12pt; border: 1em solid #000">x</div>`)
	div := findTag(doc.Body(), "div")
	style := styleOf(t, div)
	if !strings.Contains(style, "height:476.000pt") {
		t.Fatalf("style = %q, want height 476pt", style)
	}
	if !strings.Contains(style, "width:76.000pt") {
		t.Fatalf("style = %q, want width 76pt", style)
	}
}

// TestFlexItemAutoMarginWarns guards the dropped margin: the vertical part is
// lowered to padding and the horizontal auto is reported, not dropped silently.
func TestFlexItemAutoMarginWarns(t *testing.T) {
	doc, res := importSource(t, `<div style="display: flex; flex-direction: column"><p style="margin: 10pt auto">x</p></div>`)
	warned := false
	for _, d := range res.Diagnostics {
		if d.Code == "browser/flex-margin" && strings.Contains(d.Message, "auto") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("no warning for the auto margin:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	p := findTag(doc.Body(), "p")
	wrapper := p.Parent
	style := styleOf(t, wrapper)
	if !strings.Contains(style, "padding-top:10.000pt") || !strings.Contains(style, "padding-bottom:10.000pt") {
		t.Fatalf("wrapper style = %q, want 10pt padding", style)
	}
}

// TestFlexItemNegativeMarginWarns guards a negative margin: it cannot become
// padding and must be reported.
func TestFlexItemNegativeMarginWarns(t *testing.T) {
	_, res := importSource(t, `<div style="display: flex; flex-direction: column"><p style="margin: -5pt 0">x</p></div>`)
	warned := false
	for _, d := range res.Diagnostics {
		if d.Code == "browser/flex-margin" && strings.Contains(d.Message, "negative") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("no warning for the negative margin:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}
