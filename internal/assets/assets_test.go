package assets

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/testfifo"
)

// writeFile writes data under dir/name and returns the path.
func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestOneReferenceIsReadOnce pins the identity contract: the same file named
// twice — even through different spellings — is read once and shared.
func TestOneReferenceIsReadOnce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.png", []byte("image bytes"))
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	first, err := reg.Read(Request{Kind: Image, Reference: "a.png", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached {
		t.Fatal("the first read is not cached")
	}
	second, err := reg.Read(Request{Kind: Image, Reference: "./a.png?v=2", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached {
		t.Fatal("the second read of one file was not shared")
	}
	if string(second.Bytes) != "image bytes" || second.Key != first.Key {
		t.Fatalf("the records differ: %q %q", first.Key, second.Key)
	}
	if got := reg.Reads(Image); got != 1 {
		t.Fatalf("read %d times, want 1", got)
	}
}

// TestKindsAreDistinct makes sure an image and a font with one path are two
// records: identity includes the kind.
func TestKindsAreDistinct(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.bin", []byte("bytes"))
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	img, err := reg.Read(Request{Kind: Image, Reference: "a.bin", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	fnt, err := reg.Read(Request{Kind: Font, Reference: "a.bin", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if img.Key == fnt.Key {
		t.Fatal("two kinds share one identity")
	}
}

// TestOutsideRootIsRejectedEvenWhenCached guards the policy half of the
// contract: a reference that leaves its own root is refused although the same
// file is already committed under another root.
func TestOutsideRootIsRejectedEvenWhenCached(t *testing.T) {
	base := t.TempDir()
	inner := filepath.Join(base, "inner")
	nested := filepath.Join(inner, "nested")
	writeFile(t, inner, "a.png", []byte("bytes"))
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	// The file is committed under the inner root.
	if _, err := reg.Read(Request{Kind: Image, Reference: "a.png", Dir: inner}); err != nil {
		t.Fatal(err)
	}
	// A reference from a deeper root names the same file but leaves its own
	// root, so it is refused although the identity is cached.
	_, err := reg.Read(Request{Kind: Image, Reference: filepath.Join("..", "a.png"), Dir: nested})
	if err == nil || !errors.Is(err, fsx.ErrOutsideRoot) {
		t.Fatalf("an escape was accepted: %v", err)
	}
}

// TestFailureRefundsTheTotal guards the accounting contract: a reservation is
// released when the adapter discards a record, so a later reference still fits
// the total.
func TestFailureRefundsTheTotal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.png", []byte("0123456789"))
	writeFile(t, dir, "good.png", []byte("0123456789"))
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	bad, err := reg.Read(Request{Kind: Image, Reference: "bad.png", Dir: dir, MaxTotalBytes: 15})
	if err != nil {
		t.Fatal(err)
	}
	// The first read reserved 10 of 15 bytes; the second cannot fit until the
	// first is discarded.
	if _, err := reg.Read(Request{Kind: Image, Reference: "good.png", Dir: dir, MaxTotalBytes: 15}); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("the total was not enforced: %v", err)
	}
	reg.Discard(bad)
	good, err := reg.Read(Request{Kind: Image, Reference: "good.png", Dir: dir, MaxTotalBytes: 15})
	if err != nil {
		t.Fatalf("the refunded total refused a valid read: %v", err)
	}
	if good.Cached {
		t.Fatal("the discarded record was still shared")
	}
	if got := reg.Total(Image); got != 10 {
		t.Fatalf("total = %d, want 10", got)
	}
}

// TestCloseReleasesEveryRoot guards cleanup: Close is idempotent and a later
// read fails instead of using a closed handle.
func TestCloseReleasesEveryRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.png", []byte("bytes"))
	reg := New(nil)
	if _, err := reg.Root(dir); err != nil {
		t.Fatal(err)
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Close is not idempotent: %v", err)
	}
	if !reg.Closed() {
		t.Fatal("the registry is not marked closed")
	}
	if _, err := reg.Read(Request{Kind: Image, Reference: "a.png", Dir: dir}); err == nil {
		t.Fatal("a closed registry read a file")
	}
}

// TestConversionsAreIsolated guards conversion-local state: two registries with
// equal relative names under different roots keep their own records and totals.
func TestConversionsAreIsolated(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeFile(t, a, "logo.png", []byte("aaaa"))
	writeFile(t, b, "logo.png", []byte("bbbbbb"))
	var wg sync.WaitGroup
	results := make([]string, 2)
	keys := make([]string, 2)
	for i, dir := range []string{a, b} {
		wg.Add(1)
		go func(i int, dir string) {
			defer wg.Done()
			reg := New(nil)
			defer func() { _ = reg.Close() }()
			rec, err := reg.Read(Request{Kind: Image, Reference: "logo.png", Dir: dir, MaxTotalBytes: 100})
			if err != nil {
				results[i] = "error: " + err.Error()
				return
			}
			results[i] = string(rec.Bytes)
			keys[i] = rec.Key
		}(i, dir)
	}
	wg.Wait()
	if results[0] != "aaaa" || results[1] != "bbbbbb" {
		t.Fatalf("conversions leaked into each other: %q, %q", results[0], results[1])
	}
	if keys[0] == keys[1] {
		t.Fatal("two roots produced one identity")
	}
}

// TestDataIdentityIsTheDigest checks that a repeated data: payload counts once.
func TestDataIdentityIsTheDigest(t *testing.T) {
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	payload := []byte("same payload")
	first, err := reg.AdmitData(Image, "data:image/png;base64,x", payload, 0, 14)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reg.AdmitData(Image, "data:image/png;base64,x", payload, 0, 14)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || first.Key != second.Key {
		t.Fatal("the repeated payload was admitted twice")
	}
	if got := reg.Total(Image); got != int64(len(payload)) {
		t.Fatalf("total = %d, want %d", got, len(payload))
	}
	reg.Discard(first)
	if got := reg.Total(Image); got != 0 {
		t.Fatalf("after discard the total is %d", got)
	}
}

// TestPixelsAreAdmittedAndRefunded checks the pixel reservation used by images.
func TestPixelsAreAdmittedAndRefunded(t *testing.T) {
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	if err := reg.AdmitPixels(80, 100); err != nil {
		t.Fatal(err)
	}
	if err := reg.AdmitPixels(30, 100); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("the pixel total was not enforced: %v", err)
	}
	reg.RefundPixels(80)
	if got := reg.Pixels(); got != 0 {
		t.Fatalf("pixels = %d after the refund", got)
	}
}

// TestTypedErrorKeepsTheCause checks the failure provenance: the error names the
// kind and the reference and unwraps to its cause.
func TestTypedErrorKeepsTheCause(t *testing.T) {
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	_, err := reg.Read(Request{Kind: Image, Reference: "missing.png", Dir: t.TempDir()})
	var assetErr *Error
	if !errors.As(err, &assetErr) {
		t.Fatalf("error %T is not an *assets.Error", err)
	}
	if assetErr.Kind != Image || assetErr.Reference != "missing.png" {
		t.Fatalf("the error lost its provenance: %+v", assetErr)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the cause is not unwrapped: %v", err)
	}
}

// TestNonRegularReadKeepsTheTypedCause guards the admission gate and the typed
// failure: every kind refuses a FIFO with ErrNotRegular, in a root and for an
// explicit caller-selected path, instead of opening it and waiting for a writer.
func TestNonRegularReadKeepsTheTypedCause(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "probe.fifo")
	testfifo.Create(t, fifo)
	reg := New(nil)
	defer func() { _ = reg.Close() }()
	for _, req := range []Request{
		{Kind: Image, Reference: "probe.fifo", Dir: dir},
		{Kind: Stylesheet, Reference: "probe.fifo", Dir: dir},
		{Kind: Font, Reference: "probe.fifo", Dir: dir},
		{Kind: Font, Reference: fifo, Explicit: true},
	} {
		_, err := reg.Read(req)
		var assetErr *Error
		if !errors.As(err, &assetErr) {
			t.Fatalf("%s read of a FIFO returned %T, want *assets.Error", req.Kind, err)
		}
		if !errors.Is(err, fsx.ErrNotRegular) {
			t.Fatalf("%s read lost the typed cause: %v", req.Kind, err)
		}
	}
}
