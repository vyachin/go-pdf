package engine

import (
	"fmt"
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

// TestDocumentedCIPinMatchesTheWorkflow guards the same rule for the CI image
// and the renderer record: the workflow, the documentation and renderer.txt
// name one image and one renderer.
func TestDocumentedCIPinMatchesTheWorkflow(t *testing.T) {
	workflow := readDoc(t, ".github", "workflows", "ci.yml")
	image := regexp.MustCompile(`runs-on: (ubuntu-[0-9.]+)`).FindStringSubmatch(workflow)
	if image == nil {
		t.Fatal("ci.yml names no pinned image")
	}
	pinned := image[1]
	if strings.Contains(workflow, "ubuntu-latest") {
		t.Fatalf("ci.yml still uses ubuntu-latest")
	}
	renderer := strings.TrimSpace(readDoc(t, "testdata", "golden", "render", "renderer.txt"))
	if !strings.HasPrefix(renderer, "pdftoppm version ") {
		t.Fatalf("renderer.txt does not record the rasterizer: %q", renderer)
	}
	golden := readDoc(t, "docs", "golden.md")
	if !strings.Contains(golden, pinned) && !strings.Contains(golden, strings.TrimPrefix(pinned, "ubuntu-")) {
		t.Errorf("docs/golden.md does not name the pinned image %q", pinned)
	}
	contributing := readDoc(t, "CONTRIBUTING.md")
	if !strings.Contains(contributing, pinned) {
		t.Errorf("CONTRIBUTING.md does not name the pinned image %q", pinned)
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
