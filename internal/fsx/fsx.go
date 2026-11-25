// Package fsx reads local files that must stay inside a configured root.
package fsx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrOutsideRoot reports a path that leaves its configured root.
var ErrOutsideRoot = errors.New("path escapes the root directory")

// ErrTooLarge reports a file that is larger than the requested limit.
var ErrTooLarge = errors.New("file is larger than the size limit")

// Root is a directory that file paths must stay inside. A zero Root reads
// paths directly and enforces no boundary.
type Root struct {
	dir string
	// absDir is the configured directory before symlinks are resolved. It is
	// used only to judge a path that does not exist, which has no resolved form.
	absDir string
	handle *os.Root
}

// Open resolves dir once and returns a Root that reuses the handle for every
// read. An empty dir returns a Root without a boundary.
func Open(dir string) (*Root, error) {
	if dir == "" {
		return &Root{}, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving root %q: %w", dir, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolving root %q: %w", dir, err)
	}
	handle, err := os.OpenRoot(real)
	if err != nil {
		return nil, fmt.Errorf("opening root %q: %w", dir, err)
	}
	return &Root{dir: real, absDir: abs, handle: handle}, nil
}

// Close releases the root handle.
func (r *Root) Close() error {
	if r == nil || r.handle == nil {
		return nil
	}
	return r.handle.Close()
}

// Dir reports the resolved root directory, empty when there is no boundary.
func (r *Root) Dir() string {
	if r == nil {
		return ""
	}
	return r.dir
}

// Read reads path, which must stay inside the root. A path that escapes the
// root returns ErrOutsideRoot; other failures keep their original cause.
func (r *Root) Read(path string) ([]byte, error) {
	return r.read(path, 0)
}

// ReadLimit reads path like Read but refuses a file larger than limit bytes. It
// reads at most limit+1 bytes, so a growing file cannot allocate more than the
// limit even if its size changes between Stat and Read.
func (r *Root) ReadLimit(path string, limit int) ([]byte, error) {
	return r.read(path, limit)
}

// ReadFileLimit reads a file outside any root, never allocating more than
// limit+1 bytes.
func ReadFileLimit(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readLimited(f, limit)
}

func (r *Root) read(path string, limit int) ([]byte, error) {
	f, err := r.open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readLimited(f, limit)
}

func readLimited(f *os.File, limit int) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(f)
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%w: the limit is %d bytes", ErrTooLarge, limit)
	}
	return data, nil
}

// escapes reports whether rel leaves the directory it was computed against.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// open resolves path against the root and opens it, applying the containment
// rules of Read.
func (r *Root) open(path string) (*os.File, error) {
	if r == nil || r.dir == "" {
		return os.Open(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// A path that does not exist has no resolved form, so containment is
		// judged on the clean absolute path. A traversal above the root is then
		// an escape rather than a missing file, which is the diagnostic the
		// callers report for it.
		if rel, relErr := filepath.Rel(r.absDir, abs); relErr != nil || escapes(rel) {
			return nil, ErrOutsideRoot
		}
		return nil, err
	}
	rel, err := filepath.Rel(r.dir, real)
	if err != nil || escapes(rel) {
		return nil, ErrOutsideRoot
	}
	f, err := r.handle.Open(rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		if strings.Contains(err.Error(), "escapes") {
			return nil, ErrOutsideRoot
		}
		// Permission and other failures keep their cause.
		return nil, err
	}
	return f, nil
}
