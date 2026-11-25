package weasycompat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates a file below dir, creating parent directories.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hashBelow returns the hash of a file relative to dir.
func hashBelow(t *testing.T, dir, rel string) string {
	t.Helper()
	got, err := hashFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// syntheticCorpus writes a two-file corpus and returns its directory and
// manifest. Tests mutate the returned Manifest or the directory.
func syntheticCorpus(t *testing.T) (string, *Manifest) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "sample/doc.html", "<p>x</p>\n")
	writeFile(t, dir, "sample/doc.css", "p { color: #000 }\n")
	m := &Manifest{
		Source: Source{
			Repository:         "https://example.com/samples",
			Commit:             "0123456789abcdef",
			Retrieved:          "2026-01-01",
			ReferencePDFScript: "printall.sh",
		},
		Cases: []Case{{
			Name:       "sample",
			Dir:        "sample",
			HTML:       "doc.html",
			Stylesheet: "doc.css",
			State:      StateBlocked,
			BlockedBy:  "css/unsupported-property",
		}},
		Files: []File{
			{Path: "sample/doc.css", SHA256: hashBelow(t, dir, "sample/doc.css")},
			{Path: "sample/doc.html", SHA256: hashBelow(t, dir, "sample/doc.html")},
		},
	}
	return dir, m
}

// writeManifest writes m as samples.json into dir.
func writeManifest(t *testing.T, dir string, m *Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "samples.json", string(data)+"\n")
}

func TestLoadManifest(t *testing.T) {
	dir, m := syntheticCorpus(t)
	writeManifest(t, dir, m)

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if loaded.Source.Commit != m.Source.Commit {
		t.Fatalf("commit = %q, want %q", loaded.Source.Commit, m.Source.Commit)
	}
	if len(loaded.Cases) != 1 || loaded.Cases[0].Name != "sample" {
		t.Fatalf("cases = %+v", loaded.Cases)
	}
	if _, ok := loaded.Case("sample"); !ok {
		t.Fatal("Case(sample) not found")
	}
	if _, ok := loaded.Case("absent"); ok {
		t.Fatal("Case(absent) found")
	}
}

func TestLintAcceptsAWellFormedCorpus(t *testing.T) {
	dir, m := syntheticCorpus(t)
	if err := m.Lint(dir); err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if err := m.VerifyProvenance(dir); err != nil {
		t.Fatalf("VerifyProvenance: %v", err)
	}
}

func TestLintRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string, m *Manifest)
		want   string
	}{
		{
			name: "case without a directory",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				m.Cases[0].Dir = ""
			},
			want: "has no directory",
		},
		{
			name: "directory without a case",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				writeFile(t, dir, "stray/doc.html", "<p>y</p>\n")
			},
			want: "directory stray has no case",
		},
		{
			name: "unlisted file",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				writeFile(t, dir, "sample/extra.txt", "x")
			},
			want: "is not listed in samples.json",
		},
		{
			name: "missing html",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				m.Cases[0].HTML = "absent.html"
			},
			want: "no such file",
		},
		{
			name: "unknown state",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				m.Cases[0].State = "green"
			},
			want: "unknown state",
		},
		{
			name: "blocked case without a blocker",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				m.Cases[0].BlockedBy = ""
			},
			want: "names no blocked_by code",
		},
		{
			name: "a converting case that still names a blocker",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				m.Cases[0].State = StateMeasure
			},
			want: "still names a blocker",
		},
		{
			name: "duplicate case",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				m.Cases = append(m.Cases, m.Cases[0])
			},
			want: "listed twice",
		},
		{
			name: "file outside a case directory",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				m.Files = append(m.Files, File{Path: "samples.json", SHA256: "00"})
			},
			want: "not inside a case directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, m := syntheticCorpus(t)
			tt.mutate(t, dir, m)
			err := m.Lint(dir)
			if err == nil {
				t.Fatal("Lint accepted a broken corpus")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Lint error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestVerifyProvenanceRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string, m *Manifest)
		want   string
	}{
		{
			name: "edited file",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				writeFile(t, dir, "sample/doc.html", "<p>edited</p>\n")
			},
			want: "was the sample edited?",
		},
		{
			name: "added file",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				writeFile(t, dir, "sample/added.txt", "x")
			},
			want: "present in the corpus but not in samples.json",
		},
		{
			name: "removed file",
			mutate: func(t *testing.T, dir string, m *Manifest) {
				if err := os.Remove(filepath.Join(dir, "sample", "doc.css")); err != nil {
					t.Fatal(err)
				}
			},
			want: "missing from the corpus",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, m := syntheticCorpus(t)
			tt.mutate(t, dir, m)
			err := m.VerifyProvenance(dir)
			if err == nil {
				t.Fatal("VerifyProvenance accepted a mutated corpus")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("VerifyProvenance error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestRealCorpusIsConsistent(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "weasyprint")
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if err := m.Lint(dir); err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if err := m.VerifyProvenance(dir); err != nil {
		t.Fatalf("VerifyProvenance: %v", err)
	}
	if len(m.Cases) != 6 {
		t.Fatalf("the corpus has %d cases, want 6", len(m.Cases))
	}
	if m.Source.Commit != "6dc675b05a5cba59576dfaa8881ea6ccdccb13cc" {
		t.Fatalf("pinned commit = %q, want the reviewed upstream commit", m.Source.Commit)
	}
}
