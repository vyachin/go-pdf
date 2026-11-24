package profile

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

func validate(t *testing.T, src string) diag.List {
	t.Helper()
	doc, err := html.Parse([]byte(src), "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	return Validate(doc)
}

func codes(l diag.List) []string {
	out := make([]string, len(l))
	for i, d := range l {
		out[i] = d.Code
	}
	return out
}

func TestValidDocumentHasNoDiagnostics(t *testing.T) {
	src := `<html><head><title>Счёт</title>
	<style>
	  @page { size: A4; margin: 20mm }
	  @page { @bottom-center { content: "Стр. " counter(page) " из " counter(pages) } }
	  body { font-family: "PT Serif"; font-size: 12pt }
	  table { width: 100%; border: 1px solid #000 }
	  td { padding: 2mm }
	</style></head>
	<body><h1>Счёт</h1><table><tr><td>1</td></tr></table></body></html>`
	l := validate(t, src)
	if l.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", l.Lines())
	}
}

func TestUnsupportedPropertyAndValue(t *testing.T) {
	src := `<style>
	  div { display: flex }
	  p { box-shadow: 0 0 4px #000 }
	</style><div><p>x</p></div>`
	l := validate(t, src)
	if len(l) != 2 {
		t.Fatalf("got %d diagnostics, want 2: %v", len(l), l.Lines())
	}
	if l[0].Code != "css/unsupported-value" || !strings.Contains(l[0].Message, "display") {
		t.Fatalf("first diagnostic = %+v", l[0])
	}
	if l[1].Code != "css/unsupported-property" || !strings.Contains(l[1].Message, "box-shadow") {
		t.Fatalf("second diagnostic = %+v", l[1])
	}
	for _, d := range l {
		if d.Pos.Line <= 0 || d.Pos.Col <= 0 {
			t.Fatalf("diagnostic without a useful position: %+v", d)
		}
	}
}

func TestUnsupportedElement(t *testing.T) {
	l := validate(t, `<video src="x"></video>`)
	if len(l) != 1 || l[0].Code != "html/unsupported-element" {
		t.Fatalf("diagnostics = %v", l.Lines())
	}
	if !strings.Contains(l[0].Message, "video") {
		t.Fatalf("message does not name the element: %q", l[0].Message)
	}
}

func TestDiagnosticsAreDeterministic(t *testing.T) {
	src := `<style>
	  a { display: grid }
	  b { margin: 1rem }
	</style><video>x</video>`
	first := validate(t, src)
	second := validate(t, src)
	if strings.Join(first.Lines(), "|") != strings.Join(second.Lines(), "|") {
		t.Fatalf("diagnostics differ between runs:\n%v\n%v", first.Lines(), second.Lines())
	}
	if len(first) != 3 {
		t.Fatalf("got %d diagnostics, want 3: %v", len(first), first.Lines())
	}
}

func TestMediaRules(t *testing.T) {
	ok := validate(t, `<style>@media print { p { color: #000 } }</style><p>x</p>`)
	if ok.HasErrors() {
		t.Fatalf("print media must be accepted: %v", ok.Lines())
	}
	bad := validate(t, `<style>@media screen { p { color: #000 } }</style><p>x</p>`)
	if len(bad) != 1 || bad[0].Code != "css/unsupported-media" {
		t.Fatalf("screen media diagnostics = %v", bad.Lines())
	}
}

func TestPageRules(t *testing.T) {
	bad := validate(t, `<style>@page { color: red }</style><p>x</p>`)
	if len(bad) != 1 || bad[0].Code != "css/unsupported-property" {
		t.Fatalf("page diagnostics = %v", bad.Lines())
	}
	badBox := validate(t, `<style>@page { @top-left { color: red } }</style><p>x</p>`)
	if len(badBox) != 1 || badBox[0].Code != "css/unsupported-property" {
		t.Fatalf("margin box diagnostics = %v", badBox.Lines())
	}
	badSize := validate(t, `<style>@page { size: A3 }</style><p>x</p>`)
	if len(badSize) != 1 || badSize[0].Code != "css/unsupported-value" {
		t.Fatalf("page size diagnostics = %v", badSize.Lines())
	}
}

