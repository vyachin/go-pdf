package engine

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/layout"
)

// noisePNG returns a PNG of pseudo-random pixels: the noise compresses badly, so
// the encoded bytes are large enough for the hashing bounds to matter.
func noisePNG(t *testing.T, side int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	r := rand.New(rand.NewSource(1))
	for i := 0; i < len(img.Pix); i += 4 {
		// Opaque: a random alpha would add an SMask object and make the image
		// count ambiguous.
		img.Pix[i] = byte(r.Intn(256))
		img.Pix[i+1] = byte(r.Intn(256))
		img.Pix[i+2] = byte(r.Intn(256))
		img.Pix[i+3] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestImageDigestIsHashedOnce guards the digest cache: the second placement of
// one loaded image reuses the digest instead of scanning the bytes again.
func TestImageDigestIsHashedOnce(t *testing.T) {
	data := noisePNG(t, 200)
	img := &layout.ImageData{Data: data}
	h := &imageHasher{ctx: context.Background(), guard: budget.New(context.Background(), 0),
		digests: map[*layout.ImageData][32]byte{}}
	first, err := h.digest(img)
	if err != nil {
		t.Fatal(err)
	}
	if h.hashed != int64(len(data)) {
		t.Fatalf("the first digest hashed %d bytes, want %d", h.hashed, len(data))
	}
	second, err := h.digest(img)
	if err != nil {
		t.Fatal(err)
	}
	if h.hashed != int64(len(data)) {
		t.Fatalf("the second digest hashed bytes again: total %d, want %d", h.hashed, len(data))
	}
	if first != second {
		t.Fatal("the second placement produced another digest")
	}
}

// TestImageDigestBudgetStopsBeforeTheBytes guards the charge: a digest that
// cannot fit the remaining budget fails before the bytes are hashed.
func TestImageDigestBudgetStopsBeforeTheBytes(t *testing.T) {
	data := noisePNG(t, 200)
	img := &layout.ImageData{Data: data}
	// One chunk costs about 256 operations, so this budget covers one chunk of
	// the maybe-four an image this size needs.
	h := &imageHasher{ctx: context.Background(), guard: budget.New(context.Background(), 10),
		digests: map[*layout.ImageData][32]byte{}}
	_, err := h.digest(img)
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("digest returned %v, want budget.ErrExceeded", err)
	}
	if h.hashed >= int64(len(data)) {
		t.Fatalf("the digest hashed %d of %d bytes before failing", h.hashed, len(data))
	}
}

// TestImageDigestHonoursCancellation guards cancellation between chunks.
func TestImageDigestHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	img := &layout.ImageData{Data: noisePNG(t, 64)}
	h := &imageHasher{ctx: ctx, guard: budget.New(context.Background(), 0),
		digests: map[*layout.ImageData][32]byte{}}
	_, err := h.digest(img)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("digest returned %v, want context.Canceled", err)
	}
	if h.hashed != 0 {
		t.Fatalf("the digest hashed %d bytes after cancellation", h.hashed)
	}
}

// TestManyPlacementsEmbedOneImage guards the placement contract: a large image
// placed a thousand times across pages is hashed once, embedded once, and the
// output is stable.
func TestManyPlacementsEmbedOneImage(t *testing.T) {
	dir := t.TempDir()
	data := noisePNG(t, 200)
	if err := os.WriteFile(filepath.Join(dir, "large.png"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	var src strings.Builder
	for i := 0; i < 1000; i++ {
		src.WriteString(`<p>item `)
		src.WriteString(strings.Repeat("x", 1))
		src.WriteString(` <img src="large.png"></p>`)
	}
	opts := Options{BaseDir: dir, File: "many.html", FallbackFont: testFontPath(t)}
	first, err := Convert(context.Background(), []byte(src.String()), opts)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(first.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("the repeated image produced %d image objects, want 1", n)
	}
	second, err := Convert(context.Background(), []byte(src.String()), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.PDF, second.PDF) {
		t.Fatal("two conversions of the same input differ")
	}
}

// TestIdenticalContentInTwoFilesSharesOneImage guards content deduplication
// across different paths: the digest is content-based, not path-based.
func TestIdenticalContentInTwoFilesSharesOneImage(t *testing.T) {
	dir := t.TempDir()
	data := noisePNG(t, 40)
	for _, name := range []string{"a.png", "b.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Convert(context.Background(), []byte(`<img src="a.png"><img src="b.png">`), Options{
		BaseDir: dir, File: "same.html", FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(res.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("identical content in two files produced %d image objects, want 1", n)
	}
}
