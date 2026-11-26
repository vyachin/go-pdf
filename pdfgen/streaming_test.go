package pdfgen_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/pdfgen"
)

// streamingDocument builds a document that exercises every object the streamed
// serialiser orders: an @font-face font, an image, two headings that become an
// outline and a forced page break.
func streamingDocument(t *testing.T) ([]byte, pdfgen.Options) {
	t.Helper()
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 220, 30, 30, 255
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	font, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	src := fmt.Sprintf(`<html><head><title>Streaming</title>
<style>
  @page { size: A4; margin: 20mm }
  @font-face { font-family: "Bundled"; src: url(%s) }
  body { font-family: "Bundled"; font-size: 12pt }
  h1 { font-size: 18pt }
  h2 { font-size: 14pt }
</style></head>
<body>
  <h1>Первый раздел</h1>
  <p>Текст первой страницы для потоковой записи документа.</p>
  <img src="logo.png" width="20pt">
  <p style="break-after:page">Конец первой страницы.</p>
  <h2>Второй раздел</h2>
  <p>Текст второй страницы для потоковой записи документа.</p>
</body></html>`, font)
	return []byte(src), pdfgen.Options{
		File:         "stream.html",
		BaseDir:      dir,
		FontDir:      filepath.Dir(font),
		FallbackFont: font,
	}
}

// hasCode reports whether the diagnostics carry code.
func hasCode(diags pdfgen.Diagnostics, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

// TestConvertToMatchesConvert guards the byte-equality contract: the streamed
// document equals Convert's result and both entry points report the same
// diagnostics. The document covers an image, an @font-face font, an outline and
// a page break.
func TestConvertToMatchesConvert(t *testing.T) {
	src, opts := streamingDocument(t)
	want, err := pdfgen.Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(want.Diagnostics.Lines(), "\n"))
	}
	var dst bytes.Buffer
	got, err := pdfgen.ConvertTo(context.Background(), &dst, src, opts)
	if err != nil {
		t.Fatalf("ConvertTo: %v\n%s", err, strings.Join(got.Diagnostics.Lines(), "\n"))
	}
	if !bytes.Equal(dst.Bytes(), want.PDF) {
		t.Fatalf("ConvertTo wrote %d bytes, Convert returned %d; they differ", dst.Len(), len(want.PDF))
	}
	if got.PDF != nil {
		t.Fatalf("ConvertTo returned a %d-byte Result.PDF, want nil", len(got.PDF))
	}
	if !slices.Equal(got.Diagnostics, want.Diagnostics) {
		t.Fatalf("diagnostics differ:\nConvertTo: %v\nConvert:   %v", got.Diagnostics.Lines(), want.Diagnostics.Lines())
	}
	// The fixture must really exercise every object the task names.
	pdf := dst.Bytes()
	for _, marker := range []string{"/Subtype /Image", "/FontFile2", "/Type /Outlines"} {
		if !bytes.Contains(pdf, []byte(marker)) {
			t.Fatalf("the fixture has no %s object", marker)
		}
	}
	if pages := bytes.Count(pdf, []byte("/Type /Page ")); pages < 2 {
		t.Fatalf("the fixture produced %d pages, want at least 2", pages)
	}
}

// errorAfterWriter accepts limit bytes and then fails with err.
type errorAfterWriter struct {
	limit   int
	written int
	err     error
}

func (w *errorAfterWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, w.err
	}
	if len(p) > remaining {
		w.written += remaining
		return remaining, w.err
	}
	w.written += len(p)
	return len(p), nil
}

// TestConvertToWriterError guards the writer-failure scenario: the error is
// reported as pdf/write, no Result carries a PDF and the call does not panic.
func TestConvertToWriterError(t *testing.T) {
	src, opts := streamingDocument(t)
	errBoom := errors.New("socket closed")
	w := &errorAfterWriter{limit: 2048, err: errBoom}
	res, err := pdfgen.ConvertTo(context.Background(), w, src, opts)
	if err == nil {
		t.Fatal("ConvertTo accepted a writer that failed")
	}
	if res.PDF != nil {
		t.Fatalf("a failed conversion returned a %d-byte PDF", len(res.PDF))
	}
	if !hasCode(res.Diagnostics, "pdf/write") {
		t.Fatalf("the writer failure was reported as %v, want pdf/write", res.Diagnostics.Lines())
	}
	if w.written == 0 {
		t.Fatal("the writer received no bytes before the page objects")
	}
}

