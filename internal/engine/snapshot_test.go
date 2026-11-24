package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/docdump"
	"github.com/vyachin/go-pdf/internal/layout"
)

// corpusFixtures are the four real documents the suite has always converted.
var corpusFixtures = []string{"invoice.html", "act.html", "contract.html", "bigtable.html"}

type snapshotFixture struct {
	name    string
	path    string
	baseDir string
}

// snapshotFixtures returns the document snapshot set: the corpus and every
// profile fixture.
func snapshotFixtures(t *testing.T) []snapshotFixture {
	t.Helper()
	root := repoRoot()
	var out []snapshotFixture
	for _, name := range corpusFixtures {
		out = append(out, snapshotFixture{
			name:    strings.TrimSuffix(name, ".html"),
			path:    filepath.Join(root, "testdata", name),
			baseDir: filepath.Join(root, "testdata"),
		})
	}
	paths, err := filepath.Glob(filepath.Join(root, "testdata", "fixtures", "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		out = append(out, snapshotFixture{
			name:    strings.TrimSuffix(filepath.Base(path), ".html"),
			path:    path,
			baseDir: filepath.Join(root, "testdata", "fixtures"),
		})
	}
	return out
}

func repoRoot() string {
	return filepath.Join("..", "..")
}

func snapshotGoldenDir() string {
	return filepath.Join(repoRoot(), "testdata", "golden", "xml")
}

// prepareSnapshot runs the engine's preparation on a fixture and returns the
// model ready for a dump. A fixture that fails validation fails the test.
func prepareSnapshot(t *testing.T, f snapshotFixture) (*prepared, docdump.Input) {
	t.Helper()
	src, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		File:    filepath.Base(f.path),
		BaseDir: f.baseDir,
		FontDir: filepath.Join(repoRoot(), "testdata", "fonts"),
	}
	p, diags, err := prepare(context.Background(), src, opts)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if p == nil {
		t.Fatalf("fixture is not valid:\n%s", strings.Join(diags.Lines(), "\n"))
	}
	return p, docdump.Input{
		Document: p.doc,
		Cascade:  p.cascade,
		Sheets:   p.sheets,
		Size:     p.pageStyle.Size,
		Margin:   p.pageStyle.Margin,
		Boxes:    p.pageStyle.Boxes,
		Diags:    diags,
	}
}

