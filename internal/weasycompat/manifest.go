package weasycompat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The three states a case may declare. The suite enforces the declared state
// against what the engine actually does, so a promotion or a regression is a
// reviewed manifest edit rather than a silent change.
const (
	// StateBlocked: the profile rejects the document. The first diagnostic code
	// must equal the case's blocked_by.
	StateBlocked = "blocked"
	// StateMeasure: the document converts; its page text is frozen in a golden
	// and its distance from the reference text is logged, not enforced.
	StateMeasure = "measure"
	// StateAccepted: the document converts and its page count and token
	// multiset equal the reference.
	StateAccepted = "accepted"
)

// Manifest is the committed description of the corpus: where it came from, the
// hash of every vendored file, and the state of every case.
type Manifest struct {
	Source Source `json:"source"`
	Cases  []Case `json:"cases"`
	Files  []File `json:"files"`
}

// Source pins the upstream material.
type Source struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Retrieved  string `json:"retrieved"`
	// ReferencePDFScript is the upstream script that produced the reference
	// PDFs, so the oracle can be regenerated from the same recipe.
	ReferencePDFScript string `json:"reference_pdf_script"`
}

// Case is one sample document and the state the suite enforces for it.
type Case struct {
	Name       string `json:"name"`
	Dir        string `json:"dir"`
	HTML       string `json:"html"`
	Stylesheet string `json:"stylesheet"`
	State      string `json:"state"`
	// BlockedBy is the diagnostic code that rejects the case. It is set only
	// for StateBlocked and must equal the first error code observed.
	BlockedBy string `json:"blocked_by"`
}

// File is one vendored file and its committed SHA-256.
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// LoadManifest reads samples.json from the corpus directory.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "samples.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the corpus manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &m, nil
}

// Case returns the named case.
func (m *Manifest) Case(name string) (Case, bool) {
	for _, c := range m.Cases {
		if c.Name == name {
			return c, true
		}
	}
	return Case{}, false
}

// caseDirs returns the distinct vendored directories of the manifest, sorted.
func (m *Manifest) caseDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	for _, c := range m.Cases {
		if c.Dir == "" || seen[c.Dir] {
			continue
		}
		seen[c.Dir] = true
		dirs = append(dirs, c.Dir)
	}
	sort.Strings(dirs)
	return dirs
}

// scanFiles returns every regular file under the case directories, as
// slash-separated paths relative to the corpus directory.
func (m *Manifest) scanFiles(dir string) ([]string, error) {
	var out []string
	for _, caseDir := range m.caseDirs() {
		root := filepath.Join(dir, filepath.FromSlash(caseDir))
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", caseDir, err)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Lint checks the structure of the manifest against the corpus on disk: every
// case has a directory and its HTML, every vendored directory has a case, and
// every file on disk is listed. Hashes are checked by VerifyProvenance.
func (m *Manifest) Lint(dir string) error {
	var problems []string
	if m.Source.Repository == "" || m.Source.Commit == "" {
		problems = append(problems, "source.repository and source.commit must be set")
	}
	if len(m.Cases) == 0 {
		problems = append(problems, "the manifest lists no case")
	}
	names := map[string]bool{}
	caseDirs := map[string]bool{}
	for _, c := range m.Cases {
		switch {
		case c.Name == "":
			problems = append(problems, "a case has no name")
		case names[c.Name]:
			problems = append(problems, fmt.Sprintf("case %s is listed twice", c.Name))
		}
		names[c.Name] = true
		switch c.State {
		case StateBlocked:
			if c.BlockedBy == "" {
				problems = append(problems, fmt.Sprintf("case %s is blocked but names no blocked_by code", c.Name))
			}
		case StateMeasure, StateAccepted:
			if c.BlockedBy != "" {
				problems = append(problems, fmt.Sprintf("case %s is %s but still names a blocker %q", c.Name, c.State, c.BlockedBy))
			}
		default:
			problems = append(problems, fmt.Sprintf("case %s: unknown state %q", c.Name, c.State))
		}
		if c.Dir == "" {
			problems = append(problems, fmt.Sprintf("case %s has no directory", c.Name))
			continue
		}
		caseDirs[c.Dir] = true
		if c.HTML == "" {
			problems = append(problems, fmt.Sprintf("case %s names no HTML file", c.Name))
		} else if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(c.Dir), filepath.FromSlash(c.HTML))); err != nil {
			problems = append(problems, fmt.Sprintf("case %s: HTML %s: %v", c.Name, c.HTML, err))
		}
		if c.Stylesheet != "" {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(c.Dir), filepath.FromSlash(c.Stylesheet))); err != nil {
				problems = append(problems, fmt.Sprintf("case %s: stylesheet %s: %v", c.Name, c.Stylesheet, err))
			}
		}
	}

	// Every directory beside the sample directories is bookkeeping, not a
	// sample. Anything else must belong to a case.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading the corpus directory: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		switch e.Name() {
		case "licenses", "oracle", "golden":
			continue
		}
		if !caseDirs[e.Name()] {
			problems = append(problems, fmt.Sprintf("directory %s has no case", e.Name()))
		}
	}

	listed := map[string]bool{}
	for _, f := range m.Files {
		if f.Path == "" {
			problems = append(problems, "a file entry has no path")
			continue
		}
		if listed[f.Path] {
			problems = append(problems, fmt.Sprintf("%s is listed twice", f.Path))
		}
		listed[f.Path] = true
		if !inCaseDir(f.Path, caseDirs) {
			problems = append(problems, fmt.Sprintf("%s is not inside a case directory", f.Path))
		}
	}
	onDisk, err := m.scanFiles(dir)
	if err != nil {
		return err
	}
	for _, path := range onDisk {
		if !listed[path] {
			problems = append(problems, fmt.Sprintf("%s is not listed in samples.json", path))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("corpus manifest lint failed:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

// VerifyProvenance checks that every vendored file is byte-identical to the
// copy the manifest hashes, and that the corpus holds no other file. A missing,
// edited or unexpected file is an error: an edited sample would invalidate the
// oracle while every test stayed green.
func (m *Manifest) VerifyProvenance(dir string) error {
	var problems []string
	listed := map[string]string{}
	for _, f := range m.Files {
		if f.Path == "" || f.SHA256 == "" {
			problems = append(problems, fmt.Sprintf("file entry without a path or a hash: %+v", f))
			continue
		}
		if _, dup := listed[f.Path]; dup {
			problems = append(problems, fmt.Sprintf("%s is listed twice", f.Path))
			continue
		}
		listed[f.Path] = f.SHA256
	}
	onDisk, err := m.scanFiles(dir)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, path := range onDisk {
		present[path] = true
		want, ok := listed[path]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: present in the corpus but not in samples.json", path))
			continue
		}
		got, err := hashFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if got != want {
			problems = append(problems, fmt.Sprintf("%s: sha256 %s, samples.json says %s (was the sample edited?)", path, got, want))
		}
	}
	for _, f := range m.Files {
		if f.Path != "" && !present[f.Path] {
			problems = append(problems, fmt.Sprintf("%s: listed in samples.json but missing from the corpus", f.Path))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("corpus provenance check failed:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

// hashFile returns the lowercase hex SHA-256 of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// inCaseDir reports whether path starts with one of the case directories.
func inCaseDir(path string, caseDirs map[string]bool) bool {
	for dir := range caseDirs {
		if strings.HasPrefix(path, dir+"/") {
			return true
		}
	}
	return false
}