func TestFontFace(t *testing.T) {
	ok := validate(t, `<style>@font-face { font-family: "PT Serif"; src: url(fonts/pt.ttf) }</style><p>x</p>`)
	if ok.HasErrors() {
		t.Fatalf("local font face must be accepted: %v", ok.Lines())
	}
	missing := validate(t, `<style>@font-face { font-family: "PT Serif" }</style><p>x</p>`)
	if len(missing) != 1 || missing[0].Code != "css/incomplete-font-face" {
		t.Fatalf("incomplete font face diagnostics = %v", missing.Lines())
	}
	remote := validate(t, `<style>@font-face { font-family: "x"; src: url(https://example.com/f.woff2) }</style><p>x</p>`)
	if len(remote) != 1 || remote[0].Code != "css/unsupported-value" {
		t.Fatalf("remote font diagnostics = %v", remote.Lines())
	}
}

func TestInlineStyleValidation(t *testing.T) {
	l := validate(t, `<p style="display: flex">x</p>`)
	if len(l) != 1 || l[0].Code != "css/unsupported-value" {
		t.Fatalf("diagnostics = %v", l.Lines())
	}
	if !strings.Contains(l[0].Element, "p") {
		t.Fatalf("diagnostic does not name the element path: %q", l[0].Element)
	}
}

func TestGeneratedContentRules(t *testing.T) {
	ok := validate(t, `<style>a.toc::after { content: target-counter(attr(href), page) }</style><a class="toc" href="#s">Раздел</a>`)
	if ok.HasErrors() {
		t.Fatalf("a::after with target-counter must be accepted: %v", ok.Lines())
	}
	before := validate(t, `<style>p::before { content: "x" }</style><p>y</p>`)
	if len(before) != 1 || before[0].Code != "css/unsupported-selector" {
		t.Fatalf("::before diagnostics = %v", before.Lines())
	}
	wrongElement := validate(t, `<style>div::after { content: "x" }</style><div>y</div>`)
	if len(wrongElement) != 1 || wrongElement[0].Code != "css/unsupported-selector" {
		t.Fatalf("div::after diagnostics = %v", wrongElement.Lines())
	}
	wrongProp := validate(t, `<style>a::after { color: red }</style><a href="#x">y</a>`)
	if len(wrongProp) != 1 || wrongProp[0].Code != "css/unsupported-property" {
		t.Fatalf("a::after with color = %v", wrongProp.Lines())
	}
	contentOutside := validate(t, `<style>p { content: "x" }</style><p>y</p>`)
	if len(contentOutside) != 1 || contentOutside[0].Code != "css/unsupported-property" {
		t.Fatalf("content outside a::after = %v", contentOutside.Lines())
	}
}

func TestUnsupportedSelector(t *testing.T) {
	l := validate(t, `<style>tr:nth-child(2n) { color: red }</style><table><tr><td>x</td></tr></table>`)
	if len(l) != 1 || l[0].Code != "css/unsupported-selector" {
		t.Fatalf("diagnostics = %v", l.Lines())
	}
}

func TestUnknownAtRule(t *testing.T) {
	l := validate(t, `<style>@import url(x.css);</style><p>x</p>`)
	if len(l) != 1 || l[0].Code != "css/unsupported-at-rule" {
		t.Fatalf("diagnostics = %v", l.Lines())
	}
}

func TestProfileTextListsEverything(t *testing.T) {
	text := Text()
	for _, prop := range Properties() {
		if !strings.Contains(text, prop) {
			t.Fatalf("profile text does not mention property %q", prop)
		}
	}
	for _, el := range Elements() {
		if !strings.Contains(text, "  "+el+"\n") {
			t.Fatalf("profile text does not mention element %q", el)
		}
	}
}

func TestDiagnosticFormatting(t *testing.T) {
	l := validate(t, "<style>p { display: flex }</style><p>x</p>")
	if len(l) != 1 {
		t.Fatalf("want exactly one diagnostic, got %v", l.Lines())
	}
	line := l[0].String()
	if !strings.HasPrefix(line, "doc.html:1:") {
		t.Fatalf("diagnostic = %q, want it to start with the file and line", line)
	}
	if !strings.Contains(line, "error[css/unsupported-value]") {
		t.Fatalf("diagnostic = %q", line)
	}
	_ = fmt.Sprint(line)
}
