package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeNoisyPNG writes a PNG whose pixels do not compress, so its encoded size
// is large enough to measure whether one file produced one embedded object.
func writeNoisyPNG(t *testing.T, path string, w, h int) int64 {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(1)
	next := func() byte {
		seed = seed*1664525 + 1013904223
		return byte(seed >> 24)
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = next(), next(), next(), 255
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// countImageObjects counts the image XObjects in a produced PDF.
func countImageObjects(pdf []byte) int {
	return strings.Count(string(pdf), "/Subtype /Image")
}

// imageSpellings returns 50 distinct spellings of the file name a.png.
func imageSpellings() []string {
	out := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		var b strings.Builder
		for j := 0; j < i%5; j++ {
			b.WriteString("./")
		}
		for j := 0; j < i/5; j++ {
			b.WriteString("sub/../")
		}
		b.WriteString("a.png")
		out = append(out, b.String())
	}
	return out
}

// TestManySpellingsEmbedOneImage guards the asset amplification: 50 spellings of
// one file used to be read, decoded and embedded 50 times, inflating a small
// document into a huge PDF.
func TestManySpellingsEmbedOneImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	size := writeNoisyPNG(t, filepath.Join(dir, "a.png"), 256, 256)
	var b strings.Builder
	for _, spelling := range imageSpellings() {
		fmt.Fprintf(&b, `<img src="%s">`, spelling)
	}
	res, err := Convert(context.Background(), []byte(b.String()), Options{
		BaseDir: dir,
		File:    "spellings.html",
	})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if n := countImageObjects(res.PDF); n != 1 {
		t.Fatalf("the PDF embeds %d image objects, want 1", n)
	}
	if limit := 2*size + 100_000; int64(len(res.PDF)) > limit {
		t.Fatalf("the PDF is %d bytes for a %d-byte asset, want at most %d", len(res.PDF), size, limit)
	}
}

// TestRepeatedDataURIEmbedsOneImage guards the data: path: the same URI used by
// an <img> and a background is decoded once and embedded once.
func TestRepeatedDataURIEmbedsOneImage(t *testing.T) {
	// Opaque pixels: a translucent source legitimately adds a soft-mask image
	// object, which would make the object count ambiguous.
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	src := `<style>div { height: 10pt; background-image: url("` + uri + `") }</style>
	<img src="` + uri + `"><div></div>`
	res, err := Convert(context.Background(), []byte(src), Options{File: "data.html"})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if n := countImageObjects(res.PDF); n != 1 {
		t.Fatalf("the PDF embeds %d image objects, want 1", n)
	}
}

// TestAggregateImageLimits guards the totals: a document whose images cross the
// aggregate bytes or pixels is rejected with resource/limit.
func TestAggregateImageLimits(t *testing.T) {
	dir := t.TempDir()
	size := writeNoisyPNG(t, filepath.Join(dir, "a.png"), 256, 256)
	for _, tc := range []struct {
		name   string
		limits Limits
	}{
		{"bytes", Limits{MaxTotalImageBytes: int(size) - 1}},
		{"pixels", Limits{MaxTotalImagePixels: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(`<img src="a.png">`), Options{
				BaseDir: dir,
				File:    "total-image.html",
				Limits:  tc.limits,
			})
			if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
				t.Fatalf("the total was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
			}
		})
	}
}

// TestAggregateFontBytesLimit guards the font total: every @font-face source is
// counted, and the font past the total is rejected before it is read.
func TestAggregateFontBytesLimit(t *testing.T) {
	dir := t.TempDir()
	size := copyTestFont(t, dir, "a.ttf")
	copyTestFont(t, dir, "b.ttf")
	src := `<style>
	@font-face { font-family: A; src: url(a.ttf) }
	@font-face { font-family: B; src: url(b.ttf) }
	p { font-family: A }
	</style><p>Текст</p>`
	res, err := Convert(context.Background(), []byte(src), Options{
		BaseDir: dir,
		File:    "total-font.html",
		Limits:  Limits{MaxTotalFontBytes: int(size) + int(size)/2},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("the font total was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestFontSpellingsParseOnce guards canonical identity on the font side: two
// spellings of one file are parsed and counted once, so a total that fits one
// copy is enough.
func TestFontSpellingsParseOnce(t *testing.T) {
	dir := t.TempDir()
	size := copyTestFont(t, dir, "a.ttf")
	src := `<style>
	@font-face { font-family: A; src: url(a.ttf) }
	@font-face { font-family: B; src: url(./a.ttf) }
	p { font-family: A }
	</style><p>Текст</p>`
	res, err := Convert(context.Background(), []byte(src), Options{
		BaseDir: dir,
		File:    "font-spellings.html",
		Limits:  Limits{MaxTotalFontBytes: int(size) + int(size)/2},
	})
	if err != nil {
		t.Fatalf("one font under two spellings exhausted the total: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("no PDF produced")
	}
}

// copyTestFont copies the bundled test font into dir and reports its size.
func copyTestFont(t *testing.T, dir, name string) int64 {
	t.Helper()
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return int64(len(data))
}

// TestAggregateLimitDefaults guards the documented defaults and that a caller's
// value wins over them.
func TestAggregateLimitDefaults(t *testing.T) {
	l := Limits{}.resolved()
	if l.MaxTotalImageBytes != 64<<20 || l.MaxTotalImagePixels != 80_000_000 || l.MaxTotalFontBytes != 32<<20 {
		t.Fatalf("defaults = %d/%d/%d", l.MaxTotalImageBytes, l.MaxTotalImagePixels, l.MaxTotalFontBytes)
	}
	l = Limits{MaxTotalImageBytes: 1, MaxTotalImagePixels: 2, MaxTotalFontBytes: 3}.resolved()
	if l.MaxTotalImageBytes != 1 || l.MaxTotalImagePixels != 2 || l.MaxTotalFontBytes != 3 {
		t.Fatalf("caller values were overwritten: %d/%d/%d", l.MaxTotalImageBytes, l.MaxTotalImagePixels, l.MaxTotalFontBytes)
	}
}
