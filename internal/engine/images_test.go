package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
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
		{"empty payload", uri(pngHeader, ""), "", "unrecognised format"},
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

// writeTemp writes data under dir with name and returns the path.
func writeTemp(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// readFixture returns one file of the repository's test data.
func readFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{repoRoot(), "testdata"}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestImageFormatComesFromContent guards RV4-21: the format was chosen from the
// extension, so a PNG named .jpg failed and a PNG without an extension was
// unsupported. The magic bytes decide, and a mismatch names the detected format.
func TestImageFormatComesFromContent(t *testing.T) {
	png := readFixture(t, "adversarial", "a.png")
	dir := t.TempDir()
	for _, name := range []string{"misnamed.jpg", "no-extension", "image.PNG"} {
		writeTemp(t, dir, name, png)
	}
	loader := &imageLoader{baseDir: dir}
	defer func() { _ = loader.Close() }()
	for _, name := range []string{"misnamed.jpg", "no-extension", "image.PNG"} {
		img, err := loader.Load(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if img.Format != "png" {
			t.Fatalf("%s loaded as %q, want png", name, img.Format)
		}
	}
	// A genuinely unsupported format is named by its content, not its name.
	gif := append([]byte("GIF89a"), make([]byte, 20)...)
	writeTemp(t, dir, "actually.png", gif)
	if _, err := loader.Load("actually.png"); err == nil || !strings.Contains(err.Error(), "GIF") {
		t.Fatalf("a GIF named .png returned %v, want an error naming GIF", err)
	}
}

// TestAssetReferenceIsAURLPath guards RV4-22: a reference copied from a web
// template carried its query, fragment, percent encoding or surrounding spaces
// into the file name, so it never resolved.
func TestAssetReferenceIsAURLPath(t *testing.T) {
	png := readFixture(t, "adversarial", "a.png")
	dir := t.TempDir()
	writeTemp(t, dir, "logo.png", png)
	writeTemp(t, dir, "logo x.png", png)
	loader := &imageLoader{baseDir: dir}
	defer func() { _ = loader.Close() }()
	for _, src := range []string{"logo.png?v=2", "logo.png#top", "logo%20x.png", " logo.png "} {
		if _, err := loader.Load(src); err != nil {
			t.Fatalf("%q did not resolve: %v", src, err)
		}
	}
	// A missing file names the local path, not the polluted reference.
	_, err := loader.Load("missing.png?v=1")
	if err == nil || !strings.Contains(err.Error(), "missing.png") || strings.Contains(err.Error(), "?v=1") {
		t.Fatalf("the diagnostic = %v", err)
	}
}

// pngHeaderBytes builds a PNG header whose declared area overflows int32 but
// not int64, which is the case the 64-bit comparison must catch.
func pngHeaderBytes(w, h uint32) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	chunk := make([]byte, 4+4+13)
	binary.BigEndian.PutUint32(chunk[0:4], 13) // IHDR length
	copy(chunk[4:8], "IHDR")
	binary.BigEndian.PutUint32(chunk[8:12], w)
	binary.BigEndian.PutUint32(chunk[12:16], h)
	chunk[16] = 8                        // bit depth
	chunk[17] = 2                        // colour type: truecolour
	crc := crc32.ChecksumIEEE(chunk[4:]) // type and data
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc)
	b.Write(chunk)
	b.Write(sum[:])
	return b.Bytes()
}

// TestImageAreaIsComputedIn64Bits guards RV4-33: cfg.Width*cfg.Height was
// multiplied as int, so on a 32-bit build the product wrapped and bypassed
// MaxImagePixels. The comparison is in 64 bits now.
func TestImageAreaIsComputedIn64Bits(t *testing.T) {
	dir := t.TempDir()
	// 65535 * 65535 = 4294836225, which does not fit an int32.
	writeTemp(t, dir, "huge.png", pngHeaderBytes(65535, 65535))
	loader := &imageLoader{baseDir: dir, maxPixels: 1000}
	defer func() { _ = loader.Close() }()
	_, err := loader.Load("huge.png")
	if err == nil || !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("a 4.29-gigapixel header returned %v, want resource/limit", err)
	}
	if !strings.Contains(err.Error(), "4294836225") {
		t.Fatalf("the diagnostic does not name the area: %v", err)
	}
}

// TestFailedImageDoesNotConsumeTheBudget guards RV4-34: a failed load kept its
// bytes and pixels in the aggregate totals, so a later valid image was rejected.
func TestFailedImageDoesNotConsumeTheBudget(t *testing.T) {
	png := readFixture(t, "adversarial", "a.png")
	dir := t.TempDir()
	corrupt := bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 100) // 400 bytes of damage
	writeTemp(t, dir, "corrupt.png", corrupt)
	writeTemp(t, dir, "good.png", png)
	// Enough for the good image and one copy of the corrupt bytes, not for
	// both counted twice.
	loader := &imageLoader{
		baseDir:       dir,
		maxTotalBytes: len(corrupt) + len(png),
	}
	defer func() { _ = loader.Close() }()
	if _, err := loader.Load("corrupt.png"); err == nil {
		t.Fatal("the corrupt image loaded")
	}
	if _, err := loader.Load("good.png"); err != nil {
		t.Fatalf("the valid image after a corrupt one failed: %v", err)
	}
}

// TestTruncatedJPEGIsRejected guards RV4-35: a JPEG cut before its
// end-of-image marker was embedded as-is, while a truncated PNG failed.
func TestTruncatedJPEGIsRejected(t *testing.T) {
	full := readFixture(t, "fixtures", "hello-world.jpg")
	truncated := full[:len(full)*2/3]
	dir := t.TempDir()
	writeTemp(t, dir, "cut.jpg", truncated)
	loader := &imageLoader{baseDir: dir}
	defer func() { _ = loader.Close() }()
	if _, err := loader.Load("cut.jpg"); err == nil || !strings.Contains(err.Error(), "end-of-image") {
		t.Fatalf("a truncated JPEG returned %v", err)
	}
	writeTemp(t, dir, "ok.jpg", full)
	if _, err := loader.Load("ok.jpg"); err != nil {
		t.Fatalf("a complete JPEG was rejected: %v", err)
	}
}
