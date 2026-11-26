package pdf

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// streamingTestDoc builds a document with a font, an image, an outline and two
// pages: every kind of object the serialiser has to order.
func streamingTestDoc(t *testing.T) *Doc {
	t.Helper()
	f := loadFont(t)
	gid, ok := f.GlyphIndex('С')
	if !ok {
		t.Skip("test font has no С")
	}
	d := New(Options{
		Title:   "Streaming",
		Outline: []OutlineEntry{{Title: "Первый", Page: 1, Level: 1}},
	})
	d.AddPage(595276, 841890, []Op{
		{Kind: FillOpKind, Rect: Rect{X: 1000, Y: 1000, W: 5000, H: 5000}, Color: RGB{R: 255, Set: true}},
		{Kind: TextOpKind, Text: &Text{
			X: 10000, Y: 20000, Font: f, Size: 12000,
			Glyphs: []Glyph{{GID: gid, Rune: 'С'}},
		}},
		{Kind: ImageOpKind, Rect: Rect{X: 1000, Y: 30000, W: 2000, H: 2000}, Image: testImage(t, color.RGBA{R: 255, A: 255})},
	})
	d.AddPage(595276, 841890, []Op{
		{Kind: TextOpKind, Text: &Text{
			X: 10000, Y: 20000, Font: f, Size: 12000,
			Glyphs: []Glyph{{GID: gid, Rune: 'С'}},
		}},
	})
	return d
}

// TestWriteToMatchesBytes guards the single serialiser: the streaming and the
// buffered entry points must produce the same bytes for a document with fonts,
// images and an outline. Bytes delegates to WriteTo, so changing one path alone
// breaks this test.
func TestWriteToMatchesBytes(t *testing.T) {
	d := streamingTestDoc(t)
	want, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := d.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := buf.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("WriteTo produced %d bytes, Bytes produced %d; they differ", len(got), len(want))
	}
	if n != int64(len(want)) {
		t.Fatalf("WriteTo reported %d bytes, wrote %d", n, len(want))
	}
}

// TestWriteToStreamsPagesBeforeFontsAndImages pins the streaming order: a page
// and its content precede the font and image objects, the document-level
// objects follow, and the xref is last.
func TestWriteToStreamsPagesBeforeFontsAndImages(t *testing.T) {
	d := streamingTestDoc(t)
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	index := func(marker string) int {
		i := bytes.Index(data, []byte(marker))
		if i < 0 {
			t.Fatalf("the document has no %q marker", marker)
		}
		return i
	}
	page := index("/Type /Page ")
	font := index("/Subtype /Type0")
	image := index("/Subtype /Image")
	outline := index("/Type /Outlines")
	catalog := index("/Type /Catalog")
	xref := index("\nxref\n")
	if page >= font || font >= image || image >= outline || outline >= catalog || catalog >= xref {
		t.Fatalf("streaming order is page=%d font=%d image=%d outline=%d catalog=%d xref=%d", page, font, image, outline, catalog, xref)
	}
	if !bytes.HasPrefix(data, []byte(pdfHeader)) {
		t.Fatal("the header is not first")
	}
}

var errWriterFailed = errors.New("writer failed")

// failingWriter accepts limit bytes and then reports an error.
type failingWriter struct {
	limit   int
	written int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, errWriterFailed
	}
	if len(p) > remaining {
		w.written += remaining
		return remaining, errWriterFailed
	}
	w.written += len(p)
	return len(p), nil
}

// TestWriteToPropagatesWriterError guards the writer-error contract: the error
// is returned, the panic path does not exist, and the bytes the writer accepted
// are reported.
func TestWriteToPropagatesWriterError(t *testing.T) {
	d := streamingTestDoc(t)
	w := &failingWriter{limit: 2048}
	n, err := d.WriteTo(w)
	if !errors.Is(err, errWriterFailed) {
		t.Fatalf("WriteTo returned %v, want the writer error", err)
	}
	if n <= 0 || n > 2048 {
		t.Fatalf("WriteTo reported %d bytes after a failure at 2048", n)
	}
	if n != int64(w.written) {
		t.Fatalf("WriteTo reported %d bytes, the writer accepted %d", n, w.written)
	}
}

// zeroWriter reports success without accepting a byte.
type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

// TestWriteToShortWriteIsAnError guards the short-write path: a writer that
// makes no progress cannot complete the document, so WriteTo reports
// io.ErrShortWrite instead of looping or returning success.
func TestWriteToShortWriteIsAnError(t *testing.T) {
	d := streamingTestDoc(t)
	if _, err := d.WriteTo(zeroWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("WriteTo returned %v, want io.ErrShortWrite", err)
	}
}

// chunkWriter accepts one byte per call without an error, which the io.Writer
// contract allows only when the call returns a complete write; the helper must
// loop until the object is written.
type chunkWriter struct {
	buf bytes.Buffer
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return w.buf.Write(p[:1])
}

// TestWriteToAcceptsChunkedWrites guards that a writer returning short, nil
// writes still receives the complete document.
func TestWriteToAcceptsChunkedWrites(t *testing.T) {
	d := streamingTestDoc(t)
	want, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	w := &chunkWriter{}
	n, err := d.WriteTo(w)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.buf.Bytes(), want) {
		t.Fatal("a chunked writer received different bytes")
	}
	if n != int64(len(want)) {
		t.Fatalf("WriteTo reported %d bytes, wrote %d", n, len(want))
	}
	if strings.Count(w.buf.String(), "\nendobj\n") == 0 {
		t.Fatal("no object was completed")
	}
}

// cancelOnPageWriter cancels the context when the first page object reaches the
// writer, which makes cancellation deterministic without a timer.
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

// TestWriteToStopsAtPageBoundaryOnCancellation guards the context check between
// pages: the context error is returned and no further page is written.
func TestWriteToStopsAtPageBoundaryOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := streamingTestDoc(t)
	d.opts.Budget = budget.New(ctx, 0)
	w := &cancelOnPageWriter{cancel: cancel}
	if _, err := d.WriteTo(w); !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteTo returned %v, want context.Canceled", err)
	}
	if !w.canceled {
		t.Fatal("the writer never reached a page object")
	}
	if pages := bytes.Count(w.buf.Bytes(), []byte("/Type /Page ")); pages != 1 {
		t.Fatalf("the writer received %d page objects, want exactly 1 before the stop", pages)
	}
}