// TestDocumentSnapshots freezes the parsed and cascaded document model.
func TestDocumentSnapshots(t *testing.T) {
	dir := snapshotGoldenDir()
	if *updateGolden {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fixtures := snapshotFixtures(t)
	seen := map[string]bool{}
	for _, f := range fixtures {
		f := f
		t.Run(f.name, func(t *testing.T) {
			seen[f.name] = true
			_, in := prepareSnapshot(t, f)
			got := docdump.XML(in)
			path := filepath.Join(dir, f.name+".xml")
			if *updateGolden {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing snapshot %s\nrun go test ./internal/engine -run TestDocumentSnapshots -update", path)
			}
			if bytes.Equal(want, got) {
				return
			}
			actual := filepath.Join(artifactDir(t), f.name+".actual.xml")
			if err := os.WriteFile(actual, got, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Fatalf("document snapshot differs from %s\nproduced: %s\n%s", path, actual, firstDiff(want, got, 8))
		})
	}
	// A filtered run (-run TestDocumentSnapshots/invoice) processes a subset;
	// the other goldens are not stale, they were simply not visited.
	if *updateGolden || len(seen) != len(fixtures) {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".xml")
		if strings.HasSuffix(e.Name(), ".xml") && !seen[name] {
			t.Errorf("stale snapshot without a fixture: %s", filepath.Join(dir, e.Name()))
		}
	}
}

// TestDocumentSnapshotStylesMatchBoxes keeps the serializer honest: for every
// element that produces a box, the style in the dump must be the style layout
// uses.
func TestDocumentSnapshotStylesMatchBoxes(t *testing.T) {
	for _, f := range snapshotFixtures(t) {
		f := f
		t.Run(f.name, func(t *testing.T) {
			p, in := prepareSnapshot(t, f)
			styles := docdump.Compute(in)
			images := &imageLoader{baseDir: f.baseDir}
			defer func() { _ = images.Close() }()
			root, buildDiags := layout.Build(p.doc, layout.Context{
				Cascade:   p.cascade,
				Images:    images,
				PageSize:  p.pageStyle.Size,
				Margin:    p.pageStyle.Margin,
				PageStyle: p.pageStyle,
			})
			if buildDiags.HasErrors() {
				t.Fatalf("build failed: %s", strings.Join(buildDiags.Lines(), "\n"))
			}
			checked := 0
			walkBoxes(root, func(box *layout.Box) {
				// Text boxes carry the parsed text node, which has no style of
				// its own; only element boxes are compared.
				if box.Node == nil || box.Node.Tag == "" || box.Style == nil {
					return
				}
				got := styles[box.Node]
				if got == nil {
					t.Fatalf("no snapshot style for <%s>", box.Node.Tag)
				}
				// Layout propagates border-collapse to cells for painting, so
				// that one field intentionally differs from the cascade.
				a, b := *got, *box.Style
				b.BorderCollapse = a.BorderCollapse
				if !reflect.DeepEqual(&a, &b) {
					t.Fatalf("style mismatch for <%s>: snapshot %+v, box %+v", box.Node.Tag, a, b)
				}
				checked++
			})
			if checked == 0 {
				t.Fatal("no element boxes were checked")
			}
		})
	}
}

func walkBoxes(b *layout.Box, fn func(*layout.Box)) {
	fn(b)
	for _, c := range b.Children {
		walkBoxes(c, fn)
	}
	for _, row := range b.Rows {
		for _, cell := range row.Cells {
			walkBoxes(cell, fn)
		}
	}
}

// artifactDir returns the directory for failure artifacts: GOLDEN_ARTIFACTS
// when CI sets it, otherwise the test's temporary directory.
func artifactDir(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("GOLDEN_ARTIFACTS"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	return t.TempDir()
}

// firstDiff reports the first differing lines of two files, with line numbers.
func firstDiff(want, got []byte, limit int) string {
	w := strings.Split(string(want), "\n")
	g := strings.Split(string(got), "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "first differing lines (want %d lines, got %d):", len(w), len(g))
	shown := 0
	for i := 0; i < len(w) || i < len(g); i++ {
		if i < len(w) && i < len(g) && w[i] == g[i] {
			continue
		}
		wt, gt := "<missing>", "<missing>"
		if i < len(w) {
			wt = w[i]
		}
		if i < len(g) {
			gt = g[i]
		}
		fmt.Fprintf(&b, "\n:%d want %q\n:%d  got %q", i+1, wt, i+1, gt)
		shown++
		if shown >= limit {
			break
		}
	}
	return b.String()
}

// TestProfileCoverage fails when a supported property or element is not
// exercised by any snapshot fixture. A golden that never runs a feature cannot
// protect it.
func TestProfileCoverage(t *testing.T) {
	var text strings.Builder
	for _, pattern := range []string{
		filepath.Join(repoRoot(), "testdata", "*.html"),
		filepath.Join(repoRoot(), "testdata", "fixtures", "*.html"),
		filepath.Join(repoRoot(), "testdata", "fixtures", "*.css"),
	} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text.Write(data)
			text.WriteString("\n")
		}
	}
	fixtures := text.String()
	if strings.TrimSpace(fixtures) == "" {
		t.Fatal("no fixtures found")
	}
	for _, prop := range css.PropOrder {
		decl := regexp.MustCompile(`(^|[\s{;])` + regexp.QuoteMeta(prop) + `\s*:`)
		if !decl.MatchString(fixtures) {
			t.Errorf("profile property %q has no fixture that declares it", prop)
		}
	}
	for _, element := range css.ElementOrder() {
		tag := regexp.MustCompile(`<` + regexp.QuoteMeta(element) + `([\s/>])`)
		if !tag.MatchString(fixtures) {
			t.Errorf("profile element <%s> has no fixture", element)
		}
	}
}
