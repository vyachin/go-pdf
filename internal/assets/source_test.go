package assets

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/fsx"
)

// TestSourcePathResolution pins the source namespace: a reference is a URL
// path that is cleaned, must stay relative and must not climb out of the
// directory it was written against. An empty BaseDir names the source root.
func TestSourcePathResolution(t *testing.T) {
	for _, tc := range []struct {
		name string
		dir  string
		ref  string
		want string
	}{
		{"root file", "", "logo.png", "logo.png"},
		{"root directory", "", "sub/logo.png", "sub/logo.png"},
		{"dot prefix", "", "./logo.png", "logo.png"},
		{"cancelled traversal", "", "sub/../logo.png", "logo.png"},
		{"base directory", "assets", "logo.png", "assets/logo.png"},
		{"base directory and subdirectory", "assets", "sub/logo.png", "assets/sub/logo.png"},
		{"trailing slash on the directory", "assets/", "logo.png", "assets/logo.png"},
		{"trailing slash on the reference", "", "logo.png/", "logo.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SourcePath(tc.dir, tc.ref)
			if err != nil {
				t.Fatalf("SourcePath(%q, %q) failed: %v", tc.dir, tc.ref, err)
			}
			if got != tc.want {
				t.Fatalf("SourcePath(%q, %q) = %q, want %q", tc.dir, tc.ref, got, tc.want)
			}
			if !fs.ValidPath(got) {
				t.Fatalf("SourcePath returned the invalid source name %q", got)
			}
		})
	}
	for _, tc := range []struct {
		name string
		dir  string
		ref  string
	}{
		{"parent escape from the root", "", "../secret.png"},
		{"parent escape from a directory", "assets", "../secret.png"},
		{"deep parent escape", "assets", "sub/../../secret.png"},
		{"absolute reference", "", "/etc/hosts"},
		{"absolute reference inside a directory", "assets", "/etc/hosts"},
		{"empty reference", "", ""},
		{"current directory reference", "", "."},
		{"escaping base directory", "../assets", "logo.png"},
		{"absolute base directory", "/assets", "logo.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SourcePath(tc.dir, tc.ref)
			if !errors.Is(err, fsx.ErrOutsideRoot) {
				t.Fatalf("SourcePath(%q, %q) = %q, %v; want an outside-root error", tc.dir, tc.ref, got, err)
			}
			if got != "" {
				t.Fatalf("a rejected reference returned the name %q", got)
			}
		})
	}
}

// TestSourceDirValidation guards the configured directories: a source root
// defaults to the caller's root, and a configured directory must exist as a
// directory inside the source.
func TestSourceDirValidation(t *testing.T) {
	source := fstest.MapFS{
		"assets/logo.png": {Data: []byte("png")},
		"fonts":           {Data: []byte("not a directory")},
	}
	for _, tc := range []struct {
		name string
		dir  string
		want string
	}{
		{"root by default", "", ""},
		{"root by name", ".", ""},
		{"directory", "assets", "assets"},
		{"trailing slash", "assets/", "assets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SourceDir(source, tc.dir)
			if err != nil {
				t.Fatalf("SourceDir(%q) failed: %v", tc.dir, err)
			}
			if got != tc.want {
				t.Fatalf("SourceDir(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name string
		dir  string
	}{
		{"missing", "missing"},
		{"a file", "assets/logo.png"},
		{"parent escape", "../assets"},
		{"absolute", "/assets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := SourceDir(source, tc.dir); err == nil {
				t.Fatalf("SourceDir(%q) accepted an unusable directory", tc.dir)
			}
		})
	}
}

// TestSourceReadConfinesAndAccounts covers the reader: bytes come from the
// source, an escaping name is refused, the limits are applied and one file
// named twice is one record.
func TestSourceReadConfinesAndAccounts(t *testing.T) {
	source := fstest.MapFS{
		"a.png":     {Data: []byte("image bytes")},
		"sub/b.png": {Data: []byte("second")},
	}
	reg := NewWithSource(nil, source)
	defer func() { _ = reg.Close() }()
	if !reg.IsSource() {
		t.Fatal("the registry does not report its source")
	}
	first, err := reg.Read(Request{Kind: Image, Reference: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Bytes) != "image bytes" || first.Path != "a.png" || first.Canonical != "a.png" {
		t.Fatalf("unexpected record: %+v", first)
	}
	second, err := reg.Read(Request{Kind: Image, Reference: "./a.png?v=2#fragment"})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || second.Key != first.Key {
		t.Fatal("two spellings of one source file were read twice")
	}
	if got := reg.Reads(Image); got != 1 {
		t.Fatalf("read %d times, want 1", got)
	}
	if _, err := reg.Read(Request{Kind: Image, Reference: "../secret.png"}); !errors.Is(err, fsx.ErrOutsideRoot) {
		t.Fatalf("an escaping reference was accepted: %v", err)
	}
	if _, err := reg.Read(Request{Kind: Image, Reference: "/etc/hosts"}); !errors.Is(err, fsx.ErrOutsideRoot) {
		t.Fatalf("an absolute reference was accepted: %v", err)
	}
	if _, err := reg.Read(Request{Kind: Image, Reference: "missing.png"}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing source file did not keep its cause: %v", err)
	}
}

// TestSourceReadEnforcesLimits guards the per-file and total limits, including
// the refund after a failed read.
func TestSourceReadEnforcesLimits(t *testing.T) {
	source := fstest.MapFS{
		"large.png": {Data: []byte("0123456789")},
		"good.png":  {Data: []byte("0123456789")},
	}
	reg := NewWithSource(nil, source)
	defer func() { _ = reg.Close() }()
	if _, err := reg.Read(Request{Kind: Image, Reference: "large.png", MaxBytes: 5}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("the per-file limit was not enforced: %v", err)
	}
	first, err := reg.Read(Request{Kind: Image, Reference: "good.png", MaxTotalBytes: 15})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Read(Request{Kind: Image, Reference: "large.png", MaxTotalBytes: 15}); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("the total was not enforced: %v", err)
	}
	reg.Discard(first)
	if got := reg.Total(Image); got != 0 {
		t.Fatalf("after discard the total is %d", got)
	}
}

// TestSourceExplicitPathIsFromTheRoot guards FallbackFont-like paths: a caller
// path is confined to the source root, not to a document root.
func TestSourceExplicitPathIsFromTheRoot(t *testing.T) {
	source := fstest.MapFS{
		"fonts/body.ttf": {Data: []byte("font bytes")},
	}
	reg := NewWithSource(nil, source)
	defer func() { _ = reg.Close() }()
	rec, err := reg.Read(Request{Kind: Font, Reference: "fonts/body.ttf", Explicit: true, Dir: "images"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Path != "fonts/body.ttf" {
		t.Fatalf("the explicit path resolved to %q", rec.Path)
	}
	if _, err := reg.Read(Request{Kind: Font, Reference: "../body.ttf", Explicit: true}); !errors.Is(err, fsx.ErrOutsideRoot) {
		t.Fatalf("an explicit escape was accepted: %v", err)
	}
	if _, err := reg.Root("."); err == nil || !strings.Contains(err.Error(), "source-backed") {
		t.Fatalf("a source registry opened a host root: %v", err)
	}
	if got := reg.Roots(); got != 0 {
		t.Fatalf("the source registry opened %d host roots", got)
	}
}
