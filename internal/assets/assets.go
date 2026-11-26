// Package assets reads the files one conversion references — images,
// stylesheets and fonts — through one conversion-scoped registry.
//
// The registry owns the confined readers (fsx.Root), the identity of every
// asset, the conversion's admission state and the typed failures. Parsers and
// their typed caches stay with the adapters that own the format: an adapter
// asks the registry for the bytes of one authorized reference and keys its
// parsed records by the returned identity, so two references to one file share
// the read, the accounting and — when the adapter allows it — the parse.
//
// Nothing is global: a registry belongs to one conversion, is closed on every
// exit, and never merges records across conversions or access policies.
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/vyachin/go-pdf/internal/asset"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/fsx"
)

// Kind is the kind of asset a reference names. Identity and accounting are kept
// per kind, so an image and a font with one path stay distinct records.
type Kind string

const (
	// Image is an <img> or background image reference.
	Image Kind = "image"
	// Font is an @font-face source, an explicit FallbackFont or a system font.
	Font Kind = "font"
	// Stylesheet is a <link rel="stylesheet"> reference.
	Stylesheet Kind = "stylesheet"
)

// ErrTooLarge reports one file or payload over its MaxBytes. It wraps
// budget.ErrExceeded, so every caller still maps it to resource/limit, and the
// adapters can add the message their resource deserves.
var ErrTooLarge = fmt.Errorf("%w: the file exceeds its size limit", budget.ErrExceeded)

// Error is one asset failure: the kind of resource, the reference that named it
// and the cause. The cause is unwrapped, so callers keep matching
// budget.ErrExceeded, fsx.ErrOutsideRoot and the format parsers' errors.
type Error struct {
	Kind      Kind
	Reference string
	// Path is the local path the reference named, when it could be resolved
	// far enough to name one. A diagnostic uses it instead of the raw
	// reference, so a query string or a fragment never reaches the message.
	Path string
	Err  error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s %q: %v", e.Kind, e.Reference, e.Err)
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Err }

// Request is one reference the conversion asks for.
type Request struct {
	// Kind is the resource kind.
	Kind Kind
	// Reference is the reference as the document or the caller wrote it.
	Reference string
	// Dir is the root that confines a document reference. On the host
	// filesystem an empty Dir means no boundary (the working directory), like
	// fsx.Open(""); on a source it means the source root.
	Dir string
	// Explicit marks a caller-selected path: it is read as an operating-system
	// path, outside the document roots, because the caller named the file
	// itself. On a source the path is read from the source root instead and is
	// still confined to the source. The reference is not resolved as a URL.
	Explicit bool
	// MaxBytes bounds one file; zero means unlimited.
	MaxBytes int
	// MaxTotalBytes bounds one kind's successful bytes in this conversion; zero
	// means unlimited. The limit is checked against the bytes already committed
	// for the kind, and the reservation is released again by Discard.
	MaxTotalBytes int64
	// Decoding names the decoding inputs that make two reads of one file
	// distinct records — the stylesheet charset, for example.
	Decoding string
}

// Record is one successfully read asset.
type Record struct {
	// Kind is the resource kind.
	Kind Kind
	// Key is the identity: kind, the canonical file identity or content digest,
	// and the decoding inputs.
	Key string
	// Path is the path the reference named, joined with the root: the local
	// path on the host filesystem, or the validated source path. It is what a
	// diagnostic should name.
	Path string
	// Canonical is the resolved absolute path on the host filesystem, or the
	// validated source path when the conversion reads from a source. It is
	// empty for a data: URI. It is the identity of a file: two spellings of one
	// path resolve to the same string.
	Canonical string
	// Bytes are the file's contents.
	Bytes []byte
	// Cached reports that an earlier call already read and admitted these
	// bytes, so the adapter does not charge or count them again.
	Cached bool
}

// Registry is the conversion-scoped asset registry. One registry reads images,
// stylesheets and fonts for one conversion, opens each configured root once and
// releases every handle on Close.
//
// A Registry is safe for concurrent use; the adapters of one conversion may
// call it from several goroutines.
type Registry struct {
	guard *budget.Guard
	// source, when set, is the caller-supplied file system every read uses
	// instead of the host filesystem. The registry never closes it.
	source fs.FS

	mu     sync.Mutex
	roots  map[string]*fsx.Root
	cache  map[string]*Record
	totals map[Kind]int64
	pixels int64
	reads  map[Kind]int
	closed bool
}

