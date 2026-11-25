package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

// TestProfileDocMatchesParser guards RV2-25: docs/profile.md promised
// display: inline-block, which the validator rejects. README calls the table the
// complete list, so every value it lists must be accepted by the parser.
func TestProfileDocMatchesParser(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "profile.md"))
	if err != nil {
		t.Skipf("docs/profile.md is unavailable: %v", err)
	}
	values := docRowValues(t, string(data), "display")
	if len(values) == 0 {
		t.Fatal("the display row of docs/profile.md lists no values")
	}
	for _, value := range values {
		var st css.Style
		err := css.ApplyDecl(&st, "display", css.Tokenize([]byte(value)), 12000)
		if err != nil {
			t.Errorf("docs/profile.md lists display: %s, but the parser rejects it: %v", value, err)
		}
	}
	if strings.Contains(string(data), "`inline-block`") {
		t.Error("docs/profile.md still lists inline-block")
	}
}

// docRowValues returns the backticked values of the property table row.
func docRowValues(t *testing.T, doc, property string) []string {
	t.Helper()
	for _, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(line, "|") || !strings.Contains(line, "`"+property+"`") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			return nil
		}
		var out []string
		for _, part := range strings.Split(cells[3], "`") {
			part = strings.Trim(part, " ,")
			if part == "" {
				continue
			}
			out = append(out, part)
		}
		return out
	}
	return nil
}

// TestBreakAlwaysIsDocumented guards RV4-61: the parser accepted `always` as an
// alias of `page`, but the property table omitted it, so a documented-value
// check would reject a template that works.
func TestBreakAlwaysIsDocumented(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "profile.md"))
	if err != nil {
		t.Skipf("docs/profile.md is unavailable: %v", err)
	}
	for _, property := range []string{"break-before", "break-after"} {
		values := docRowValues(t, string(data), property)
		found := false
		for _, value := range values {
			if value == "always" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s does not list always: %v", property, values)
		}
		var st css.Style
		if err := css.ApplyDecl(&st, property, css.Tokenize([]byte("always")), 12000); err != nil {
			t.Fatalf("%s no longer accepts always: %v", property, err)
		}
		if st.BreakBefore != css.BreakPage && st.BreakAfter != css.BreakPage {
			t.Fatalf("%s: always did not produce a page break: %+v", property, st)
		}
	}
}

// TestDiagnosticCatalogueMatchesTheCode guards RV4-25 and IMP-01: the catalogue
// in docs/profile.md is generated from the code's registry, and this test fails
// when the two disagree in either direction.
func TestDiagnosticCatalogueMatchesTheCode(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "profile.md"))
	if err != nil {
		t.Skipf("docs/profile.md is unavailable: %v", err)
	}
	doc := string(data)
	start := strings.Index(doc, "<!-- diagnostics:start -->")
	end := strings.Index(doc, "<!-- diagnostics:end -->")
	if start < 0 || end < start {
		t.Fatal("docs/profile.md has no diagnostics catalogue markers")
	}
	got := strings.TrimSpace(doc[start+len("<!-- diagnostics:start -->") : end])
	want := strings.TrimSpace(diag.Catalogue())
	if got != want {
		t.Fatalf("the catalogue is stale; regenerate it from internal/diag:\n--- documented ---\n%s\n--- registry ---\n%s", got, want)
	}
}

// TestDocumentedGeneratedContentValidates guards RV4-27: the profile document
// claimed forms the code rejects. Every form it now documents must validate,
// and every form it says is rejected must be rejected.
func TestDocumentedGeneratedContentValidates(t *testing.T) {
	accepted := `<style>
	a.toc::after { content: " →" target-counter(attr(href), page) }
	@page { @top-center { content: "Title" } }
	@page { @bottom-center { content: "Page " counter(page) " of " counter(pages) } }
	</style><p><a class="toc" href="#x">x</a></p>`
	doc := parseDoc(t, accepted)
	if diags := validateDoc(t, doc); diags.HasErrors() {
		t.Fatalf("the documented generated content does not validate: %v", diags.Lines())
	}
	// The document says a page counter inside a::after and a target-counter in
	// a margin box are rejected.
	for _, src := range []string{
		`<style>a::after { content: counter(page) }</style><p><a href="#x">x</a></p>`,
		`<style>@page { @top-center { content: target-counter(attr(href), page) } }</style><p><a href="#x">x</a></p>`,
	} {
		diags := validateDoc(t, parseDoc(t, src))
		if !diags.HasErrors() {
			t.Fatalf("a rejected form was accepted: %s", src)
		}
	}
}

// validateDoc runs profile validation on a parsed document.
func validateDoc(t *testing.T, doc *html.Document) diag.List {
	t.Helper()
	sheets, sheetDiags := CollectStylesheets(doc, Options{})
	out := ValidateSheets(doc, Options{}, sheets, sheetDiags)
	out.Sort()
	return out
}
