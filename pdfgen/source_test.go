package pdfgen_test

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/vyachin/go-pdf/pdfgen"
)

// embeddedSource is the testdata directory laid out the way a caller ships it
// next to the program: the image and the font are inside the binary.
//
//go:embed testdata/source
var embeddedSource embed.FS

// sourceDocument references an image and declares a @font-face source, so one
// conversion reads both asset kinds through the same source.
const sourceDocument = `<html><head><style>
  @font-face { font-family: "SourceFace"; src: url(pacifico.ttf) }
  body { font-family: "SourceFace"; font-size: 12pt }
</style></head>
<body><p>Source assets</p><img src="logo.png"></body></html>`

// embeddedSourceFS returns the asset directory of the embedded source.
func embeddedSourceFS(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(embeddedSource, "testdata/source")
	if err != nil {
		t.Fatalf("sub of the embedded source: %v", err)
	}
	return sub
}

// embeddedAsset reads one asset from the embedded testdata directory.
func embeddedAsset(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(embeddedSource, "testdata/source/"+name)
	if err != nil {
		t.Fatalf("reading embedded %s: %v", name, err)
	}
	return data
}

// writeEmbeddedAssets writes the embedded image and font into dir, so the same
// bytes can be converted from the host filesystem.
func writeEmbeddedAssets(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"pacifico.ttf", "logo.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), embeddedAsset(t, name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// diagnosticWithCode returns the first diagnostic with code, or nil.
func diagnosticWithCode(list pdfgen.Diagnostics, code string) *pdfgen.Diagnostic {
	for i := range list {
		if list[i].Code == code {
			return &list[i]
		}
	}
	return nil
}

// TestEmbeddedSourceConvertsLikeDirectory covers the delta scenario: a document
// from an embed.FS with an image and a @font-face source converts, and the same
// bytes read from a directory produce a byte-identical PDF.
func TestEmbeddedSourceConvertsLikeDirectory(t *testing.T) {
	dir := t.TempDir()
	writeEmbeddedAssets(t, dir)
	source := embeddedSourceFS(t)

	host, err := pdfgen.Convert(context.Background(), []byte(sourceDocument), pdfgen.Options{
		File:    "embedded.html",
		BaseDir: dir,
	})
	if err != nil {
		t.Fatalf("the directory conversion failed: %v\n%s", err, strings.Join(host.Diagnostics.Lines(), "\n"))
	}
	embedded, err := pdfgen.Convert(context.Background(), []byte(sourceDocument), pdfgen.Options{
		File:   "embedded.html",
		Source: source,
	})
	if err != nil {
		t.Fatalf("the embedded conversion failed: %v\n%s", err, strings.Join(embedded.Diagnostics.Lines(), "\n"))
	}
	if !bytes.HasPrefix(embedded.PDF, []byte("%PDF-")) {
		t.Fatalf("the embedded conversion did not produce a PDF: %q", embedded.PDF[:min(16, len(embedded.PDF))])
	}
	if !bytes.Equal(host.PDF, embedded.PDF) {
		t.Fatal("the source conversion and the directory conversion produced different PDFs")
	}
}

// TestSourceEscapesMatchFilesystemDiagnostics covers the delta scenario: an
// absolute or escaping reference is rejected with the diagnostic the
// filesystem source reports, and no PDF is produced.
func TestSourceEscapesMatchFilesystemDiagnostics(t *testing.T) {
	dir := t.TempDir()
	source := embeddedSourceFS(t)
	hostFont := testFontPath(t)
	for _, tc := range []struct {
		name    string
		src     string
		code    string
		message string
	}{
		{
			name:    "image parent",
			src:     `<img src="../secret.png"><p>x</p>`,
			code:    "img/load",
			message: `image "../secret.png" is outside the base directory`,
		},
		{
			name:    "image absolute",
			src:     `<img src="/etc/hosts"><p>x</p>`,
			code:    "img/load",
			message: `image "/etc/hosts" is outside the base directory`,
		},
		{
			name:    "stylesheet parent",
			src:     `<link rel="stylesheet" href="../secret.css"><p>x</p>`,
			code:    "css/link-path",
			message: `stylesheet path "../secret.css" escapes the base directory`,
		},
		{
			name:    "stylesheet absolute",
			src:     `<link rel="stylesheet" href="/etc/hosts"><p>x</p>`,
			code:    "css/link-path",
			message: `stylesheet path "/etc/hosts" escapes the base directory`,
		},
		{
			name:    "font parent",
			src:     `<style>@font-face{font-family:X;src:url(../secret.ttf)}</style><p>x</p>`,
			code:    "font/unavailable",
			message: `font "../secret.ttf" is outside the font directory`,
		},
		{
			name:    "font absolute",
			src:     `<style>@font-face{font-family:X;src:url(/etc/hosts)}</style><p>x</p>`,
			code:    "font/unavailable",
			message: `font "/etc/hosts" is outside the font directory`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hostRes, hostErr := pdfgen.Convert(context.Background(), []byte(tc.src), pdfgen.Options{
				File:         "escape.html",
				BaseDir:      dir,
				FallbackFont: hostFont,
			})
			srcRes, srcErr := pdfgen.Convert(context.Background(), []byte(tc.src), pdfgen.Options{
				File:         "escape.html",
				Source:       source,
				FallbackFont: "pacifico.ttf",
			})
			if hostErr == nil || srcErr == nil {
				t.Fatalf("an escaping reference was accepted: host err=%v, source err=%v", hostErr, srcErr)
			}
			if len(hostRes.PDF) != 0 || len(srcRes.PDF) != 0 {
				t.Fatal("an escaping reference produced a PDF")
			}
			hostDiag := diagnosticWithCode(hostRes.Diagnostics, tc.code)
			srcDiag := diagnosticWithCode(srcRes.Diagnostics, tc.code)
			if hostDiag == nil || srcDiag == nil {
				t.Fatalf("missing %q: source diags %v; host diags %v",
					tc.code, srcRes.Diagnostics.Lines(), hostRes.Diagnostics.Lines())
			}
			if srcDiag.Message != tc.message {
				t.Fatalf("source diagnostic = %q, want %q", srcDiag.Message, tc.message)
			}
			if srcDiag.Message != hostDiag.Message {
				t.Fatalf("the source diagnostic %q differs from the filesystem one %q", srcDiag.Message, hostDiag.Message)
			}
		})
	}
}