// New returns a registry for one conversion. A nil guard leaves the work
// unlimited, like the rest of the engine.
func New(guard *budget.Guard) *Registry {
	return &Registry{
		guard:  guard,
		roots:  map[string]*fsx.Root{},
		cache:  map[string]*Record{},
		totals: map[Kind]int64{},
		reads:  map[Kind]int{},
	}
}

// NewWithSource returns a registry for one conversion that reads every asset
// through source instead of the host filesystem. The registry does not take
// ownership of source: it never closes it, and the caller owns its lifetime.
func NewWithSource(guard *budget.Guard, source fs.FS) *Registry {
	r := New(guard)
	r.source = source
	return r
}

// IsSource reports whether the registry reads assets from a caller-supplied
// fs.FS rather than from the host filesystem.
func (r *Registry) IsSource() bool {
	return r != nil && r.source != nil
}

// Close releases every root the registry opened. It is idempotent, so a caller
// can defer it and close early on an error path as well.
func (r *Registry) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var first error
	for _, root := range r.roots {
		if err := root.Close(); err != nil && first == nil {
			first = err
		}
	}
	r.roots = map[string]*fsx.Root{}
	r.cache = map[string]*Record{}
	return first
}

// Closed reports whether Close has run.
func (r *Registry) Closed() bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// Root returns the confined reader of one directory, opening it on first use.
// Two spellings of one directory share one handle. An empty dir returns a root
// without a boundary, the same way fsx.Open("") does.
//
// A source-backed registry has no host directory: Root reports an error instead
// of opening one, so no code path can mix a source with host reads.
func (r *Registry) Root(dir string) (*fsx.Root, error) {
	if r != nil && r.source != nil {
		return nil, errors.New("a source-backed asset registry has no host directory")
	}
	if r == nil {
		return fsx.Open(dir)
	}
	key := ""
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		key = abs
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("the asset registry is closed")
	}
	if root, ok := r.roots[key]; ok {
		return root, nil
	}
	root, err := fsx.Open(dir)
	if err != nil {
		return nil, err
	}
	r.roots[key] = root
	return root, nil
}

// Roots reports how many confined readers are open. Tests use it to prove that
// a directory is opened once however many references read from it.
func (r *Registry) Roots() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.roots)
}

// Reads reports how many times a kind was read successfully. Tests use it to
// prove that one file was read once however many references named it.
func (r *Registry) Reads(kind Kind) int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads[kind]
}

// Read resolves one reference, enforces its access policy, returns its bytes
// and reserves their size against the conversion total. A reference whose
// record is already cached returns the cached record without reading again.
//
// The bytes are reserved, not committed: the adapter calls Discard when it
// cannot use them, so a failed parse does not consume the total a later valid
// asset needs.
func (r *Registry) Read(req Request) (*Record, error) {
	if r != nil && r.source != nil {
		return r.readSource(req)
	}
	path, canonical, explicit, err := r.locate(req)
	if err != nil {
		return nil, err
	}
	key := req.KeyFor(path, canonical)
	if rec, ok := r.cached(key); ok {
		return rec, nil
	}
	size, err := r.size(req, canonical, path, explicit)
	if err != nil {
		return nil, err
	}
	if req.MaxBytes > 0 && size > int64(req.MaxBytes) {
		return nil, &Error{Kind: req.Kind, Reference: req.Reference,
			Err: fmt.Errorf("%w: the file is %d bytes, the limit is %d", ErrTooLarge, size, req.MaxBytes)}
	}
	if err := r.reserve(req.Kind, size, req.MaxTotalBytes); err != nil {
		return nil, &Error{Kind: req.Kind, Reference: req.Reference, Err: err}
	}
	bytes, err := r.readBytes(req, canonical, explicit)
	if err != nil {
		r.refund(req.Kind, size)
		return nil, &Error{Kind: req.Kind, Reference: req.Reference, Err: err}
	}
	rec := &Record{Kind: req.Kind, Key: key, Path: path, Canonical: canonical, Bytes: bytes}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		r.refund(req.Kind, size)
		return nil, &Error{Kind: req.Kind, Reference: req.Reference, Err: fmt.Errorf("the asset registry is closed")}
	}
	r.cache[key] = rec
	r.reads[req.Kind]++
	r.mu.Unlock()
	out := *rec
	return &out, nil
}

