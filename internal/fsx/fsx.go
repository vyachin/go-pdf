// Package fsx reads local files that must stay inside a configured root.
package fsx

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrOutsideRoot reports a path that leaves its configured root.
var ErrOutsideRoot = errors.New("path escapes the root directory")

// ErrTooLarge reports a file that is larger than the requested limit.
var ErrTooLarge = errors.New("file is larger than the size limit")

// ErrNotRegular reports a path that is not a regular file: a directory, a
// device, a FIFO, a socket or another special file. An asset is read only when
// it is a regular file; every other kind is rejected before it is opened for
// reading, so a FIFO cannot wait for a writer.
var ErrNotRegular = errors.New("not a regular file")

// Root is a directory that file paths must stay inside. A zero Root reads
// paths directly and enforces no boundary.
type Root struct {
	dir string
	// absDir is the configured directory before symlinks are resolved. It is
	// used only to judge a path that does not exist, which has no resolved form.
	absDir string
	handle *os.Root
	// beforeOpen, when set, runs between the pre-open stat and the open. It is
	// an instance-local test seam, like the registry's read hooks, so a test can
	// replace the file and prove the post-open check still rejects it.
	beforeOpen func()
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
// limit+1 bytes. A path that is not a regular file is rejected before it is
// opened, so a FIFO cannot wait for a writer.
func ReadFileLimit(path string, limit int) ([]byte, error) {
	f, err := openOutsideRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readLimited(f, limit)
}

// CheckRegular returns ErrNotRegular when info does not describe a regular
// file, naming the path and what the file is instead. It is the one gate every
// reader applies — through a root, outside a root and on a caller-supplied
// source — so a directory, device, FIFO or socket is reported the same way.
func CheckRegular(path string, info fs.FileInfo) error {
	if info.Mode().IsRegular() {
		return nil
	}
	return fmt.Errorf("%w: %q is %s", ErrNotRegular, path, fileKind(info.Mode()))
}

// fileKind names a non-regular mode for a diagnostic.
func fileKind(mode fs.FileMode) string {
	switch {
	case mode.IsDir():
		return "a directory"
	case mode&fs.ModeNamedPipe != 0:
		return "a FIFO"
	case mode&fs.ModeSocket != 0:
		return "a socket"
	case mode&fs.ModeCharDevice != 0:
		return "a character device"
	case mode&fs.ModeDevice != 0:
		return "a device"
	case mode&fs.ModeSymlink != 0:
		return "a symlink"
	default:
		return "a special file"
	}
}

// openOutsideRoot opens an operating-system path that is not confined to a
// root, applying the same regular-file gate as Root.open: a stat before the
// open and another through the descriptor, so a file replaced in between cannot
// turn into a FIFO unnoticed.
func openOutsideRoot(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if err := CheckRegular(path, info); err != nil {
		return nil, err
	}
	f, err := openPath(path)
	if err != nil {
		return nil, err
	}
	if err := checkOpenedRegular(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// checkOpenedRegular repeats the regular-file check through the descriptor that
// was opened. The non-blocking open means the check runs instead of waiting for
// a writer, so a FIFO that replaced the file between the stat and the open is
// still rejected before anything is read.
func checkOpenedRegular(f *os.File, path string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return CheckRegular(path, info)
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

// full joins a relative path with the root directory. A relative path is
// interpreted inside the root, not against the process working directory.
func (r *Root) full(path string) string {
	if r == nil || r.dir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(r.dir, path)
}

// relative returns the containment-checked path relative to the root.
func (r *Root) relative(path string) (string, error) {
	abs, err := filepath.Abs(r.full(path))
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// A path that does not exist has no resolved form, so containment is
		// judged on the clean absolute path. A traversal above the root is then
		// an escape rather than a missing file, which is the diagnostic the
		// callers report for it.
		if rel, relErr := filepath.Rel(r.containmentBase(abs), abs); relErr != nil || escapes(rel) {
			return "", ErrOutsideRoot
		}
		return "", err
	}
	rel, err := filepath.Rel(r.dir, real)
	if err != nil || escapes(rel) {
		return "", ErrOutsideRoot
	}
	return rel, nil
}

// escapes reports whether rel leaves the directory it was computed against.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Resolve returns the absolute path of path with symlinks resolved, after the
// same containment check Read applies. It is the identity of a file: two
// spellings of one path resolve to the same string. A path outside the root
// returns ErrOutsideRoot; a missing path keeps its open error.
func (r *Root) Resolve(path string) (string, error) {
	if r == nil || r.dir == "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			return real, nil
		}
		return abs, nil
	}
	abs, err := filepath.Abs(r.full(path))
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// A path that does not exist has no resolved form, so containment is
		// judged on the path itself. The configured directory may be reached
		// through a symlink (a temporary directory on macOS), so the path is
		// compared against whichever of the two spellings it belongs to.
		if rel, relErr := filepath.Rel(r.containmentBase(abs), abs); relErr != nil || escapes(rel) {
			return "", ErrOutsideRoot
		}
		return "", err
	}
	rel, err := filepath.Rel(r.dir, real)
	if err != nil || escapes(rel) {
		return "", ErrOutsideRoot
	}
	return real, nil
}

