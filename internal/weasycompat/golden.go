package weasycompat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// pageSeparator divides the pages of a committed page-text golden. It is the
// form feed a text extractor writes, so a golden reads like pdftotext output
// and a page boundary is a byte, not a convention.
const pageSeparator = "\f\n"

// goldenPath returns the committed golden of a case.
func goldenPath(dir, name string) string {
	return filepath.Join(dir, "golden", name+".txt")
}

// pageText joins the pages of one conversion into the golden format.
func pageText(pages []string) string {
	return strings.Join(pages, pageSeparator)
}

// checkGolden compares the produced page text with the case golden. With update
// it rewrites the golden of this case only, so a regeneration shows up as a
// diff of that file and of nothing else.
func checkGolden(dir, artifacts string, c Case, pages []string, update bool) error {
	produced := pageText(pages)
	path := goldenPath(dir, c.Name)
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating the golden directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(produced), 0o644); err != nil {
			return fmt.Errorf("writing the golden of %s: %w", c.Name, err)
		}
		return nil
	}
	want, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("case %s has no golden at %s; create it with:\n    go test ./internal/weasycompat -update", c.Name, path)
		}
		return fmt.Errorf("reading the golden of %s: %w", c.Name, err)
	}
	if string(want) == produced {
		return nil
	}
	artifact := filepath.Join(artifacts, c.Name+".txt")
	if err := os.MkdirAll(artifacts, 0o755); err == nil {
		_ = os.WriteFile(artifact, []byte(produced), 0o644)
	}
	return fmt.Errorf("case %s: page text differs\n%s\ngolden:   %s\nproduced: %s\nregenerate with:\n    go test ./internal/weasycompat -update",
		c.Name, describeDifference(string(want), produced), path, artifact)
}

// describeDifference names the first page and line that differ, so a review can
// read the mismatch without rerunning the suite.
func describeDifference(want, got string) string {
	wantPages := strings.Split(want, pageSeparator)
	gotPages := strings.Split(got, pageSeparator)
	if len(wantPages) != len(gotPages) {
		return fmt.Sprintf("page count: golden has %d, produced %d", len(wantPages), len(gotPages))
	}
	for page := range wantPages {
		if wantPages[page] == gotPages[page] {
			continue
		}
		wantLines := strings.Split(wantPages[page], "\n")
		gotLines := strings.Split(gotPages[page], "\n")
		for line := 0; line < len(wantLines) || line < len(gotLines); line++ {
			wantLine, gotLine := "<missing>", "<missing>"
			if line < len(wantLines) {
				wantLine = wantLines[line]
			}
			if line < len(gotLines) {
				gotLine = gotLines[line]
			}
			if wantLine == gotLine {
				continue
			}
			return fmt.Sprintf("page %d line %d differs:\n    golden:   %q\n    produced: %q", page+1, line+1, wantLine, gotLine)
		}
	}
	return "the texts differ in their page separators"
}

// checkStaleGoldens fails when a committed golden has no case: a golden that is
// never compared would rot silently after a case is renamed or deleted.
func checkStaleGoldens(dir string, m *Manifest) error {
	entries, err := os.ReadDir(filepath.Join(dir, "golden"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading the golden directory: %w", err)
	}
	known := make(map[string]bool, len(m.Cases))
	for _, c := range m.Cases {
		known[c.Name] = true
	}
	var stale []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		if name := strings.TrimSuffix(e.Name(), ".txt"); !known[name] {
			stale = append(stale, e.Name())
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		return fmt.Errorf("golden files without a case: %s; delete them or add the case to samples.json", strings.Join(stale, ", "))
	}
	return nil
}