// cancelOnPageWriter cancels the conversion as soon as a page object reaches
// the writer, which is deterministic without a timer.
type cancelOnPageWriter struct {
	cancel   context.CancelFunc
	buf      bytes.Buffer
	canceled bool
}

func (w *cancelOnPageWriter) Write(p []byte) (int, error) {
	if !w.canceled && bytes.Contains(p, []byte("/Type /Page ")) {
		w.cancel()
		w.canceled = true
	}
	return w.buf.Write(p)
}

// TestConvertToStopsAtPageBoundaryOnCancellation guards the cancellation
// scenario: the context error is returned and the writer stops at the next page
// boundary instead of finishing the document.
func TestConvertToStopsAtPageBoundaryOnCancellation(t *testing.T) {
	src, opts := streamingDocument(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &cancelOnPageWriter{cancel: cancel}
	res, err := pdfgen.ConvertTo(ctx, w, src, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ConvertTo returned %v, want context.Canceled", err)
	}
	if res.PDF != nil {
		t.Fatalf("a cancelled conversion returned a %d-byte PDF", len(res.PDF))
	}
	if !w.canceled {
		t.Fatal("the writer never reached a page object")
	}
	if pages := bytes.Count(w.buf.Bytes(), []byte("/Type /Page ")); pages != 1 {
		t.Fatalf("the writer received %d page objects, want exactly 1 before the stop", pages)
	}
}

// TestConvertToPreviewDoesNotChangeTheStream guards the preview scenario: the
// first-page raster is returned while the streamed bytes are unchanged.
func TestConvertToPreviewDoesNotChangeTheStream(t *testing.T) {
	src, opts := streamingDocument(t)
	var plain bytes.Buffer
	if _, err := pdfgen.ConvertTo(context.Background(), &plain, src, opts); err != nil {
		t.Fatal(err)
	}
	opts.RenderPNG = true
	var preview bytes.Buffer
	res, err := pdfgen.ConvertTo(context.Background(), &preview, src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(res.PNG, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("Result.PNG is not a PNG: %q", res.PNG[:min(8, len(res.PNG))])
	}
	if !bytes.Equal(preview.Bytes(), plain.Bytes()) {
		t.Fatalf("RenderPNG changed the streamed bytes: %d against %d", preview.Len(), plain.Len())
	}
}

// TestConvertToInvalidTemplateWritesNothing guards the partial-output rule: a
// validation error is reported before the first byte.
func TestConvertToInvalidTemplateWritesNothing(t *testing.T) {
	var dst bytes.Buffer
	res, err := pdfgen.ConvertTo(context.Background(), &dst,
		[]byte(`<p style="flex: 1">x</p>`), testOptions(t))
	if err == nil {
		t.Fatal("ConvertTo accepted a property outside the profile")
	}
	if res.PDF != nil {
		t.Fatalf("an invalid template returned a %d-byte PDF", len(res.PDF))
	}
	if dst.Len() != 0 {
		t.Fatalf("a validation error wrote %d bytes before the first byte contract", dst.Len())
	}
}

// TestConvertToPreviewFailureWritesNothing guards the order of the preview and
// the write: a raster failure is reported before the first byte, so a failed
// streamed conversion never leaves a complete document in the writer.
func TestConvertToPreviewFailureWritesNothing(t *testing.T) {
	opts := testOptions(t)
	opts.RenderPNG = true
	opts.Limits.MaxRasterPixels = 1
	var dst bytes.Buffer
	res, err := pdfgen.ConvertTo(context.Background(), &dst, []byte(`<p>A</p>`), opts)
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") {
		t.Fatalf("the raster limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	if res.PNG != nil || res.PDF != nil {
		t.Fatalf("a failed preview returned PNG=%d PDF=%d bytes", len(res.PNG), len(res.PDF))
	}
	if dst.Len() != 0 {
		t.Fatalf("a failed preview wrote %d bytes before the first byte contract", dst.Len())
	}
}