// AdmitData records bytes the caller already holds — a data: URI payload — and
// reserves them against the conversion total. The identity is the SHA-256 of
// the payload, so repeating one URI counts once. The caller calls Discard when
// the payload cannot be used.
func (r *Registry) AdmitData(kind Kind, reference string, data []byte, maxBytes int, maxTotalBytes int64) (*Record, error) {
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, &Error{Kind: kind, Reference: reference,
			Err: fmt.Errorf("%w: the payload is %d bytes, the limit is %d", ErrTooLarge, len(data), maxBytes)}
	}
	sum := sha256.Sum256(data)
	key := string(kind) + "|sha256:" + hex.EncodeToString(sum[:])
	if rec, ok := r.cached(key); ok {
		return rec, nil
	}
	if err := r.reserve(kind, int64(len(data)), maxTotalBytes); err != nil {
		return nil, &Error{Kind: kind, Reference: reference, Err: err}
	}
	rec := &Record{Kind: kind, Key: key, Bytes: data}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		r.refundLocked(kind, int64(len(data)))
		return nil, &Error{Kind: kind, Reference: reference, Err: fmt.Errorf("the asset registry is closed")}
	}
	r.cache[key] = rec
	r.reads[kind]++
	out := *rec
	return &out, nil
}

// Discard releases a record the adapter could not use: its bytes are refunded
// from the kind's total and the record stops being shared. A record that was
// already closed by another adapter's success is left alone.
func (r *Registry) Discard(rec *Record) {
	if r == nil || rec == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache[rec.Key] != nil {
		delete(r.cache, rec.Key)
		r.refundLocked(rec.Kind, int64(len(rec.Bytes)))
		r.reads[rec.Kind]--
	}
}

// AdmitPixels reserves n decoded pixels against the conversion total.
func (r *Registry) AdmitPixels(n, max int64) error {
	if r == nil || max <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("the asset registry is closed")
	}
	if r.pixels+n > max {
		return fmt.Errorf("%w: the document's images exceed %d pixels in total", budget.ErrExceeded, max)
	}
	r.pixels += n
	return nil
}

// RefundPixels releases a pixel reservation whose image did not decode.
func (r *Registry) RefundPixels(n int64) {
	if r == nil || n <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pixels -= n
	if r.pixels < 0 {
		r.pixels = 0
	}
}

// Pixels reports the pixels currently admitted.
func (r *Registry) Pixels() int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pixels
}

// KeyFor returns the identity of one file reference: the kind, the canonical
// file identity and the decoding inputs. An adapter that caches a parsed record
// before reading builds the same key with it.
func (req Request) KeyFor(path, canonical string) string {
	identity := "file:" + canonical
	if req.Decoding != "" {
		return string(req.Kind) + "|" + identity + "|" + req.Decoding
	}
	return string(req.Kind) + "|" + identity
}