// TestSourceMissingAssetsMatchFilesystemDiagnostics covers the delta scenario:
// a missing source image, stylesheet or font reports the filesystem source's
// diagnostic and names the same reference.
func TestSourceMissingAssetsMatchFilesystemDiagnostics(t *testing.T) {
	font := embeddedAsset(t, "pacifico.ttf")
	source := fstest.MapFS{"pacifico.ttf": {Data: font}}
	dir := t.TempDir()
	hostFont := testFontPath(t)
	for _, tc := range []struct {
		name           string
		src            string
		code           string
		reference      string
		sourceFallback bool
	}{
		{
			name:           "image",
			src:            `<img src="missing.png"><p>x</p>`,
			code:           "img/load",
			reference:      "missing.png",
			sourceFallback: true,
		},
		{
			name:           "stylesheet",
			src:            `<link rel="stylesheet" href="missing.css"><p>x</p>`,
			code:           "css/link-load",
			reference:      "missing.css",
			sourceFallback: true,
		},
		{
			name:      "font",
			src:       `<style>@font-face{font-family:X;src:url(missing.ttf)}</style><p>x</p>`,
			code:      "font/unavailable",
			reference: "missing.ttf",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hostRes, hostErr := pdfgen.Convert(context.Background(), []byte(tc.src), pdfgen.Options{
				File:         "missing.html",
				BaseDir:      dir,
				FallbackFont: hostFont,
			})
			srcOpts := pdfgen.Options{File: "missing.html", Source: source}
			if tc.sourceFallback {
				srcOpts.FallbackFont = "pacifico.ttf"
			}
			srcRes, srcErr := pdfgen.Convert(context.Background(), []byte(tc.src), srcOpts)
			if hostErr == nil || srcErr == nil {
				t.Fatalf("a missing asset was accepted: host err=%v, source err=%v", hostErr, srcErr)
			}
			if len(srcRes.PDF) != 0 {
				t.Fatal("a missing source asset produced a PDF")
			}
			hostDiag := diagnosticWithCode(hostRes.Diagnostics, tc.code)
			srcDiag := diagnosticWithCode(srcRes.Diagnostics, tc.code)
			if hostDiag == nil || srcDiag == nil {
				t.Fatalf("missing %q: source diags %v; host diags %v",
					tc.code, srcRes.Diagnostics.Lines(), hostRes.Diagnostics.Lines())
			}
			if !strings.Contains(srcDiag.Message, tc.reference) {
				t.Fatalf("source diagnostic %q does not name %q", srcDiag.Message, tc.reference)
			}
			if !strings.Contains(hostDiag.Message, tc.reference) {
				t.Fatalf("filesystem diagnostic %q does not name %q", hostDiag.Message, tc.reference)
			}
		})
	}
}

