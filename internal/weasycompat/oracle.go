package weasycompat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Oracle is the committed fidelity reference of one case: what the PDF that
// WeasyPrint produced says, reduced to the two facts this engine can be held to
// — how many pages it has and which words appear, with their counts.
//
// The pages are kept as extracted, for review; the comparison uses Tokens, the
// normalised multiset. Storing both keeps the oracle inspectable without making
// the test depend on an extractor.
type Oracle struct {
	Case            string         `json:"case"`
	SourceCommit    string         `json:"source_commit"`
	ReferencePDF    string         `json:"reference_pdf"`
	ReferenceSHA256 string         `json:"reference_sha256"`
	Extractor       string         `json:"extractor"`
	PageCount       int            `json:"page_count"`
	Pages           []string       `json:"pages"`
	Tokens          map[string]int `json:"tokens"`
}

// oraclePath returns the committed oracle of a case.
func oraclePath(dir, name string) string {
	return filepath.Join(dir, "oracle", name+".json")
}

// referencePDFPath returns the path of the reference PDF inside the corpus.
func referencePDFPath(dir string, c Case) string {
	return filepath.Join(dir, filepath.FromSlash(c.Dir), c.Name+".pdf")
}

// loadOracle reads the oracle of a case and checks it against the corpus: the
// PDF it was extracted from must still be the one the manifest hashes, so
// replacing a PDF without regenerating its oracle fails instead of quietly
// comparing against a stale reference.
func loadOracle(dir string, m *Manifest, c Case) (*Oracle, error) {
	path := oraclePath(dir, c.Name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the oracle of %s: %w; generate it with:\n    go test ./internal/weasycompat -update-oracle", c.Name, err)
	}
	var o Oracle
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if o.Case != c.Name {
		return nil, fmt.Errorf("%s describes case %q, not %q", path, o.Case, c.Name)
	}
	if o.PageCount != len(o.Pages) {
		return nil, fmt.Errorf("%s: page_count is %d but %d pages are recorded", path, o.PageCount, len(o.Pages))
	}
	if err := verifyReferencePDF(dir, m, c, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// verifyReferencePDF checks both the manifest hash and the hash recorded in the
// oracle against the file on disk.
func verifyReferencePDF(dir string, m *Manifest, c Case, o *Oracle) error {
	if o.ReferencePDF == "" {
		return fmt.Errorf("the oracle of %s names no reference PDF", c.Name)
	}
	manifestHash := ""
	for _, f := range m.Files {
		if f.Path == o.ReferencePDF {
			manifestHash = f.SHA256
			break
		}
	}
	if manifestHash == "" {
		return fmt.Errorf("the oracle of %s names %s, which samples.json does not list", c.Name, o.ReferencePDF)
	}
	got, err := hashFile(filepath.Join(dir, filepath.FromSlash(o.ReferencePDF)))
	if err != nil {
		return fmt.Errorf("hashing the reference PDF of %s: %w", c.Name, err)
	}
	if got != manifestHash {
		return fmt.Errorf("%s: sha256 %s, samples.json says %s", o.ReferencePDF, got, manifestHash)
	}
	if got != o.ReferenceSHA256 {
		return fmt.Errorf("%s: sha256 %s, the oracle was generated from %s; regenerate it with -update-oracle", o.ReferencePDF, got, o.ReferenceSHA256)
	}
	return nil
}

// fidelity is the measured distance between our page text and the oracle.
type fidelity struct {
	WantPages, GotPages   int
	WantTokens, GotTokens int
	Missing, Extra        []string
}

// ok reports whether the page count and the token multiset both match.
func (f fidelity) ok() bool {
	return f.WantPages == f.GotPages && len(f.Missing) == 0 && len(f.Extra) == 0
}

// String renders the measurement for the test log. It is printed for every
// converting case, enforced only for an accepted one.
func (f fidelity) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "pages %d/%d, tokens %d/%d", f.GotPages, f.WantPages, f.GotTokens, f.WantTokens)
	if len(f.Missing) > 0 {
		fmt.Fprintf(&b, ", missing %s", joinLimited(f.Missing, 20))
	}
	if len(f.Extra) > 0 {
		fmt.Fprintf(&b, ", extra %s", joinLimited(f.Extra, 20))
	}
	return b.String()
}

