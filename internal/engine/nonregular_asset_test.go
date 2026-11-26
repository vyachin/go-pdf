package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/testfifo"
)

// convertRejectsNonRegular runs one conversion and requires the resource's
// load diagnostic, no PDF and the non-regular cause in the message. A FIFO that
// is opened without the gate instead waits in open(2) until the test timeout.
func convertRejectsNonRegular(t *testing.T, src []byte, opts Options, code string) {
	t.Helper()
	res, err := Convert(context.Background(), src, opts)
	if err == nil {
		t.Fatalf("Convert accepted a non-regular asset: %v", res.Diagnostics.Lines())
	}
	if len(res.PDF) != 0 {
		t.Fatal("a PDF was produced for a non-regular asset")
	}
	if !hasCode(res.Diagnostics, code) {
		t.Fatalf("diagnostics = %v, want %s", res.Diagnostics.Lines(), code)
	}
	if !strings.Contains(strings.Join(res.Diagnostics.Lines(), "\n"), "not a regular file") {
		t.Fatalf("the diagnostic does not name a non-regular file: %v", res.Diagnostics.Lines())
	}
}

// TestFIFOAssetsAreRejected guards the end-to-end contract of the change: an
// image, a stylesheet, an @font-face source and an explicit FallbackFont that
// name a FIFO all report the resource's load diagnostic, produce no PDF and
// return instead of waiting for a writer.
func TestFIFOAssetsAreRejected(t *testing.T) {
	fontPath := testFontPath(t)
	newFIFO := func(t *testing.T) (dir, fifo string) {
		t.Helper()
		dir = t.TempDir()
		fifo = filepath.Join(dir, "probe.fifo")
		testfifo.Create(t, fifo)
		return dir, fifo
	}

	t.Run("image", func(t *testing.T) {
		dir, _ := newFIFO(t)
		src := []byte(`<p><img src="probe.fifo"></p>`)
		convertRejectsNonRegular(t, src, Options{File: "fifo.html", BaseDir: dir, FallbackFont: fontPath}, "img/load")
	})
	t.Run("stylesheet", func(t *testing.T) {
		dir, _ := newFIFO(t)
		src := []byte(`<link rel="stylesheet" href="probe.fifo"><p>x</p>`)
		convertRejectsNonRegular(t, src, Options{File: "fifo.html", BaseDir: dir, FallbackFont: fontPath}, "css/link-load")
	})
	t.Run("font face", func(t *testing.T) {
		dir, _ := newFIFO(t)
		src := []byte(`<style>@font-face { font-family: Probe; src: url(probe.fifo) } p { font-family: Probe }</style><p>x</p>`)
		convertRejectsNonRegular(t, src, Options{File: "fifo.html", BaseDir: dir, FontDir: dir, FallbackFont: fontPath}, "font/unavailable")
	})
	t.Run("fallback font", func(t *testing.T) {
		_, fifo := newFIFO(t)
		src := []byte(`<p>x</p>`)
		convertRejectsNonRegular(t, src, Options{File: "fifo.html", FallbackFont: fifo}, "font/unavailable")
	})
	t.Run("image through a source", func(t *testing.T) {
		dir, _ := newFIFO(t)
		src := []byte(`<img src="probe.fifo">`)
		opts := Options{File: "fifo.html", Source: os.DirFS(dir)}
		convertRejectsNonRegular(t, src, opts, "img/load")
	})
}

// TestDirectoryAssetIsRejected guards the other non-regular kind through the
// same three adapters: a directory reference reports the resource's load
// diagnostic and produces no PDF.
func TestDirectoryAssetIsRejected(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "probe.dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Run("image", func(t *testing.T) {
		src := []byte(`<p><img src="probe.dir"></p>`)
		convertRejectsNonRegular(t, src, Options{File: "dir.html", BaseDir: dir, FallbackFont: fontPath}, "img/load")
	})
	t.Run("stylesheet", func(t *testing.T) {
		src := []byte(`<link rel="stylesheet" href="probe.dir"><p>x</p>`)
		convertRejectsNonRegular(t, src, Options{File: "dir.html", BaseDir: dir, FallbackFont: fontPath}, "css/link-load")
	})
	t.Run("font face", func(t *testing.T) {
		src := []byte(`<style>@font-face { font-family: Probe; src: url(probe.dir) } p { font-family: Probe }</style><p>x</p>`)
		convertRejectsNonRegular(t, src, Options{File: "dir.html", BaseDir: dir, FontDir: dir, FallbackFont: fontPath}, "font/unavailable")
	})
}

// TestAssetSymlinkClassification guards RV5-08's symlink rule through Convert:
// a symlink to a regular file inside the root is read and the conversion
// succeeds, a symlink to a FIFO is rejected as non-regular, and a symlink whose
// target is outside the root is still a path escape.
func TestAssetSymlinkClassification(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	outsideDir := t.TempDir()
	regular := filepath.Join(dir, "logo.png")
	writeNoisyPNG(t, regular, 4, 4)
	outside := filepath.Join(outsideDir, "secret.png")
	writeNoisyPNG(t, outside, 4, 4)
	regularLink := filepath.Join(dir, "regular.png")
	if err := os.Symlink(regular, regularLink); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	outsideLink := filepath.Join(dir, "outside.png")
	if err := os.Symlink(outside, outsideLink); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	t.Run("regular target inside the root", func(t *testing.T) {
		src := []byte(`<p><img src="regular.png"></p>`)
		res, err := Convert(context.Background(), src, Options{
			File: "symlink.html", BaseDir: dir, FallbackFont: fontPath,
		})
		if err != nil {
			t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
		if len(res.PDF) == 0 {
			t.Fatal("a symlink to a regular file produced no PDF")
		}
	})
	t.Run("FIFO target inside the root", func(t *testing.T) {
		fifo := filepath.Join(dir, "probe.fifo")
		testfifo.Create(t, fifo)
		link := filepath.Join(dir, "fifo.png")
		if err := os.Symlink(fifo, link); err != nil {
			t.Skipf("symlinks are not available: %v", err)
		}
		src := []byte(`<p><img src="fifo.png"></p>`)
		convertRejectsNonRegular(t, src, Options{
			File: "symlink.html", BaseDir: dir, FallbackFont: fontPath,
		}, "img/load")
	})
	t.Run("target outside the root", func(t *testing.T) {
		src := []byte(`<p><img src="outside.png"></p>`)
		res, err := Convert(context.Background(), src, Options{
			File: "symlink.html", BaseDir: dir, FallbackFont: fontPath,
		})
		if err == nil {
			t.Fatalf("a symlink outside the root was accepted: %v", res.Diagnostics.Lines())
		}
		if !hasCode(res.Diagnostics, "img/load") {
			t.Fatalf("diagnostics = %v, want img/load", res.Diagnostics.Lines())
		}
		if !strings.Contains(strings.Join(res.Diagnostics.Lines(), "\n"), "outside the base directory") {
			t.Fatalf("the diagnostic does not report the escape: %v", res.Diagnostics.Lines())
		}
	})
}