// TestSourceReadsLinkedStylesheet covers the third asset kind: a
// <link rel="stylesheet"> is opened through the source, and a problem inside it
// names the source file.
func TestSourceReadsLinkedStylesheet(t *testing.T) {
	font := embeddedAsset(t, "pacifico.ttf")
	source := fstest.MapFS{
		"styles/print.css": {Data: []byte(`p { color: #ff0000 }`)},
		"styles/bad.css":   {Data: []byte(`p { display: flex }`)},
		"pacifico.ttf":     {Data: font},
	}
	res, err := pdfgen.Convert(context.Background(), []byte(`<link rel="stylesheet" href="styles/print.css"><p>Hi</p>`), pdfgen.Options{
		File:         "link.html",
		Source:       source,
		FallbackFont: "pacifico.ttf",
	})
	if err != nil {
		t.Fatalf("the linked source stylesheet was not read: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}

	res, err = pdfgen.Convert(context.Background(), []byte(`<link rel="stylesheet" href="styles/bad.css"><p>Hi</p>`), pdfgen.Options{
		File:         "link.html",
		Source:       source,
		FallbackFont: "pacifico.ttf",
	})
	if err == nil {
		t.Fatal("an unsupported declaration in a source stylesheet was accepted")
	}
	diag := diagnosticWithCode(res.Diagnostics, "css/unsupported-value")
	if diag == nil {
		t.Fatalf("missing css/unsupported-value: %v", res.Diagnostics.Lines())
	}
	if diag.File != "styles/bad.css" {
		t.Fatalf("the diagnostic names %q, want the source stylesheet", diag.File)
	}
}

// TestFallbackFontComesFromSource covers the delta scenario: FallbackFont is
// read from the source, and the source is authoritative, so neither a host path
// nor the system font list is consulted.
func TestFallbackFontComesFromSource(t *testing.T) {
	font := embeddedAsset(t, "pacifico.ttf")
	source := fstest.MapFS{"fonts/body.ttf": {Data: font}}
	res, err := pdfgen.Convert(context.Background(), []byte(`<p>Hello</p>`), pdfgen.Options{
		File:         "fallback.html",
		Source:       source,
		FallbackFont: "fonts/body.ttf",
	})
	if err != nil {
		t.Fatalf("the source fallback font was not used: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("no PDF was produced")
	}

	// A host path cannot name the fallback while a source is set.
	hostPath, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	res, err = pdfgen.Convert(context.Background(), []byte(`<p>Hello</p>`), pdfgen.Options{
		File:         "fallback.html",
		Source:       source,
		FallbackFont: hostPath,
	})
	if err == nil || diagnosticWithCode(res.Diagnostics, "font/unavailable") == nil {
		t.Fatalf("a host fallback font was read from the source conversion: %v", err)
	}

	// Without a declared or configured font no host font is read: the family
	// is font/unavailable instead of being silently replaced.
	res, err = pdfgen.Convert(context.Background(), []byte(`<p>Hello</p>`), pdfgen.Options{
		File:   "fallback.html",
		Source: source,
	})
	if err == nil || diagnosticWithCode(res.Diagnostics, "font/unavailable") == nil {
		t.Fatalf("a host font was used in a source conversion: %v", err)
	}
}

// TestSourceBaseDirAndFontDir covers the configured directories: BaseDir and
// FontDir are names inside the source and default to its root.
func TestSourceBaseDirAndFontDir(t *testing.T) {
	source := fstest.MapFS{
		"assets/logo.png": {Data: embeddedAsset(t, "logo.png")},
		"fonts/body.ttf":  {Data: embeddedAsset(t, "pacifico.ttf")},
	}
	src := `<html><head><style>
  @font-face { font-family: "Body"; src: url(body.ttf) }
  body { font-family: "Body" }
</style></head><body><img src="logo.png"><p>Hi</p></body></html>`
	res, err := pdfgen.Convert(context.Background(), []byte(src), pdfgen.Options{
		File:    "dirs.html",
		Source:  source,
		BaseDir: "assets",
		FontDir: "fonts",
	})
	if err != nil {
		t.Fatalf("the source directories were not resolved: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}

	// A source FontDir does not fall back to BaseDir: an empty FontDir names
	// the source root.
	res, err = pdfgen.Convert(context.Background(), []byte(src), pdfgen.Options{
		File:    "dirs.html",
		Source:  source,
		BaseDir: "assets",
	})
	if err == nil || diagnosticWithCode(res.Diagnostics, "font/unavailable") == nil {
		t.Fatalf("a source font was found through BaseDir without FontDir: %v", err)
	}

	// A configured directory the source cannot honour is options/asset-root,
	// the code the host filesystem reports for an unusable directory. A host
	// path is not a source path, so it cannot mix a host directory into the
	// conversion.
	hostDir := t.TempDir()
	for _, tc := range []struct {
		name string
		opts pdfgen.Options
	}{
		{"missing base directory", pdfgen.Options{Source: source, BaseDir: "missing"}},
		{"missing font directory", pdfgen.Options{Source: source, FontDir: "missing"}},
		{"escaping base directory", pdfgen.Options{Source: source, BaseDir: "../assets"}},
		{"host base directory", pdfgen.Options{Source: source, BaseDir: hostDir}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.File = "dirs.html"
			res, err := pdfgen.Convert(context.Background(), []byte(`<p>Hi</p>`), tc.opts)
			if err == nil || diagnosticWithCode(res.Diagnostics, "options/asset-root") == nil {
				t.Fatalf("want options/asset-root, got err=%v diags %v", err, res.Diagnostics.Lines())
			}
			if len(res.PDF) != 0 {
				t.Fatal("an unusable source directory produced a PDF")
			}
		})
	}
}

// TestSourceAndFilesystemProduceTheSameBytes covers the determinism promise: the
// same document read from os.DirFS and from the host filesystem produces the
// same PDF, and neither conversion writes or deletes a file.
func TestSourceAndFilesystemProduceTheSameBytes(t *testing.T) {
	dir := t.TempDir()
	writeEmbeddedAssets(t, dir)
	cwdBefore := directoryEntries(t, ".")
	assetsBefore := directoryEntries(t, dir)

	host, err := pdfgen.Convert(context.Background(), []byte(sourceDocument), pdfgen.Options{
		File:    "same.html",
		BaseDir: dir,
		FontDir: dir,
	})
	if err != nil {
		t.Fatalf("the host conversion failed: %v\n%s", err, strings.Join(host.Diagnostics.Lines(), "\n"))
	}
	source, err := pdfgen.Convert(context.Background(), []byte(sourceDocument), pdfgen.Options{
		File:   "same.html",
		Source: os.DirFS(dir),
	})
	if err != nil {
		t.Fatalf("the os.DirFS conversion failed: %v\n%s", err, strings.Join(source.Diagnostics.Lines(), "\n"))
	}
	if !bytes.Equal(host.PDF, source.PDF) {
		t.Fatal("os.DirFS and the host filesystem produced different PDFs")
	}

	if after := directoryEntries(t, "."); !slices.Equal(cwdBefore, after) {
		t.Fatalf("the working directory changed: %v -> %v", cwdBefore, after)
	}
	if after := directoryEntries(t, dir); !slices.Equal(assetsBefore, after) {
		t.Fatalf("the asset directory changed: %v -> %v", assetsBefore, after)
	}
}

// directoryEntries returns the sorted names in dir.
func directoryEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	slices.Sort(names)
	return names
}

// TestNilSourceKeepsTheFilesystem pins the compatibility rule: with no source
// the image and font are read from the host filesystem as before.
func TestNilSourceKeepsTheFilesystem(t *testing.T) {
	dir := t.TempDir()
	writeEmbeddedAssets(t, dir)
	withNil := pdfgen.Options{File: "nil.html", BaseDir: dir, FontDir: dir, Source: nil}
	res, err := pdfgen.Convert(context.Background(), []byte(sourceDocument), withNil)
	if err != nil {
		t.Fatalf("the nil-source conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	without := pdfgen.Options{File: "nil.html", BaseDir: dir, FontDir: dir}
	baseline, err := pdfgen.Convert(context.Background(), []byte(sourceDocument), without)
	if err != nil {
		t.Fatalf("the baseline conversion failed: %v", err)
	}
	if !bytes.Equal(res.PDF, baseline.PDF) {
		t.Fatal("setting Source to nil changed the output")
	}
}

// TestSourceRemoteAndDataReferencesAreRejected keeps the existing refusal of a
// remote or data: image when a source is set.
func TestSourceRemoteAndDataReferencesAreRejected(t *testing.T) {
	source := fstest.MapFS{"pacifico.ttf": {Data: embeddedAsset(t, "pacifico.ttf")}}
	res, err := pdfgen.Convert(context.Background(), []byte(`<img src="https://example.test/a.png"><p>x</p>`), pdfgen.Options{
		File:         "remote.html",
		Source:       source,
		FallbackFont: "pacifico.ttf",
	})
	if err == nil || diagnosticWithCode(res.Diagnostics, "img/load") == nil {
		t.Fatalf("a remote image was accepted: %v", err)
	}
}

// TestSourceKeepsItsOwnRootAndRejectsTheOptIn covers the source-mode
// interaction: a source resolves relative assets from its own root, never from
// the directory of Options.File, and the absolute opt-in does not apply to a
// source namespace.
func TestSourceKeepsItsOwnRootAndRejectsTheOptIn(t *testing.T) {
	source := fstest.MapFS{
		"body.ttf": {Data: embeddedAsset(t, "pacifico.ttf")},
		"logo.png": {Data: embeddedAsset(t, "logo.png")},
	}
	// The host directory the diagnostic name points at holds no assets, so a
	// conversion that derived a host root from File would fail.
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	doc := []byte(`<style>@font-face { font-family: "Body"; src: url(body.ttf) }
body { font-family: "Body" }</style><p>x</p><img src="logo.png">`)
	res, err := pdfgen.Convert(context.Background(), doc, pdfgen.Options{
		File:   filepath.Join(elsewhere, "invoice.html"),
		Source: source,
	})
	if err != nil {
		t.Fatalf("a source did not resolve from its own root: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.Contains(res.PDF, []byte("/Subtype /Image")) {
		t.Fatal("the source image was not embedded")
	}

	// The option admits operating-system paths; a source has none.
	res, err = pdfgen.Convert(context.Background(), []byte(`<img src="/etc/hosts"><p>x</p>`), pdfgen.Options{
		File:                "invoice.html",
		Source:              source,
		AllowAbsoluteAssets: true,
	})
	if err == nil || len(res.PDF) != 0 {
		t.Fatalf("the opt-in admitted an absolute source path: err=%v", err)
	}
	diag := diagnosticWithCode(res.Diagnostics, "img/load")
	if diag == nil {
		t.Fatalf("the absolute source path was not reported as img/load: %v", res.Diagnostics.Lines())
	}
	if !strings.Contains(diag.Message, `"/etc/hosts"`) {
		t.Fatalf("the diagnostic does not name the reference: %q", diag.Message)
	}
}
