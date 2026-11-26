package fsx

import (
	"errors"
	"fmt"
	"io/fs"
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
	defer func() { _ = root.Close() }()
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
	defer func() { _ = root.Close() }()
	if _, err := root.Read(filepath.Join(outside, "secret.css")); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("absolute outside path: %v", err)
	}
	// A missing file above the root is an escape, not a missing file: the
	// boundary is a property of the path, not of what exists on disk.
	if _, err := root.Read(filepath.Join(dir, "..", "x.css")); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("traversal to a missing file: %v", err)
	}
	if _, err := root.Read(filepath.Join(dir, "x.css")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing file inside the root: %v", err)
	}
}

func TestOpenErrorKeepsWrappedNotExist(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "present.css", "x")
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	// A caller may wrap the error further; the not-exist cause must stay
	// detectable instead of being reported as an escape.
	wrapped := fmt.Errorf("loading styles: %w", fs.ErrNotExist)
	if got := root.classifyOpenError(filepath.Join(dir, "present.css"), wrapped); got != wrapped {
		t.Fatalf("classifyOpenError returned %v, want the cause unchanged", got)
	}
	if !errors.Is(root.classifyOpenError(filepath.Join(dir, "present.css"), wrapped), fs.ErrNotExist) {
		t.Fatal("a wrapped fs.ErrNotExist must stay detectable")
	}
}

// TestOpenErrorIsClassifiedByContainment guards the rule that the error text
// cannot decide an escape: a message that mentions "escapes" is preserved for a
// path inside the root, while a path that really leaves the root becomes
// ErrOutsideRoot whatever the message says.
func TestOpenErrorIsClassifiedByContainment(t *testing.T) {
	dir := t.TempDir()
	inside := writeFile(t, dir, "escapes.css", "x")
	outside := writeFile(t, t.TempDir(), "secret.css", "x")
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	failure := errors.New("openat escapes.css: path escapes from parent")
	if got := root.classifyOpenError(inside, failure); got != failure {
		t.Fatalf("a message mentioning escapes changed the cause inside the root: %v", got)
	}
	if got := root.classifyOpenError(outside, failure); !errors.Is(got, ErrOutsideRoot) {
		t.Fatalf("a path outside the root returned %v, want ErrOutsideRoot", got)
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
	defer func() { _ = root.Close() }()
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
	defer func() { _ = root.Close() }()
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
	defer func() { _ = root.Close() }()
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

func TestRelativeRootWithAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "a.css", "x")
	t.Chdir(dir)
	root, err := Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if _, err := root.Read(file); err != nil {
		t.Fatalf("an absolute path inside a relative root must be allowed: %v", err)
	}
	if _, err := root.Read("a.css"); err != nil {
		t.Fatalf("a relative path inside a relative root must be allowed: %v", err)
	}
}

func TestReadLimitBoundsTheRead(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "big.css", strings.Repeat("x", 100))
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	data, err := root.ReadLimit(path, 100)
	if err != nil || len(data) != 100 {
		t.Fatalf("a file at the limit must be read: %d bytes, err=%v", len(data), err)
	}
	if _, err := root.ReadLimit(path, 99); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized read returned %v, want ErrTooLarge", err)
	}
	if _, err := ReadFileLimit(path, 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadFileLimit returned %v, want ErrTooLarge", err)
	}
	if data, err := ReadFileLimit(path, 100); err != nil || len(data) != 100 {
		t.Fatalf("ReadFileLimit at the limit: %d bytes, err=%v", len(data), err)
	}
}

// TestResolveCanonicalizesSpellings guards the identity a caller uses to cache
// an asset: every spelling of one file resolves to the same absolute path.
func TestResolveCanonicalizesSpellings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	plain, err := root.Resolve(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	spelled, err := root.Resolve(filepath.Join(dir, "sub", "..", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if plain != spelled {
		t.Fatalf("spellings resolved to %q and %q", plain, spelled)
	}
	size, err := root.Size(filepath.Join(dir, "sub", "..", "a.txt"))
	if err != nil || size != 3 {
		t.Fatalf("Size = %d, %v; want 3", size, err)
	}
	if _, err := root.Resolve(filepath.Join(dir, "..", "missing.txt")); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("an outside path resolved to %v", err)
	}
}

// TestRelativePathsResolveInsideTheRoot guards RV4-36: Read, Resolve and Size
// used the process working directory for a relative path, so a file inside the
// root was reported as an escape whenever the process ran elsewhere.
func TestRelativePathsResolveInsideTheRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "sub", "file.txt")
	if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	// A different working directory must not change the meaning of the path.
	t.Chdir(t.TempDir())
	data, err := root.Read("sub/file.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("Read returned %q", data)
	}
	if size, err := root.Size("sub/file.txt"); err != nil || size != int64(len("content")) {
		t.Fatalf("Size = %d, %v", size, err)
	}
	// Resolve reports the canonical path: on macOS the temporary directory is
	// reached through /private, so compare the resolved form of the target.
	wantTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := root.Resolve("sub/file.txt")
	if err != nil || resolved != wantTarget {
		t.Fatalf("Resolve = %q, %v; want %q", resolved, err, wantTarget)
	}
	// Containment still holds: a relative traversal leaves the root.
	if _, err := root.Read("../outside.txt"); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("a traversal returned %v, want ErrOutsideRoot", err)
	}
}