// containmentBase returns the directory a path that does not exist is compared
// against: the resolved root when the path lies inside it, otherwise the
// configured directory as the caller spelled it.
func (r *Root) containmentBase(abs string) string {
	if rel, err := filepath.Rel(r.dir, abs); err == nil && !escapes(rel) {
		return r.dir
	}
	return r.absDir
}

// Size returns the size of path in bytes after the same containment and
// regular-file checks as Read, so a caller can refuse a non-regular file and
// report its load diagnostic before it reserves any bytes. A missing path keeps
// its stat error.
func (r *Root) Size(path string) (int64, error) {
	if r == nil || r.dir == "" {
		real, err := r.Resolve(path)
		if err != nil {
			return 0, err
		}
		info, err := os.Stat(real)
		if err != nil {
			return 0, err
		}
		if err := CheckRegular(path, info); err != nil {
			return 0, err
		}
		return info.Size(), nil
	}
	rel, err := r.relative(path)
	if err != nil {
		return 0, err
	}
	// The size is read through the root handle, like the content: a symlink
	// swapped between the two calls cannot point the stat somewhere else.
	info, err := r.handle.Stat(rel)
	if err != nil {
		return 0, r.classifyOpenError(path, err)
	}
	if err := CheckRegular(path, info); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// open resolves path against the root and opens it, applying the containment
// rules of Read and refusing a non-regular file. The stat before the open is
// what keeps a FIFO from waiting for a writer in the common case; the stat
// through the descriptor decides about the file that was actually opened.
func (r *Root) open(path string) (*os.File, error) {
	if r == nil || r.dir == "" {
		return openOutsideRoot(path)
	}
	rel, err := r.relative(path)
	if err != nil {
		return nil, err
	}
	info, err := r.handle.Stat(rel)
	if err != nil {
		return nil, r.classifyOpenError(path, err)
	}
	if err := CheckRegular(path, info); err != nil {
		return nil, err
	}
	if r.beforeOpen != nil {
		r.beforeOpen()
	}
	f, err := openInRoot(r.handle, rel)
	if err != nil {
		return nil, r.classifyOpenError(path, err)
	}
	if err := checkOpenedRegular(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// classifyOpenError keeps the cause of a failed stat or open. A missing file
// keeps its error; every other failure is reported as an escape only when the
// containment check itself says the path leaves the root, so a permission error
// inside the root stays a permission error and a file name can never change the
// classification. The check is rerun rather than inspected in the error text
// because a path can start to escape between the first check and the open, and
// because the unexported error os.Root returns for an escape is not part of its
// public API.
func (r *Root) classifyOpenError(path string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, containmentErr := r.relative(path); errors.Is(containmentErr, ErrOutsideRoot) {
		return ErrOutsideRoot
	}
	return err
}