// locate resolves a request to its path, canonical identity and whether it is a
// caller-selected path that must not be resolved as a URL.
func (r *Registry) locate(req Request) (path, canonical string, explicit bool, err error) {
	if req.Explicit {
		path = req.Reference
		if path == "" {
			return "", "", true, &Error{Kind: req.Kind, Reference: req.Reference, Err: errors.New("empty asset reference")}
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return "", "", true, &Error{Kind: req.Kind, Reference: req.Reference, Err: absErr}
		}
		canonical, absErr = filepath.EvalSymlinks(abs)
		if absErr != nil {
			canonical = filepath.Clean(abs)
		}
		return path, canonical, true, nil
	}
	ref, refErr := asset.Resolve(req.Reference)
	if refErr != nil {
		return "", "", false, &Error{Kind: req.Kind, Reference: req.Reference, Err: refErr}
	}
	if ref.Remote {
		return "", "", false, &Error{Kind: req.Kind, Reference: req.Reference, Err: errors.New("remote references are not supported")}
	}
	if ref.Data {
		return "", "", false, &Error{Kind: req.Kind, Reference: req.Reference, Err: errors.New("a data: URI is read with AdmitData")}
	}
	// The reference is relative to the root it was written against, so the root
	// resolves it; joining the configured directory here would apply the
	// directory twice when it is itself relative.
	named := ref.Path
	if !filepath.IsAbs(named) {
		named = filepath.Join(req.Dir, named)
	}
	if abs, absErr := filepath.Abs(named); absErr == nil {
		named = abs
	}
	root, rootErr := r.Root(req.Dir)
	if rootErr != nil {
		return "", "", false, &Error{Kind: req.Kind, Reference: req.Reference, Path: named, Err: rootErr}
	}
	canonical, rootErr = root.Resolve(ref.Path)
	if rootErr != nil {
		return "", "", false, &Error{Kind: req.Kind, Reference: req.Reference, Path: named, Err: rootErr}
	}
	// A diagnostic names the path as the caller configured it (not the
	// symlink-resolved one), so it points at the directory the author named.
	return named, canonical, false, nil
}

// size returns the size of one asset without reading it.
func (r *Registry) size(req Request, canonical, path string, explicit bool) (int64, error) {
	if explicit {
		info, err := os.Stat(canonical)
		if err != nil {
			return 0, &Error{Kind: req.Kind, Reference: req.Reference, Path: path, Err: err}
		}
		return info.Size(), nil
	}
	root, err := r.Root(req.Dir)
	if err != nil {
		return 0, &Error{Kind: req.Kind, Reference: req.Reference, Path: path, Err: err}
	}
	size, err := root.Size(canonical)
	if err != nil {
		return 0, &Error{Kind: req.Kind, Reference: req.Reference, Path: path, Err: err}
	}
	return size, nil
}

// readBytes reads one asset: through the root handle for a document reference,
// from the file system for a caller-selected path.
func (r *Registry) readBytes(req Request, canonical string, explicit bool) ([]byte, error) {
	if explicit {
		data, err := fsx.ReadFileLimit(canonical, req.MaxBytes)
		if errors.Is(err, fsx.ErrTooLarge) {
			return nil, fmt.Errorf("%w: %v", ErrTooLarge, err)
		}
		return data, err
	}
	root, err := r.Root(req.Dir)
	if err != nil {
		return nil, err
	}
	// The limit is applied to the read as well as to the size check above, so a
	// file that grows between the two cannot allocate past the limit.
	data, err := root.ReadLimit(canonical, req.MaxBytes)
	if errors.Is(err, fsx.ErrTooLarge) {
		return nil, fmt.Errorf("%w: %v", ErrTooLarge, err)
	}
	return data, err
}

// cached returns a committed record, marked as cached.
func (r *Registry) cached(key string) (*Record, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.cache[key]
	if !ok {
		return nil, false
	}
	out := *rec
	out.Cached = true
	return &out, true
}

// reserve adds n to the kind's total, checking the limit first.
func (r *Registry) reserve(kind Kind, n, max int64) error {
	if r == nil {
		return nil
	}
	if max <= 0 {
		// The bytes are still committed so a later limit change cannot be
		// bypassed by a read that started without a limit.
		r.mu.Lock()
		r.totals[kind] += n
		r.mu.Unlock()
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("the asset registry is closed")
	}
	if r.totals[kind]+n > max {
		return fmt.Errorf("%w: the document's %s exceed %d bytes in total", budget.ErrExceeded, kind, max)
	}
	r.totals[kind] += n
	return nil
}

// refund releases n from the kind's total.
func (r *Registry) refund(kind Kind, n int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refundLocked(kind, n)
}

func (r *Registry) refundLocked(kind Kind, n int64) {
	r.totals[kind] -= n
	if r.totals[kind] < 0 {
		r.totals[kind] = 0
	}
}

// Total reports the bytes currently committed for one kind.
func (r *Registry) Total(kind Kind) int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.totals[kind]
}
