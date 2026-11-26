package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// Pixel snapshots can preserve an accidental missing glyph or blank page.
// Check the fixtures' content independently before accepting their goldens.
func TestVisualFixtureContracts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages int
		marks []string
	}{
		{"visual-inline-flow", 1, []string{"Before", "READY", "after.", "Underlined", "bold", "removed", "10\u00a0000", "forced line break.", "first line", "second line"}},
		{"visual-nested-lists", 1, []string{"8.", "9.", "10.", "3.", "4.", "Outer eight", "Inner three", "Inner four", "Outer nine", "Outer ten", "Bullet one", "Bullet two"}},
		{"visual-table-pagination", 2, []string{"ROW-01", "ROW-02", "ROW-03", "ROW-04", "ROW-05", "ROW-06", "ROW-07", "ROW-08", "TOTAL", "360"}},
		{"visual-page-counters", 3, []string{"CONTENTS", "Alpha ... 2", "Beta ... 3", "ALPHA", "BETA", "First section.", "Second section."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(repoRoot(), "testdata", "fixtures")
			src, err := os.ReadFile(filepath.Join(dir, tc.name+".html"))
			if err != nil {
				t.Fatal(err)
			}
			res, err := Convert(context.Background(), src, Options{
				FallbackFont: bundledFont(t), File: tc.name + ".html", BaseDir: dir,
			})
			if err != nil || len(res.Diagnostics) != 0 {
				t.Fatalf("Convert = %v; diagnostics: %v", err, res.Diagnostics.Lines())
			}
			validateWithPDFCPU(t, res.PDF)
			pages, err := pdfdebug.PageTexts(res.PDF)
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) != tc.pages {
				t.Fatalf("got %d pages, want %d: %q", len(pages), tc.pages, pages)
			}
			all := strings.Join(pages, "\n")
			for _, mark := range tc.marks {
				if strings.Count(all, mark) != 1 {
					t.Errorf("want exactly one %q in %q", mark, all)
				}
			}
			if strings.Contains(all, "SECRET") {
				t.Error("hidden content reached the PDF")
			}
			for i, page := range pages {
				switch tc.name {
				case "visual-table-pagination":
					if strings.Count(page, "Item") != 1 || strings.Count(page, "Amount") != 1 {
						t.Errorf("page %d lost or duplicated its table header: %q", i+1, page)
					}
				case "visual-page-counters":
					if !strings.Contains(page, fmt.Sprintf("Page %d of 3", i+1)) || strings.Count(page, "REPORT") != 1 {
						t.Errorf("page %d has incorrect margin counters: %q", i+1, page)
					}
				}
			}
			// Pin the complete per-page text too: moving a row, section or
			// margin box to another page must produce a reviewable text diff.
			got := []byte(strings.Join(pages, "\n--- page ---\n") + "\n")
			path := filepath.Join(repoRoot(), "testdata", "golden", tc.name+".txt")
			if *updateGolden {
				if t.Failed() {
					return
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing text golden: %v; run go test ./internal/engine -run TestVisualFixtureContracts -update", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("page text differs from %s\n%s", path, firstDiff(want, got, 8))
			}
		})
	}
}