// compareFidelity measures our pages against the oracle.
func compareFidelity(o *Oracle, pages []string) fidelity {
	got := tokenCounts(tokens(strings.Join(pages, "\n")))
	missing, extra := countDiff(o.Tokens, got)
	return fidelity{
		WantPages:  o.PageCount,
		GotPages:   len(pages),
		WantTokens: total(o.Tokens),
		GotTokens:  total(got),
		Missing:    missing,
		Extra:      extra,
	}
}

// checkFidelity fails an accepted case that stopped matching the reference, and
// nothing else: a case in measure is allowed to differ, because its difference
// has not been reviewed and accepted yet.
func checkFidelity(c Case, f fidelity) error {
	if c.State != StateAccepted || f.ok() {
		return nil
	}
	return fmt.Errorf("case %s is accepted but no longer matches the reference: %s", c.Name, f)
}

func total(counts map[string]int) int {
	n := 0
	for _, v := range counts {
		n += v
	}
	return n
}

// joinLimited joins at most n items and says how many were left out.
func joinLimited(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:n], ", ") + ", and " + strconv.Itoa(len(items)-n) + " more"
}

// buildOracle extracts a case's reference PDF and returns the oracle to commit.
// It is the only path in the suite that runs pdftotext; the normal test reads
// the committed JSON, so the corpus runs without poppler installed.
func buildOracle(dir string, m *Manifest, c Case) (*Oracle, error) {
	pdf := referencePDFPath(dir, c)
	rel, err := filepath.Rel(dir, pdf)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	hash := ""
	for _, f := range m.Files {
		if f.Path == rel {
			hash = f.SHA256
			break
		}
	}
	if hash == "" {
		return nil, fmt.Errorf("%s is not listed in samples.json", rel)
	}
	got, err := hashFile(pdf)
	if err != nil {
		return nil, fmt.Errorf("hashing %s: %w", rel, err)
	}
	if got != hash {
		return nil, fmt.Errorf("%s: sha256 %s, samples.json says %s; refusing to write an oracle for an edited reference PDF", rel, got, hash)
	}

	pageText, err := runPDFToText(pdf)
	if err != nil {
		return nil, err
	}
	extractor, err := pdfToTextVersion()
	if err != nil {
		return nil, err
	}
	pages := splitExtractedPages(pageText)
	return &Oracle{
		Case:            c.Name,
		SourceCommit:    m.Source.Commit,
		ReferencePDF:    rel,
		ReferenceSHA256: hash,
		Extractor:       extractor,
		PageCount:       len(pages),
		Pages:           pages,
		Tokens:          tokenCounts(tokens(strings.Join(pages, "\n"))),
	}, nil
}

// writeOracle commits an oracle as stable JSON: map keys are sorted, so a second
// generation from the same PDF produces identical bytes.
func writeOracle(dir string, o *Oracle) error {
	path := oraclePath(dir, o.Case)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating the oracle directory: %w", err)
	}
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// runPDFToText extracts the text layer of a PDF with the external extractor.
func runPDFToText(path string) (string, error) {
	cmd := exec.Command("pdftotext", "-enc", "UTF-8", path, "-")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running pdftotext on %s: %w (%s); install poppler (brew install poppler)", path, err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// pdfToTextVersion returns the first line of `pdftotext -v`, which poppler
// prints on stderr even on success.
func pdfToTextVersion() (string, error) {
	cmd := exec.Command("pdftotext", "-v")
	out, err := cmd.CombinedOutput()
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if first == "" {
		return "", fmt.Errorf("pdftotext -v printed nothing (err %v); install poppler (brew install poppler)", err)
	}
	return strings.TrimSpace(first), nil
}

// splitExtractedPages splits extractor output into pages. pdftotext writes a
// form feed after every page, including the last one, so exactly one trailing
// empty part is the separator's artifact and not a page; a document whose last
// page is genuinely blank ends with two form feeds and keeps one empty page.
func splitExtractedPages(text string) []string {
	parts := strings.Split(text, "\f")
	if len(parts) > 1 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
