package docdump

import (
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/layout"
	"github.com/vyachin/go-pdf/internal/profile"
)

var (
	testSize   = css.PageSize{Width: 595276, Height: 841890}
	testMargin = [4]int32{56693, 56693, 56693, 56693}
)

// dump parses and prepares a snippet the same way the engine does, without
// importing the engine (which would cycle through docdump).
func dump(t *testing.T, src string) string {
	t.Helper()
	doc, err := html.Parse([]byte(src), "test.html")
	if err != nil {
		t.Fatal(err)
	}
	opts := profile.Options{}
	diags := profile.Validate(doc, opts)
	sheets, sheetDiags := profile.CollectStylesheets(doc, opts)
	diags = append(diags, sheetDiags...)
	if diags.HasErrors() {
		t.Fatalf("validation failed: %v", diags.Lines())
	}
	cascade := css.NewCascadeForDocument(css.DefaultStylesheet(), sheets...)
	page := layout.ParsePageStyle(sheets, testSize, testMargin)
	in := Input{
		Document: doc,
		Cascade:  cascade,
		Sheets:   sheets,
		Size:     page.Size,
		Margin:   page.Margin,
		Boxes:    page.Boxes,
		Diags:    diags,
	}
	return string(XML(in))
}

func TestXMLCanonical(t *testing.T) {
	src := `<html><head><title>T</title></head><body><p class="x">Hi</p></body></html>`
	want := `<?xml version="1.0" encoding="UTF-8"?>
<document version="1">
  <meta title="T"/>
  <page width="595.276pt" height="841.89pt" margin-top="56.693pt" margin-right="56.693pt" margin-bottom="56.693pt" margin-left="56.693pt"/>
  <html display="block">
    <body display="block">
      <p class="x" display="block" margin-bottom="8.504pt">Hi</p>
    </body>
  </html>
  <diagnostics/>
</document>
`
	got := dump(t, src)
	if got != want {
		t.Fatalf("snapshot differs\n got:\n%s\nwant:\n%s", got, want)
	}
	if again := dump(t, src); again != got {
		t.Fatal("a second snapshot of the same document differs")
	}
}

func TestStyleElision(t *testing.T) {
	src := `<html><body style="font-size: 11pt; margin: 0 0 0 20pt">
		<div style="color: #666666">x</div>
		<p style="font-size: 13pt">y</p>
	</body></html>`
	got := dump(t, src)

	// The body differs from html in an inherited property and from the initial
	// value in a non-inherited one: both are written.
	body := lineWith(t, got, "<body")
	for _, want := range []string{`font-size="11pt"`, `margin-left="20pt"`, `display="block"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body does not carry %s: %s", want, body)
		}
	}
	// Zero margins are the initial value and stay out.
	if strings.Contains(body, `margin-bottom="0pt"`) {
		t.Fatalf("body repeats the initial margin: %s", body)
	}
	// The div inherits 11pt and brings its own colour.
	div := lineWith(t, got, "<div")
	if strings.Contains(div, "font-size") {
		t.Fatalf("div repeats the inherited font-size: %s", div)
	}
	if !strings.Contains(div, `color="#666666"`) || !strings.Contains(div, `display="block"`) {
		t.Fatalf("div does not carry its own values: %s", div)
	}
	// The paragraph sets its own size and does not repeat the inherited colour.
	p := lineWith(t, got, "<p ")
	if !strings.Contains(p, `font-size="13pt"`) {
		t.Fatalf("p does not carry its font size: %s", p)
	}
	if strings.Contains(p, "color") {
		t.Fatalf("p repeats an inherited colour: %s", p)
	}
}

func TestLengthRendering(t *testing.T) {
	src := `<html><body><p style="margin: 0 0 0 22.677pt; width: 50%; text-indent: 1.5em">x</p></body></html>`
	got := dump(t, src)
	// 1.5em is resolved against the 12pt font size by the cascade.
	for _, want := range []string{`margin-left="22.677pt"`, `width="50%"`, `text-indent="18pt"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("snapshot does not contain %s:\n%s", want, got)
		}
	}
}

func TestTextNormalisation(t *testing.T) {
	compact := `<html><body><p>Hello <strong>world</strong>!</p></body></html>`
	indented := `<html><body>
		<p>
			Hello
			<strong>world</strong>!
		</p>
	</body></html>`
	if a, b := dump(t, compact), dump(t, indented); a != b {
		t.Fatalf("reindenting the source changed the snapshot\ncompact:\n%s\nindented:\n%s", a, b)
	}
	got := dump(t, compact)
	if !strings.Contains(got, `<p display="block" margin-bottom="8.504pt">Hello <strong font-weight="700">world</strong>!</p>`) {
		t.Fatalf("inline text lost its separating space or a style:\n%s", got)
	}
}

func TestPreKeepsSpacing(t *testing.T) {
	src := "<html><body><p style=\"white-space: pre\">  a\n   b</p></body></html>"
	got := dump(t, src)
	if !strings.Contains(got, "<p display=\"block\" margin-bottom=\"8.504pt\" white-space=\"pre\">  a\n   b</p>") {
		t.Fatalf("pre content is not verbatim:\n%s", got)
	}
}

func TestDocumentLevel(t *testing.T) {
	src := `<html><head>
		<title>Doc</title>
		<meta name="author" content="A">
		<style>
			@page { size: 100pt 200pt; margin: 10mm; @top-right { content: "X" } }
			@font-face { font-family: "F"; src: url("DejaVuSans.ttf") }
		</style>
	</head><body><p>x</p></body></html>`
	got := dump(t, src)
	for _, want := range []string{
		`<meta title="Doc" author="A"/>`,
		`<page width="100pt" height="200pt" margin-top="28.346pt"`,
		`<margin-box name="top-right" content="&quot;X&quot;"/>`,
		`<font-face family="F" src="DejaVuSans.ttf" weight="400" style="normal"/>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("snapshot does not contain %s:\n%s", want, got)
		}
	}
}

func lineWith(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, prefix) {
			return line
		}
	}
	t.Fatalf("no line contains %q", prefix)
	return ""
}

func TestBackgroundImageInSnapshot(t *testing.T) {
	src := `<html><body><div style="background-image: url(cover.jpg); background-size: cover">x</div></body></html>`
	got := dump(t, src)
	div := lineWith(t, got, "<div")
	if !strings.Contains(div, `background-image="url(cover.jpg)"`) {
		t.Fatalf("div does not carry its background image: %s", div)
	}
	if !strings.Contains(div, `background-size="cover"`) {
		t.Fatalf("div does not carry its background size: %s", div)
	}
	body := lineWith(t, got, "<body")
	if strings.Contains(body, "background-") {
		t.Fatalf("the background leaked to the body: %s", body)
	}
	// Without an explicit size the initial auto is elided like any other
	// initial value.
	implicit := dump(t, `<html><body><div style="background-image: url(cover.jpg)">x</div></body></html>`)
	if strings.Contains(lineWith(t, implicit, "<div"), "background-size") {
		t.Fatalf("the initial background-size was written: %s", lineWith(t, implicit, "<div"))
	}
}
