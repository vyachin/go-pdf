package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadInsideRoot(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.css", "body {}")
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data, err := root.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "body {}" {
		t.Fatalf("read %q", data)
	}
}

func TestReadRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "secret.css", "secret")
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := root.Read(filepath.Join(outside, "secret.css")); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("absolute outside path: %v", err)
	}
	if _, err := root.Read(filepath.Join(dir, "..", "x.css")); !errors.Is(err, ErrOutsideRoot) && !os.IsNotExist(err) {
		t.Fatalf("traversal: %v", err)
	}
}

func TestReadFollowsInternalSymlink(t *testing.T) {
	dir := t.TempDir()
	real := writeFile(t, dir, "real.css", "p {}")
	link := filepath.Join(dir, "link.css")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := root.Read(link); err != nil {
		t.Fatalf("internal symlink: %v", err)
	}
}

func TestReadRejectsEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	target := writeFile(t, outside, "secret.css", "secret")
	link := filepath.Join(dir, "link.css")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := root.Read(link); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("escaping symlink: %v", err)
	}
}

func TestReadReportsPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	path := writeFile(t, dir, "denied.css", "x")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	_, err = root.Read(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("a permission error must not be reported as an escape: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "permission") {
		t.Fatalf("error = %v, want a permission error", err)
	}
}

func TestNoRootReadsDirectly(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.css", "x")
	root, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	if root.Dir() != "" {
		t.Fatalf("dir = %q", root.Dir())
	}
	if _, err := root.Read(path); err != nil {
		t.Fatal(err)
	}
}
