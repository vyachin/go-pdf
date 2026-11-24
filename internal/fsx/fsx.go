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

// Root is a directory that file paths must stay inside. A zero Root reads
// paths directly and enforces no boundary.
type Root struct {
	dir    string
	handle *os.Root
}

// Open resolves dir once and returns a Root that reuses the handle for every
// read. An empty dir returns a Root without a boundary.
func Open(dir string) (*Root, error) {
	if dir == "" {
		return &Root{}, nil
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving root %q: %w", dir, err)
	}
	handle, err := os.OpenRoot(real)
	if err != nil {
		return nil, fmt.Errorf("opening root %q: %w", dir, err)
	}
	return &Root{dir: real, handle: handle}, nil
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
	if r == nil || r.dir == "" {
		return os.ReadFile(path)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(r.dir, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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
	defer f.Close()
	return io.ReadAll(f)
}
