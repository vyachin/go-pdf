package profile

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

func validate(t *testing.T, src string) diag.List {
	t.Helper()
	return validateDoc(t, parseDoc(t, src), Options{})
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
	badMargin := validate(t, `<style>@page { margin: auto }</style><p>x</p>`)
	if len(badMargin) != 1 || badMargin[0].Code != "css/unsupported-value" {
		t.Fatalf("page margin diagnostics = %v", badMargin.Lines())
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

// TestInlineStyleDeclarationList guards RV5-06 at validation: a brace or an
// at-rule in a style attribute is reported with the attribute's position, the
// declarations around the offending token are still validated, and a
// well-formed attribute stays clean.
func TestInlineStyleDeclarationList(t *testing.T) {
	cases := []struct {
		name  string
		style string
		want  []string
	}{
		{"leading brace", `} display:flex`, []string{"css/syntax", "css/unsupported-value"}},
		{"middle brace", `color:red} display:flex`, []string{"css/syntax", "css/unsupported-value"}},
		{"opening brace", `color:red{display:flex`, []string{"css/syntax", "css/unsupported-value"}},
		{"media rule", `@media print { p { display: flex } }`, []string{"css/unsupported-at-rule"}},
		{"page rule", `@page { size: A4 }`, []string{"css/unsupported-at-rule"}},
		{"well-formed", `color:#c00;font-size:10pt`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<html><body>\n  <p style=\"" + tc.style + "\">x</p>\n</body></html>"
			l := validate(t, src)
			var codes []string
			for _, d := range l {
				codes = append(codes, d.Code)
			}
			if strings.Join(codes, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("diagnostics = %v, want codes %v", l.Lines(), tc.want)
			}
			for _, d := range l {
				if d.Pos.File != "doc.html" || d.Pos.Line != 2 || d.Pos.Col != 3 {
					t.Fatalf("diagnostic position = %+v, want doc.html:2:3", d.Pos)
				}
				if !strings.Contains(d.Element, "p") {
					t.Fatalf("diagnostic does not name the element path: %q", d.Element)
				}
				switch d.Code {
				case "css/syntax":
					if !strings.ContainsAny(d.Message, "{}") {
						t.Fatalf("the syntax problem does not name the brace: %q", d.Message)
					}
				case "css/unsupported-value":
					if !strings.Contains(d.Message, "display") {
						t.Fatalf("the value problem does not name the declaration: %q", d.Message)
					}
				case "css/unsupported-at-rule":
					if !strings.Contains(d.Message, "@") {
						t.Fatalf("the at-rule problem does not name the at-rule: %q", d.Message)
					}
				}
			}
		})
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

func TestPageOrientationValidation(t *testing.T) {
	ok := validate(t, `<style>@page { size: A4 landscape }</style><p>x</p>`)
	if ok.HasErrors() {
		t.Fatalf("A4 landscape must be accepted: %v", ok.Lines())
	}
	for _, value := range []string{"A4 sideways", "210mm 297mm landscape", "A4 A5"} {
		bad := validate(t, `<style>@page { size: `+value+` }</style><p>x</p>`)
		if len(bad) != 1 || bad[0].Code != "css/unsupported-value" {
			t.Fatalf("size %q diagnostics = %v", value, bad.Lines())
		}
	}
}

func TestExternalStylesheetPositions(t *testing.T) {
	src := []byte("p {\n  display: flex;\n}\n")
	sheet := css.ParseSheet(css.SheetOptions{File: "styles/print.css"}, src)
	doc, err := html.Parse([]byte("<p>x</p>"), "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	var out diag.List
	validateSheet(doc, sheet, &out)
	if len(out) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %v", len(out), out.Lines())
	}
	if out[0].Pos.File != "styles/print.css" || out[0].Pos.Line != 2 || out[0].Pos.Col <= 0 {
		t.Fatalf("diagnostic position = %+v, want styles/print.css:2:<col>", out[0].Pos)
	}
	if out[0].Code != "css/unsupported-value" {
		t.Fatalf("code = %q", out[0].Code)
	}
}

func TestScriptIsIgnoredWithWarning(t *testing.T) {
	l := validate(t, `<script>console.log("x")</script><p>текст</p>`)
	if l.HasErrors() {
		t.Fatalf("a script must not fail validation: %v", l.Lines())
	}
	if len(l) != 1 || l[0].Code != "html/ignored-script" || l[0].Severity != diag.Warning {
		t.Fatalf("diagnostics = %v", l.Lines())
	}
	strict := validateDoc(t, parseDoc(t, `<script>console.log("x")</script><p>x</p>`), Options{Strict: true})
	if !strict.HasErrors() {
		t.Fatalf("strict mode must turn the warning into an error: %v", strict.Lines())
	}
	if strict[0].Code != "html/ignored-script" {
		t.Fatalf("code = %q", strict[0].Code)
	}
}

func TestGeneratorsAreRestrictedToTheirContext(t *testing.T) {
	doc := parseDoc(t, `<p><a href="#x">A</a></p>`)
	sheet := css.ParseSheet(css.SheetOptions{}, []byte(`a::after { content: " page=" counter(page) }`))
	diags := validateSheetDiags(t, doc, sheet)
	if !diags.HasErrors() {
		t.Fatal("counter(page) in a::after must be rejected")
	}

	doc = parseDoc(t, `<p>x</p>`)
	sheet = css.ParseSheet(css.SheetOptions{}, []byte(`@page { @bottom-center { content: target-counter(attr(href), page) } }`))
	diags = validateSheetDiags(t, doc, sheet)
	if !diags.HasErrors() {
		t.Fatal("target-counter in a margin box must be rejected")
	}

	// The supported combinations stay valid.
	sheet = css.ParseSheet(css.SheetOptions{}, []byte(`a::after { content: target-counter(attr(href), page) }`))
	if diags := validateSheetDiags(t, doc, sheet); diags.HasErrors() {
		t.Fatalf("a::after target-counter: %v", diags.Lines())
	}
	sheet = css.ParseSheet(css.SheetOptions{}, []byte(`@page { @bottom-center { content: "p " counter(page) } }`))
	if diags := validateSheetDiags(t, doc, sheet); diags.HasErrors() {
		t.Fatalf("margin box counter: %v", diags.Lines())
	}
}

func validateSheetDiags(t *testing.T, doc *html.Document, sheet *css.Stylesheet) diag.List {
	t.Helper()
	return ValidateSheets(doc, Options{}, []*css.Stylesheet{sheet}, nil)
}
