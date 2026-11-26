package engine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// readDoc returns one repository document.
func readDoc(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{repoRoot()}, parts...)...))
	if err != nil {
		t.Skipf("%v is unavailable", err)
	}
	return string(data)
}

// TestDocumentedLimitsMatchTheCode guards IMP-06: the limits a document states
// are checked against engine.DefaultLimits, so changing a default fails the
// documentation test until the document follows.
func TestDocumentedLimitsMatchTheCode(t *testing.T) {
	limits := DefaultLimits()
	doc := readDoc(t, "docs", "profile.md")
	checks := []struct {
		name  string
		value int
		doc   []string
	}{
		{"MaxOperations", limits.MaxOperations, []string{formatThousands(limits.MaxOperations) + " operations"}},
		{"MaxDOMDepth", limits.MaxDOMDepth, []string{fmt.Sprintf("(%d by default)", limits.MaxDOMDepth)}},
		{"MaxImagePixels", limits.MaxImagePixels, []string{formatThousands(limits.MaxImagePixels)}},
		{"MaxTotalImageBytes", limits.MaxTotalImageBytes, []string{mib(limits.MaxTotalImageBytes)}},
		{"MaxTotalImagePixels", limits.MaxTotalImagePixels, []string{formatThousands(limits.MaxTotalImagePixels)}},
		{"MaxTotalFontBytes", limits.MaxTotalFontBytes, []string{mib(limits.MaxTotalFontBytes)}},
	}
	for _, c := range checks {
		found := false
		for _, want := range c.doc {
			if strings.Contains(doc, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("docs/profile.md does not state %s = %d (looked for %v)", c.name, c.value, c.doc)
		}
	}
}

// TestDocumentedCIImageMatchesTheWorkflow guards the CI image rule: every job
// runs on one label, the documentation names that label, and renderer.txt
// records the rasterizer those goldens came from.
func TestDocumentedCIImageMatchesTheWorkflow(t *testing.T) {
	workflow := readDoc(t, ".github", "workflows", "ci.yml")
	labels := regexp.MustCompile(`runs-on: (\S+)`).FindAllStringSubmatch(workflow, -1)
	if len(labels) == 0 {
		t.Fatal("ci.yml names no runner image")
	}
	label := labels[0][1]
	for _, m := range labels[1:] {
		if m[1] != label {
			t.Fatalf("ci.yml mixes runner images: %q and %q", label, m[1])
		}
	}
	renderer := strings.TrimSpace(readDoc(t, "testdata", "golden", "render", "renderer.txt"))
	if !strings.HasPrefix(renderer, "pdftoppm version ") {
		t.Fatalf("renderer.txt does not record the rasterizer: %q", renderer)
	}
	golden := readDoc(t, "docs", "golden.md")
	if !strings.Contains(golden, label) {
		t.Errorf("docs/golden.md does not name the runner image %q", label)
	}
	contributing := readDoc(t, "CONTRIBUTING.md")
	if !strings.Contains(contributing, label) {
		t.Errorf("CONTRIBUTING.md does not name the runner image %q", label)
	}
}

// TestMirroredFieldDocs guards RV5-52: a field mirrored from the public pdfgen
// package states the semantics of the field it mirrors. The comment is read
// from the field's own doc through the parser, so removing a phrase fails here
// even if a similar one survives elsewhere in the file.
func TestMirroredFieldDocs(t *testing.T) {
	path := filepath.Join(repoRoot(), "internal", "engine", "engine.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	docs := make(map[string]string)
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				docs[spec.Name.Name+"."+name.Name] = field.Doc.Text()
			}
		}
		return true
	})
	checks := []struct {
		field   string
		phrases []string
	}{
		{"Limits.MaxTableCells", []string{"grid area", "column count times", "its row count"}},
		{"Options.RenderPNG", []string{"debug preview", "first page"}},
	}
	for _, check := range checks {
		doc, ok := docs[check.field]
		if !ok {
			t.Errorf("%s has no doc comment for %s", path, check.field)
			continue
		}
		for _, phrase := range check.phrases {
			if !strings.Contains(doc, phrase) {
				t.Errorf("%s does not say %q; the mirrored semantics must be stated", check.field, phrase)
			}
		}
	}
	// The stale sentence named the command line as a caller. Match the claim as
	// a pattern so that this guard does not itself keep the literal phrase in
	// the tree.
	if regexp.MustCompile(`CLI\s+debug`).Match(src) {
		t.Errorf("%s still claims a command-line flag drives the preview", path)
	}
}

// formatThousands renders an integer with thousands separators.
func formatThousands(v int) string {
	s := strconv.Itoa(v)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// mib renders a byte count as the "N MiB" the documentation uses.
func mib(v int) string {
	return strconv.Itoa(v/(1<<20)) + " MiB"
}
