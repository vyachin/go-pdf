package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
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
	if diags := validateDoc(t, doc, Options{}); diags.HasErrors() {
		t.Fatalf("the documented generated content does not validate: %v", diags.Lines())
	}
	// The document says a page counter inside a::after and a target-counter in
	// a margin box are rejected.
	for _, src := range []string{
		`<style>a::after { content: counter(page) }</style><p><a href="#x">x</a></p>`,
		`<style>@page { @top-center { content: target-counter(attr(href), page) } }</style><p><a href="#x">x</a></p>`,
	} {
		diags := validateDoc(t, parseDoc(t, src), Options{})
		if !diags.HasErrors() {
			t.Fatalf("a rejected form was accepted: %s", src)
		}
	}
}

// TestProfileTablesMatchTheCode guards RV5-47: docs/profile.md claims to be the
// complete list, so its element block, attribute table and inherited column
// must match the code that validates them. Every failure names the drifted row.
func TestProfileTablesMatchTheCode(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "profile.md"))
	if err != nil {
		t.Skipf("docs/profile.md is unavailable: %v", err)
	}
	doc := string(data)
	checkDocumentedElements(t, doc)
	checkDocumentedAttributes(t, doc)
	checkDocumentedInheritance(t, doc)
	checkDocumentedLineHeight(t, doc)
	checkDocumentedMedia(t, doc)
}

// checkDocumentedElements compares the element code block with the allow-list.
func checkDocumentedElements(t *testing.T, doc string) {
	t.Helper()
	section := docSection(t, doc, "Elements")
	open := strings.Index(section, "```")
	if open < 0 {
		t.Fatal("the Elements section has no fenced code block")
	}
	rest := section[open+len("```"):]
	closeFence := strings.Index(rest, "```")
	if closeFence < 0 {
		t.Fatal("the Elements code block is not closed")
	}
	documented := map[string]bool{}
	for _, name := range strings.Fields(rest[:closeFence]) {
		if documented[name] {
			t.Errorf("element row %q is documented twice", name)
		}
		documented[name] = true
	}
	if len(documented) == 0 {
		t.Fatal("the Elements code block is empty")
	}
	for name := range documented {
		if !css.SupportedElements[name] {
			t.Errorf("element row %q: documented but not in css.SupportedElements", name)
		}
	}
	for name := range css.SupportedElements {
		if !documented[name] {
			t.Errorf("element row %q: in css.SupportedElements but not documented", name)
		}
	}
}

// checkDocumentedAttributes compares the attribute table with the global and
// per-element allow-lists.
func checkDocumentedAttributes(t *testing.T, doc string) {
	t.Helper()
	rows := markdownTableRows(docSection(t, doc, "Elements"), "| Scope | Attributes |")
	if len(rows) == 0 {
		t.Fatal("the attribute table is missing")
	}
	documentedGlobal := map[string]bool{}
	documentedElement := map[string]map[string]bool{}
	for _, cells := range rows {
		if len(cells) < 2 {
			continue
		}
		scope := strings.TrimSpace(cells[0])
		attrs := docBackticked(stripParenthetical(cells[1]))
		if scope == "every element" {
			for _, name := range attrs {
				documentedGlobal[name] = true
			}
			continue
		}
		for _, element := range docBackticked(scope) {
			if _, ok := documentedElement[element]; ok {
				t.Errorf("attribute row %q is documented twice", element)
			}
			documentedElement[element] = setOf(attrs)
		}
	}
	compareDocSet(t, "every element", documentedGlobal, allowedAttributes)
	for element, want := range elementAttributes {
		got, ok := documentedElement[element]
		if !ok {
			t.Errorf("attribute row %q: the code accepts attributes but the document has no row", element)
			continue
		}
		compareDocSet(t, element, got, want)
	}
	for element := range documentedElement {
		if _, ok := elementAttributes[element]; !ok {
			t.Errorf("attribute row %q: documented but the code accepts no attributes for it", element)
		}
	}
	if !documentedElement["style"]["media"] {
		t.Error("attribute row \"style\": the document must list media")
	}
}

// checkDocumentedInheritance compares the Inherited column with css.Table in
// both directions.
func checkDocumentedInheritance(t *testing.T, doc string) {
	t.Helper()
	rows := markdownTableRows(docSection(t, doc, "Properties"), "| Property | Inherited | Values |")
	if len(rows) == 0 {
		t.Fatal("the property table is missing")
	}
	documented := map[string]bool{}
	for _, cells := range rows {
		if len(cells) < 2 {
			continue
		}
		row := strings.TrimSpace(cells[0])
		inherited := strings.TrimSpace(cells[1])
		if inherited != "yes" && inherited != "no" {
			t.Errorf("property row %q: inherited column is %q, want yes or no", row, inherited)
			continue
		}
		for _, name := range expandPropertyNames(docBackticked(cells[0])) {
			if _, ok := documented[name]; ok {
				t.Errorf("property row %q is documented twice", name)
			}
			documented[name] = inherited == "yes"
		}
	}
	for name, got := range documented {
		spec, ok := css.Table[name]
		if !ok {
			t.Errorf("property row %q: documented but not in css.Table", name)
			continue
		}
		if spec.Inherited != got {
			t.Errorf("property row %q: documented inherited=%t, but css.Table says %t", name, got, spec.Inherited)
		}
	}
	for name := range css.Table {
		if _, ok := documented[name]; !ok {
			t.Errorf("property row %q: in css.Table but not documented", name)
		}
	}
}

