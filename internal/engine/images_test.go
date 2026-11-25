package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// imageData returns the header and base64 payload of a data: URI for a tiny
// valid image of the given media type.
func imageData(t *testing.T, media string) (string, string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	var buf bytes.Buffer
	var err error
	switch media {
	case "image/png":
		err = png.Encode(&buf, img)
	case "image/jpeg":
		err = jpeg.Encode(&buf, img, nil)
	default:
		t.Fatalf("no encoder for %q", media)
	}
	if err != nil {
		t.Fatal(err)
	}
	return media + ";base64", base64.StdEncoding.EncodeToString(buf.Bytes())
}

// chunked inserts whitespace between base64 chunks, as a wrapped data: URI in
// a document may contain.
func chunked(s string, n int) string {
	var b strings.Builder
	for len(s) > n {
		b.WriteString(s[:n])
		b.WriteString("\r\n\t ")
		s = s[n:]
	}
	b.WriteString(s)
	return b.String()
}

func TestLoadDataURI(t *testing.T) {
	pngHeader, pngPayload := imageData(t, "image/png")
	jpegHeader, jpegPayload := imageData(t, "image/jpeg")
	uri := func(header, payload string) string { return "data:" + header + "," + payload }
	cases := []struct {
		name string
		src  string
		// format is the decoded image format on success; wantErr is a substring
		// of the error on failure.
		format  string
		wantErr string
	}{
		{"png", uri(pngHeader, pngPayload), "png", ""},
		{"jpeg", uri(jpegHeader, jpegPayload), "jpeg", ""},
		{"jpg alias", uri(strings.Replace(jpegHeader, "image/jpeg", "image/jpg", 1), jpegPayload), "jpeg", ""},
		{"parameters", uri(strings.Replace(pngHeader, ";base64", ";charset=utf-8;base64", 1), pngPayload), "png", ""},
		{"uppercase header", uri(strings.ToUpper(pngHeader), pngPayload), "png", ""},
		{"whitespace", "data: " + pngHeader + " , " + chunked(pngPayload, 16), "png", ""},

		{"no comma", "data:" + pngHeader, "", "missing ','"},
		{"no base64 marker", uri("image/png", pngPayload), "", "must be base64-encoded"},
		{"empty payload", uri(pngHeader, ""), "", "unexpected EOF"},
		{"unknown media type", uri("image/svg+xml;base64", "PHN2Zy8+"), "", "unsupported data: URI image type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loader := &imageLoader{}
			img, err := loader.loadDataURI(tc.src)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("decoded %q, want an error mentioning %q", tc.src, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if img.Format != tc.format {
				t.Fatalf("format = %q, want %q", img.Format, tc.format)
			}
			if len(img.Data) == 0 {
				t.Fatal("no image data")
			}
		})
	}
}

// TestImageIdentityIsCanonicalPath guards the cache key: different spellings of
// one file must share one decode and one ImageData.
func TestImageIdentityIsCanonicalPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	loader := &imageLoader{baseDir: dir}
	defer func() { _ = loader.Close() }()

	first, err := loader.Load("a.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, spelling := range []string{"./a.png", "sub/../a.png", "./sub/../a.png"} {
		img, err := loader.Load(spelling)
		if err != nil {
			t.Fatalf("%s: %v", spelling, err)
		}
		if img != first {
			t.Fatalf("%s decoded a second copy", spelling)
		}
	}
	if loader.loads != 1 {
		t.Fatalf("decoded %d times, want 1", loader.loads)
	}
}

// TestImageIdentityIsDataURIBytes guards the data: identity: the same bytes in
// two spellings share one decode and one ImageData.
func TestImageIdentityIsDataURIBytes(t *testing.T) {
	header, payload := imageData(t, "image/png")
	loader := &imageLoader{}
	first, err := loader.Load("data:" + header + "," + payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loader.Load("data: " + header + " , " + chunked(payload, 16))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("the second spelling decoded a copy")
	}
	if loader.loads != 1 {
		t.Fatalf("decoded %d times, want 1", loader.loads)
	}
}

// TestImageTotalBytesLimit guards the aggregate byte cap: the size is checked
// before the file is opened or decoded.
func TestImageTotalBytesLimit(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	info, err := os.Stat(filepath.Join(dir, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	loader := &imageLoader{baseDir: dir, maxTotalBytes: int(info.Size()) - 1}
	defer func() { _ = loader.Close() }()
	if _, err := loader.Load("a.png"); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("Load returned %v, want budget.ErrExceeded", err)
	}
	if loader.loads != 0 {
		t.Fatalf("the rejected image was decoded %d times", loader.loads)
	}
}

// TestImageTotalPixelsLimit guards the aggregate pixel cap.
func TestImageTotalPixelsLimit(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	loader := &imageLoader{baseDir: dir, maxTotalPixels: 1}
	defer func() { _ = loader.Close() }()
	if _, err := loader.Load("a.png"); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("Load returned %v, want budget.ErrExceeded", err)
	}
	if loader.loads != 0 {
		t.Fatalf("the rejected image was decoded %d times", loader.loads)
	}
}

// TestWideImageIntrinsicSizeDoesNotWrap guards the intrinsic conversion: a
// 3,000,000 x 2 PNG is legal under the pixel limit, and 3,000,000 px at
// 0.75 pt each exceeds int32 millipoints, which used to wrap to a negative
// width and reach the PDF as a negative image scale.
func TestWideImageIntrinsicSizeDoesNotWrap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	const width, height = 3_000_000, 2
	img := image.NewGray(image.Rect(0, 0, width, height))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	loader := &imageLoader{baseDir: dir}
	defer func() { _ = loader.Close() }()
	got, err := loader.Load("wide.png")
	if err != nil {
		t.Fatal(err)
	}
	if got.Width <= 0 || got.Height <= 0 {
		t.Fatalf("intrinsic size = %d x %d, want positive", got.Width, got.Height)
	}
	if got.PixelW != width || got.PixelH != height {
		t.Fatalf("pixel size = %d x %d, want %d x %d", got.PixelW, got.PixelH, width, height)
	}
	res, err := Convert(context.Background(), []byte(`<img src="wide.png">`), Options{
		BaseDir:      dir,
		File:         "wide.html",
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	for _, stream := range pdfdebug.ContentStreams(res.PDF) {
		if bytes.Contains(stream, []byte("q -")) {
			t.Fatalf("the image scale is negative: %q", stream)
		}
	}
}
