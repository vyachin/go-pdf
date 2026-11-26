package assets

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/vyachin/go-pdf/internal/asset"
	"github.com/vyachin/go-pdf/internal/fsx"
)

// SourcePath resolves one document reference against a directory inside a
// caller-supplied fs.FS and returns the name to open. The reference is a URL
// path, so it is cleaned with path.Clean before it is validated. A reference
// that is absolute, still contains ".." after cleaning or names a directory
// rather than a file is rejected with fsx.ErrOutsideRoot, which callers map to
// the diagnostic a filesystem escape produces.
//
// An empty dir means the source root.
func SourcePath(dir, reference string) (string, error) {
	ref := path.Clean(reference)
	if ref == "." || !fs.ValidPath(ref) {
		return "", fmt.Errorf("%w: %q is not a relative source path", fsx.ErrOutsideRoot, reference)
	}
	if dir == "" {
		return ref, nil
	}
	name := path.Join(dir, ref)
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("%w: %q leaves the source directory", fsx.ErrOutsideRoot, reference)
	}
	return name, nil
}

// SourceDir validates a directory name inside a caller-supplied fs.FS and
// returns the name to open. An empty name and "." both name the source root.
// The directory must exist and be a directory: a configured root the source
// cannot honour is reported like an unusable host asset directory.
func SourceDir(source fs.FS, dir string) (string, error) {
	if source == nil {
		return "", errors.New("no asset source")
	}
	name := path.Clean(dir)
	if dir == "" || name == "." {
		return "", nil
	}
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("%w: %q is not a relative source directory", fsx.ErrOutsideRoot, dir)
	}
	info, err := fs.Stat(source, name)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", name)
	}
	return name, nil
}

// sourceFilePath validates a caller-selected source path. The caller named the
// file itself, so it is read from the source root rather than from a document
// root, and it is still confined to the source.
func sourceFilePath(reference string) (string, error) {
	name := path.Clean(reference)
	if name == "." || !fs.ValidPath(name) {
		return "", fmt.Errorf("%w: %q is not a relative source path", fsx.ErrOutsideRoot, reference)
	}
	return name, nil
}

// sourceName returns the source name one request names.
func (r *Registry) sourceName(req Request) (string, error) {
	if req.Explicit {
		return sourceFilePath(req.Reference)
	}
	ref, err := asset.Resolve(req.Reference)
	if err != nil {
		return "", err
	}
	if ref.Remote {
		return "", errors.New("remote references are not supported")
	}
	if ref.Data {
		return "", errors.New("a data: URI is read with AdmitData")
	}
	return SourcePath(req.Dir, ref.Path)
}

// keyForSource returns the identity of one source file. The cleaned source path
// is the identity inside the conversion, the same way the canonical host path
// identifies a file on the filesystem.
func (req Request) keyForSource(name string) string {
	identity := "source:" + name
	if req.Decoding != "" {
		return string(req.Kind) + "|" + identity + "|" + req.Decoding
	}
	return string(req.Kind) + "|" + identity
}

// readSource reads one asset through the caller-supplied fs.FS. Concurrent
// callers of one identity share one open, one admission and one charge: the
// first caller leads and the others wait on its call.
func (r *Registry) readSource(req Request) (*Record, error) {
	name, err := r.sourceName(req)
	if err != nil {
		return nil, &Error{Kind: req.Kind, Reference: req.Reference, Err: err}
	}
	key := req.keyForSource(name)
	rec, call, leader, hook, err := r.acquire(key)
	if err != nil {
		return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
	}
	if rec != nil {
		return rec, nil
	}
	if !leader {
		return awaitRead(call, hook)
	}
	rec, err = r.readSourceLeader(req, key, name)
	r.finishRead(key, call, rec, err)
	if err != nil {
		return nil, err
	}
	out := *rec
	return &out, nil
}

// readSourceLeader performs the leader half of a source read. The file is
// classified before the open when the source can stat it, and through the
// descriptor otherwise, so a source backed by the host filesystem cannot block
// in open(2) on a FIFO. The file is opened once and closed on every path, its
// declared size is checked and reserved, and the contents are read through
// io.LimitReader, so a source cannot hand the conversion more bytes than
// MaxBytes however large the object claims to be. The reservation is then
// reconciled with the bytes actually admitted.
func (r *Registry) readSourceLeader(req Request, key, name string) (*Record, error) {
	// A source that can report file information is classified before the open;
	// a source that cannot is classified through the descriptor after it. The
	// pre-open stat is what keeps an operating-system source from waiting in
	// open(2) on a FIFO.
	if statter, ok := r.source.(fs.StatFS); ok {
		info, err := statter.Stat(name)
		if err != nil {
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
		}
		if err := fsx.CheckRegular(name, info); err != nil {
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
		}
	}
	f, err := r.source.Open(name)
	if err != nil {
		return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
	}
	defer func() { _ = f.Close() }()
	size := int64(-1)
	if openedInfo, statErr := f.Stat(); statErr == nil {
		if err := fsx.CheckRegular(name, openedInfo); err != nil {
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
		}
		size = openedInfo.Size()
	}
	if req.MaxBytes > 0 && size > int64(req.MaxBytes) {
		return nil, &Error{Kind: req.Kind, Reference: req.Reference,
			Err: fmt.Errorf("%w: the file is %d bytes, the limit is %d", ErrTooLarge, size, req.MaxBytes)}
	}
	// A source that reports a size is charged before the read; one that cannot
	// report a size is charged for the bytes actually read below.
	declared := size >= 0
	if declared {
		if err := r.reserve(req.Kind, size, req.MaxTotalBytes); err != nil {
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Err: err}
		}
		if err := r.guard.ChargeBytes(int(size)); err != nil {
			r.refund(req.Kind, size)
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Err: err}
		}
	}
	r.beforeRead()
	data, readErr := readSourceLimited(f, req.MaxBytes)
	if readErr != nil {
		if declared {
			r.refund(req.Kind, size)
		}
		if errors.Is(readErr, fsx.ErrTooLarge) {
			readErr = fmt.Errorf("%w: %v", ErrTooLarge, readErr)
		}
		return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: readErr}
	}
	actual := int64(len(data))
	if declared {
		if err := r.reconcile(req.Kind, size, actual, req.MaxTotalBytes); err != nil {
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
		}
	} else {
		if err := r.reserve(req.Kind, actual, req.MaxTotalBytes); err != nil {
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
		}
		if err := r.guard.ChargeBytes(int(actual)); err != nil {
			r.refund(req.Kind, actual)
			return nil, &Error{Kind: req.Kind, Reference: req.Reference, Path: name, Err: err}
		}
	}
	rec := &Record{Kind: req.Kind, Key: key, Path: name, Canonical: name, Bytes: data, charged: actual}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		r.refund(req.Kind, actual)
		return nil, &Error{Kind: req.Kind, Reference: req.Reference,
			Err: errors.New("the asset registry is closed")}
	}
	r.cache[key] = rec
	r.reads[req.Kind]++
	r.mu.Unlock()
	return rec, nil
}

// readSourceLimited reads a source file, never allocating more than limit+1
// bytes. A limit of zero or less leaves the read unlimited.
func readSourceLimited(f fs.File, limit int) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(f)
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%w: the limit is %d bytes", fsx.ErrTooLarge, limit)
	}
	return data, nil
}