// checkDocumentedLineHeight compares the line-height value list with the
// parser.
func checkDocumentedLineHeight(t *testing.T, doc string) {
	t.Helper()
	rows := markdownTableRows(docSection(t, doc, "Properties"), "| Property | Inherited | Values |")
	for _, cells := range rows {
		if len(cells) < 3 || !containsString(docBackticked(cells[0]), "line-height") {
			continue
		}
		values := strings.ToLower(strings.Join(cells[2:], "|"))
		for _, want := range []string{"number", "length", "percentage", "normal"} {
			if !strings.Contains(values, want) {
				t.Errorf("property row %q: the value list does not mention %s: %q", "line-height", want, values)
			}
		}
		for _, sample := range []string{"1.5", "12pt", "120%", "normal"} {
			var st css.Style
			if err := css.ApplyDecl(&st, "line-height", css.Tokenize([]byte(sample)), 12000); err != nil {
				t.Errorf("property row %q: the parser rejects the documented form %q: %v", "line-height", sample, err)
			}
		}
		return
	}
	t.Fatal("the property table has no line-height row")
}

// checkDocumentedMedia compares the documented media rule with
// css.MediaAppliesToPrint.
func checkDocumentedMedia(t *testing.T, doc string) {
	t.Helper()
	bullet := ""
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "- `media` on `<link>` and `<style>`") {
			bullet = line
			break
		}
	}
	if bullet == "" {
		t.Fatal("docs/profile.md has no media bullet in the External stylesheets section")
	}
	lower := strings.ToLower(bullet)
	for _, phrase := range []string{"whole", "`only print`", "skipped", "`print`", "`all`"} {
		if !strings.Contains(lower, phrase) {
			t.Errorf("the media bullet does not state %q: %s", phrase, bullet)
		}
	}
	cases := []struct {
		list string
		want bool
	}{
		{"", true},
		{"print", true},
		{"all", true},
		{"print, screen", true},
		{"only print", false},
		{"screen", false},
		{"print and (min-width: 1px)", false},
	}
	for _, tc := range cases {
		if got := css.MediaAppliesToPrint(tc.list); got != tc.want {
			t.Errorf("MediaAppliesToPrint(%q) = %t, want %t; the document states whole-item matching", tc.list, got, tc.want)
		}
	}
}

// docSection returns the body of a level-two section, up to the next one.
func docSection(t *testing.T, doc, heading string) string {
	t.Helper()
	marker := "## " + heading + "\n"
	start := strings.Index(doc, marker)
	if start < 0 {
		t.Fatalf("docs/profile.md has no %q section", "## "+heading)
	}
	rest := doc[start+len(marker):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// markdownTableRows returns the rows of the table that starts with header. The
// table ends at the first line that is not a row.
func markdownTableRows(section, header string) [][]string {
	lines := strings.Split(section, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	var rows [][]string
	for _, line := range lines[start:] {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := tableCells(line)
		if isTableSeparator(cells) {
			continue
		}
		rows = append(rows, cells)
	}
	return rows
}

// tableCells splits one markdown table row into its cells.
func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	return strings.Split(line, "|")
}

// isTableSeparator reports whether a row is the |---|---| separator.
func isTableSeparator(cells []string) bool {
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		if strings.Trim(cell, ":-") != "" {
			return false
		}
	}
	return true
}

// docBackticked returns the backticked spans of s in order.
func docBackticked(s string) []string {
	var out []string
	for {
		open := strings.Index(s, "`")
		if open < 0 {
			return out
		}
		rest := s[open+1:]
		closeSpan := strings.Index(rest, "`")
		if closeSpan < 0 {
			return out
		}
		out = append(out, rest[:closeSpan])
		s = rest[closeSpan+1:]
	}
}

// stripParenthetical removes the parenthetical annotations in a table cell, so
// "`href` (a local `#id` link)" yields only the attribute name.
func stripParenthetical(s string) string {
	for {
		open := strings.Index(s, "(")
		if open < 0 {
			return s
		}
		closeParen := strings.Index(s[open:], ")")
		if closeParen < 0 {
			return s[:open]
		}
		s = s[:open] + s[open+closeParen+1:]
	}
}

// expandPropertyNames expands the slash shorthand of a row such as
// "page-break-before/after/inside".
func expandPropertyNames(names []string) []string {
	var out []string
	for _, name := range names {
		parts := strings.Split(name, "/")
		out = append(out, parts[0])
		if len(parts) == 1 {
			continue
		}
		prefix := parts[0]
		if i := strings.LastIndex(prefix, "-"); i >= 0 {
			prefix = prefix[:i+1]
		} else {
			prefix = ""
		}
		for _, suffix := range parts[1:] {
			out = append(out, prefix+suffix)
		}
	}
	return out
}

// setOf turns a name list into a set.
func setOf(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

// compareDocSet reports every name present on one side only.
func compareDocSet(t *testing.T, row string, got, want map[string]bool) {
	t.Helper()
	for name := range got {
		if !want[name] {
			t.Errorf("row %q: documents %q, but the code does not accept it", row, name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("row %q: the code accepts %q, but the document does not list it", row, name)
		}
	}
}

// containsString reports whether names contains want.
func containsString(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
