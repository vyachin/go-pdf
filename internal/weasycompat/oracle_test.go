package weasycompat

import (
	"os"
	"strings"
	"testing"
)

// oracleFixture writes a one-page reference PDF, its manifest entry and an
// oracle for it, and returns the corpus directory.
func oracleFixture(t *testing.T) (string, *Manifest, Case, *Oracle) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "sample/sample.pdf", "%PDF-1.7 reference\n")
	writeFile(t, dir, "sample/sample.html", "<p>alpha beta</p>\n")
	hash := hashBelow(t, dir, "sample/sample.pdf")
	m := &Manifest{
		Source: Source{Commit: "abc123"},
		Cases:  []Case{{Name: "sample", Dir: "sample", HTML: "sample.html", State: StateMeasure}},
		Files:  []File{{Path: "sample/sample.pdf", SHA256: hash}},
	}
	o := &Oracle{
		Case:            "sample",
		SourceCommit:    "abc123",
		ReferencePDF:    "sample/sample.pdf",
		ReferenceSHA256: hash,
		Extractor:       "pdftotext version test",
		PageCount:       1,
		Pages:           []string{"alpha beta"},
		Tokens:          map[string]int{"alpha": 1, "beta": 1},
	}
	if err := writeOracle(dir, o); err != nil {
		t.Fatal(err)
	}
	return dir, m, m.Cases[0], o
}

func TestLoadOracle(t *testing.T) {
	dir, m, c, want := oracleFixture(t)

	got, err := loadOracle(dir, m, c)
	if err != nil {
		t.Fatalf("loadOracle: %v", err)
	}
	if got.PageCount != want.PageCount || got.Extractor != want.Extractor {
		t.Fatalf("oracle = %+v, want %+v", got, want)
	}
	if f := compareFidelity(got, []string{"alpha beta"}); !f.ok() {
		t.Fatalf("fidelity = %v, want a match", f)
	}
}

func TestLoadOracleRejectsAnEditedReferencePDF(t *testing.T) {
	dir, m, c, _ := oracleFixture(t)
	writeFile(t, dir, "sample/sample.pdf", "%PDF-1.7 edited\n")

	_, err := loadOracle(dir, m, c)
	if err == nil {
		t.Fatal("an edited reference PDF was accepted")
	}
	if !strings.Contains(err.Error(), "samples.json says") {
		t.Fatalf("error %q does not name the manifest hash", err)
	}
}

func TestLoadOracleRejectsAStaleHash(t *testing.T) {
	dir, m, c, _ := oracleFixture(t)
	// A reference PDF that matches the manifest but not the oracle: the hash in
	// samples.json was regenerated without regenerating the oracle.
	writeFile(t, dir, "sample/sample.pdf", "%PDF-1.7 newer\n")
	hash := hashBelow(t, dir, "sample/sample.pdf")
	m.Files[0].SHA256 = hash

	_, err := loadOracle(dir, m, c)
	if err == nil {
		t.Fatal("an oracle older than its PDF was accepted")
	}
	if !strings.Contains(err.Error(), "-update-oracle") {
		t.Fatalf("error %q does not name the regeneration command", err)
	}
}

func TestLoadOracleRejectsAnInconsistentPageCount(t *testing.T) {
	dir, m, c, o := oracleFixture(t)
	o.PageCount = 3
	if err := writeOracle(dir, o); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOracle(dir, m, c); err == nil {
		t.Fatal("an oracle with a page_count that contradicts its pages was accepted")
	}
}

func TestCompareFidelity(t *testing.T) {
	o := &Oracle{PageCount: 2, Tokens: map[string]int{"alpha": 1, "beta": 2}}
	tests := []struct {
		name    string
		pages   []string
		ok      bool
		missing []string
		extra   []string
	}{
		{
			name:  "same page count and multiset",
			pages: []string{"alpha", "beta beta"},
			ok:    true,
		},
		{
			name:  "a page lost",
			pages: []string{"alpha beta beta"},
		},
		{
			name:    "one token lost",
			pages:   []string{"alpha", "beta"},
			missing: []string{"beta"},
		},
		{
			name:  "one token invented",
			pages: []string{"alpha", "beta beta gamma"},
			extra: []string{"gamma"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := compareFidelity(o, tt.pages)
			if f.ok() != tt.ok {
				t.Fatalf("ok = %v, want %v (%v)", f.ok(), tt.ok, f)
			}
			if len(tt.missing) > 0 && strings.Join(f.Missing, ",") != strings.Join(tt.missing, ",") {
				t.Fatalf("missing = %q, want %q", f.Missing, tt.missing)
			}
			if len(tt.extra) > 0 && strings.Join(f.Extra, ",") != strings.Join(tt.extra, ",") {
				t.Fatalf("extra = %q, want %q", f.Extra, tt.extra)
			}
		})
	}
}

func TestFidelityReportsPageCounts(t *testing.T) {
	o := &Oracle{PageCount: 8, Tokens: map[string]int{"a": 1}}
	f := compareFidelity(o, []string{"a", "b"})
	if !strings.Contains(f.String(), "pages 2/8") {
		t.Fatalf("log line = %q, want both page counts", f.String())
	}
}

func TestCheckFidelityEnforcesOnlyAcceptedCases(t *testing.T) {
	mismatch := fidelity{WantPages: 1, GotPages: 2, Missing: []string{"x"}}
	measure := Case{Name: "m", State: StateMeasure}
	accepted := Case{Name: "a", State: StateAccepted}

	if err := checkFidelity(measure, mismatch); err != nil {
		t.Fatalf("a measure case failed on a reviewed difference: %v", err)
	}
	if err := checkFidelity(accepted, mismatch); err == nil {
		t.Fatal("an accepted case passed with a mismatch")
	}
	if err := checkFidelity(accepted, fidelity{WantPages: 1, GotPages: 1}); err != nil {
		t.Fatalf("an accepted case that matches failed: %v", err)
	}
}

func TestBuildOracleRefusesAnEditedReferencePDF(t *testing.T) {
	dir, m, c, _ := oracleFixture(t)
	writeFile(t, dir, "sample/sample.pdf", "%PDF-1.7 edited\n")

	_, err := buildOracle(dir, m, c)
	if err == nil {
		t.Fatal("buildOracle wrote an oracle for an edited PDF")
	}
	if !strings.Contains(err.Error(), "refusing to write") {
		t.Fatalf("error %q does not explain the refusal", err)
	}
}

func TestSplitExtractedPages(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"one page", "alpha\f", []string{"alpha"}},
		{"two pages", "alpha\fbeta\f", []string{"alpha", "beta"}},
		{"blank last page", "alpha\f\f", []string{"alpha", ""}},
		{"no form feed", "alpha", []string{"alpha"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitExtractedPages(tt.in)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("splitExtractedPages(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWriteOracleIsStable(t *testing.T) {
	dir := t.TempDir()
	o := &Oracle{Case: "c", PageCount: 1, Pages: []string{"x"}, Tokens: map[string]int{"b": 2, "a": 1}}
	if err := writeOracle(dir, o); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(oraclePath(dir, "c"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeOracle(dir, o); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(oraclePath(dir, "c"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("the oracle is not byte-stable:\n%s\n%s", first, second)
	}
	if strings.Index(string(first), "\"a\"") > strings.Index(string(first), "\"b\"") {
		t.Fatalf("token keys are not sorted:\n%s", first)
	}
}
