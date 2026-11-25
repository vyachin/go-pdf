package weasycompat

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

var (
	update       = flag.Bool("update", false, "rewrite the page-text goldens of the cases this run processed")
	updateOracle = flag.Bool("update-oracle", false, "regenerate the committed oracle from the reference PDFs; needs pdftotext")
	updateHashes = flag.Bool("update-hashes", false, "rewrite the file hashes in samples.json from the corpus on disk")
)

// corpusDir returns the corpus directory.
func corpusDir() string {
	return filepath.Join("..", "..", "testdata", "weasyprint")
}

// artifacts returns the directory for failure artifacts: GOLDEN_ARTIFACTS when
// CI sets it, otherwise the test's temporary directory.
func artifacts(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("GOLDEN_ARTIFACTS"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	return t.TempDir()
}

func loadCorpus(t *testing.T) (string, *Manifest) {
	t.Helper()
	dir := corpusDir()
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := m.Lint(dir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := m.VerifyProvenance(dir); err != nil {
		t.Fatalf("%v", err)
	}
	return dir, m
}

// TestUpdateOracle regenerates oracle/<case>.json from the reference PDFs. It is
// the only path that runs pdftotext; a normal test run reads the committed JSON.
func TestUpdateOracle(t *testing.T) {
	if !*updateOracle {
		t.Skip("only with -update-oracle")
	}
	dir, m := loadCorpus(t)
	for _, c := range m.Cases {
		o, err := buildOracle(dir, m, c)
		if err != nil {
			t.Fatalf("building the oracle of %s: %v", c.Name, err)
		}
		if err := writeOracle(dir, o); err != nil {
			t.Fatalf("writing the oracle of %s: %v", c.Name, err)
		}
		t.Logf("%s: %d pages, %d tokens, %s", c.Name, o.PageCount, total(o.Tokens), o.Extractor)
	}
}

// TestUpdateHashes rewrites the committed hashes after the vendored files are
// replaced. It touches only the hashes: the cases and the source pin change by
// hand, and the diff of samples.json is what a reviewer reads.
func TestUpdateHashes(t *testing.T) {
	if !*updateHashes {
		t.Skip("only with -update-hashes")
	}
	dir := corpusDir()
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := m.scanFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := make([]File, 0, len(paths))
	for _, path := range paths {
		hash, err := hashFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, File{Path: path, SHA256: hash})
	}
	m.Files = files
	writeManifest(t, dir, m)
	t.Logf("rewrote %d file hashes", len(files))
}

// TestCorpus enforces the declared state of every case.
func TestCorpus(t *testing.T) {
	dir, m := loadCorpus(t)
	artifacts := artifacts(t)

	for _, c := range m.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Logf("case %s: state %s, blocker %q", c.Name, c.State, c.BlockedBy)
			first := runCase(dir, c)
			switch c.State {
			case StateBlocked:
				second := runCase(dir, c)
				if err := checkBlocked(c, first, second); err != nil {
					t.Fatal(err)
				}
			case StateMeasure, StateAccepted:
				if err := checkMeasures(t, dir, m, c, first, artifacts); err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatalf("case %s declares unknown state %q", c.Name, c.State)
			}
		})
	}

	t.Run("stale goldens", func(t *testing.T) {
		if err := checkStaleGoldens(dir, m); err != nil {
			t.Fatal(err)
		}
	})
}

// checkMeasures runs the assertions shared by a converting case: it produced a
// PDF, its page text matches the golden, and its distance from the reference is
// measured (and enforced only when the case is accepted).
func checkMeasures(t *testing.T, dir string, m *Manifest, c Case, first outcome, artifacts string) error {
	t.Helper()
	if first.panic != "" {
		return fmt.Errorf("case %s panicked:\n%s", c.Name, first.panic)
	}
	if first.err != nil {
		return fmt.Errorf("case %s is declared %s but was rejected: %v", c.Name, c.State, first.err)
	}
	pages, err := pdfdebug.PageTexts(first.pdf)
	if err != nil {
		return fmt.Errorf("case %s: reading back our own PDF: %w", c.Name, err)
	}
	if err := checkGolden(dir, artifacts, c, pages, *update); err != nil {
		return err
	}
	o, err := loadOracle(dir, m, c)
	if err != nil {
		return err
	}
	f := compareFidelity(o, pages)
	t.Logf("case %s: %s", c.Name, f)
	return checkFidelity(c, f)
}

// TestFontLicences proves every bundled font is covered by exactly one licence
// and that the notice names the excluded sample, so the corpus cannot ship a
// font whose terms nobody recorded.
func TestFontLicences(t *testing.T) {
	dir := corpusDir()

	var index struct {
		Fonts []struct {
			Family  string   `json:"family"`
			License string   `json:"license"`
			Files   []string `json:"files"`
		} `json:"fonts"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "licenses", "index.json"))
	if err != nil {
		t.Fatalf("reading the licence index: %v", err)
	}
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("parsing the licence index: %v", err)
	}

	covered := map[string]string{}
	for _, entry := range index.Fonts {
		if entry.Family == "" || entry.License == "" {
			t.Errorf("licence entry without a family or a licence: %+v", entry)
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "licenses", entry.License)); err != nil {
			t.Errorf("%s: licence text %s: %v", entry.Family, entry.License, err)
		}
		for _, f := range entry.Files {
			if owner, dup := covered[f]; dup {
				t.Errorf("%s is covered twice (%s and %s)", f, owner, entry.License)
				continue
			}
			covered[f] = entry.License
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
				t.Errorf("%s: listed but missing: %v", f, err)
			}
		}
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := fontFiles(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fonts {
		if covered[f] == "" {
			t.Errorf("font %s has no licence entry", f)
		}
	}
	for f := range covered {
		if !contains(fonts, f) {
			t.Errorf("licence index lists %s, which is not a font of the corpus", f)
		}
	}

	notice, err := os.ReadFile(filepath.Join(dir, "NOTICE.md"))
	if err != nil {
		t.Fatalf("reading NOTICE.md: %v", err)
	}
	for _, want := range []string{"book", m.Source.Commit, m.Source.Repository} {
		if !strings.Contains(string(notice), want) {
			t.Errorf("NOTICE.md does not mention %q", want)
		}
	}
}

// fontFiles returns every font file under the case directories.
func fontFiles(dir string, m *Manifest) ([]string, error) {
	all, err := m.scanFiles(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, path := range all {
		lower := strings.ToLower(path)
		if strings.HasSuffix(lower, ".ttf") || strings.HasSuffix(lower, ".otf") {
			out = append(out, path)
		}
	}
	return out, nil
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
