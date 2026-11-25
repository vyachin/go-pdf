package diag

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// moduleRoot walks up from the test's working directory to the module root.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}

// TestEveryEmittedCodeIsRegistered guards IMP-01: a code that reaches a call
// site or a struct literal must be declared here, and a code declared here must
// still be emitted everywhere the catalogue implies, so neither side can drift.
func TestEveryEmittedCodeIsRegistered(t *testing.T) {
	registered := map[string]bool{}
	domains := map[string]bool{}
	for _, c := range Codes() {
		registered[string(c)] = true
		domains[strings.SplitN(string(c), "/", 2)[0]] = true
	}
	literal := regexp.MustCompile(`"([a-z][a-z0-9]*/[a-z0-9-]+)"`)
	seen := map[string]bool{}
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".cache", "testdata", "openspec", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range literal.FindAllStringSubmatch(string(data), -1) {
			code := match[1]
			if !domains[strings.SplitN(code, "/", 2)[0]] {
				// Not a diagnostic code: a media type or another slash-separated
				// name in the same domain would have to be registered.
				continue
			}
			seen[code] = true
			if !registered[code] {
				t.Errorf("%s emits unregistered code %q", path, code)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for code := range registered {
		if !seen[code] {
			t.Errorf("%q is registered but no file emits it", code)
		}
	}
}

// TestEveryCodeHasAMeaning keeps the catalogue complete: the registry and the
// meanings map must describe the same set of codes.
func TestEveryCodeHasAMeaning(t *testing.T) {
	for _, c := range Codes() {
		if meanings[c] == "" {
			t.Errorf("%s has no description", c)
		}
	}
	if len(meanings) != len(Codes()) {
		t.Errorf("the meanings map has %d entries, the registry %d", len(meanings), len(Codes()))
	}
	catalogue := Catalogue()
	for _, c := range Codes() {
		if !strings.Contains(catalogue, "`"+string(c)+"`") {
			t.Errorf("the catalogue omits %s", c)
		}
	}
}

// TestRegisteredCodesAreUnique catches a duplicate value under two names.
func TestRegisteredCodesAreUnique(t *testing.T) {
	seen := map[Code]bool{}
	for _, c := range Codes() {
		if seen[c] {
			t.Errorf("%q is registered twice", c)
		}
		seen[c] = true
	}
}
