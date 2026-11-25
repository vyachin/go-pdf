package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// TestSubsetTagAndDescriptor guards the embedded subset name and the descriptor
// values: a six-letter tag on /BaseFont and /FontName, and CapHeight from the
// font rather than the ascent.
func TestSubsetTagAndDescriptor(t *testing.T) {
	res, err := Convert(context.Background(), []byte(`<p>Текст</p>`), Options{
		File:         "subset.html",
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	tag := regexp.MustCompile(`/BaseFont /([A-Z]{6})\+`)
	m := tag.FindSubmatch(res.PDF)
	if m == nil {
		t.Fatal("the embedded font has no six-letter subset tag")
	}
	if !bytes.Contains(res.PDF, []byte("/FontName /"+string(m[1])+"+")) {
		t.Fatalf("/FontName does not carry the tag %q", m[1])
	}
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	wantCap := int32(float64(f.CapHeight()) * 1000 / float64(f.UnitsPerEm()))
	if !bytes.Contains(res.PDF, []byte(fmt.Sprintf("/CapHeight %d", wantCap))) {
		t.Fatalf("the descriptor does not carry CapHeight %d", wantCap)
	}
	if f.ItalicAngle() == 0 && !bytes.Contains(res.PDF, []byte("/ItalicAngle 0")) {
		t.Fatal("the descriptor does not carry the font's italic angle")
	}
	validateWithPDFCPU(t, res.PDF)
}

// TestItalicAngleFromPost guards a non-zero italic angle: the writer must read
// the post table instead of emitting 0.
func TestItalicAngleFromPost(t *testing.T) {
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	out := append([]byte(nil), data...)
	n := int(binary.BigEndian.Uint16(out[4:6]))
	found := false
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if string(out[rec:rec+4]) != "post" {
			continue
		}
		off := int(binary.BigEndian.Uint32(out[rec+8 : rec+12]))
		if off+8 > len(out) {
			t.Fatal("the post table is too short")
		}
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(12<<16))
		found = true
		break
	}
	if !found {
		t.Fatal("the test font has no post table")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "italic.ttf")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), []byte(`<p>Текст</p>`), Options{
		File:         "italic.html",
		FallbackFont: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(res.PDF, []byte("/ItalicAngle 12")) {
		t.Fatal("the descriptor does not carry the post table's angle")
	}
}

// TestMetadataUTF8 guards metadata: an invalid value is rejected with its own
// diagnostic and a valid one round-trips.
func TestMetadataUTF8(t *testing.T) {
	res, err := Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "meta.html",
		FallbackFont: testFontPath(t),
		Title:        string([]byte{0xff}),
	})
	if err == nil || !hasCode(res.Diagnostics, "options/title") || len(res.PDF) != 0 {
		t.Fatalf("invalid metadata was not rejected: err=%v diags=%v", err, res.Diagnostics.Lines())
	}

	title := "Счёт (№ 42) \\ 😀"
	res, err = Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "meta-ok.html",
		FallbackFont: testFontPath(t),
		Title:        title,
	})
	if err != nil {
		t.Fatalf("valid metadata was rejected: %v", err)
	}
	if bin, lookErr := exec.LookPath("pdfinfo"); lookErr == nil {
		path := filepath.Join(t.TempDir(), "meta.pdf")
		if err := os.WriteFile(path, res.PDF, 0o644); err != nil {
			t.Fatal(err)
		}
		out, runErr := exec.Command(bin, path).CombinedOutput()
		if runErr != nil {
			t.Fatalf("pdfinfo failed: %v\n%s", runErr, out)
		}
		if !strings.Contains(string(out), title) {
			t.Fatalf("pdfinfo did not round-trip the title:\n%s", out)
		}
	}
}

// TestNonBMPExtraction keeps a non-BMP character whole through subsetting.
func TestNonBMPExtraction(t *testing.T) {
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	const emoji = '\U0001F600'
	if _, ok := f.GlyphIndex(emoji); !ok {
		t.Skip("the bundled font has no non-BMP glyph")
	}
	res, err := Convert(context.Background(), []byte(`<p>`+string(emoji)+`</p>`), Options{
		File:         "emoji.html",
		FallbackFont: testFontPath(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, string(emoji)) {
		t.Fatalf("text = %q, want %q", text, string(emoji))
	}
}

// TestPDFFontsReportsSubset guards the subset flag an independent tool reads.
func TestPDFFontsReportsSubset(t *testing.T) {
	bin, err := exec.LookPath("pdffonts")
	if err != nil {
		t.Skip("pdffonts is not installed")
	}
	res, convErr := Convert(context.Background(), []byte(`<p>Текст</p>`), Options{
		File:         "fonts.html",
		FallbackFont: testFontPath(t),
	})
	if convErr != nil {
		t.Fatal(convErr)
	}
	path := filepath.Join(t.TempDir(), "fonts.pdf")
	if err := os.WriteFile(path, res.PDF, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, path).CombinedOutput()
	if err != nil {
		t.Fatalf("pdffonts failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), " yes yes") {
		t.Fatalf("pdffonts does not report the font as embedded and subset:\n%s", out)
	}
}

// TestRenderPNGClip guards the debug raster clip: a cover background wider than
// its box must not paint outside the box, and RenderPNG must not change the PDF.
func TestRenderPNGClip(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 100, 10))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	img.Set(0, 0, color.RGBA{B: 255, A: 255})
	// Make every pixel blue so the outside test is unambiguous.
	for y := 0; y < 10; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, "cover.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	src := []byte(`<div style="width:50pt; height:50pt; background-image:url(cover.png); background-size:cover"></div>`)
	opts := Options{BaseDir: dir, File: "clip.html", RenderPNG: true}
	withPNG, err := Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.RenderPNG = false
	withoutPNG, err := Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withPNG.PDF, withoutPNG.PDF) {
		t.Fatal("RenderPNG changed the PDF")
	}
	pngImg, err := png.Decode(bytes.NewReader(withPNG.PNG))
	if err != nil {
		t.Fatal(err)
	}
	// 96 dpi: the content area starts at the 20 mm page margin (about 76 px)
	// and the box is 66 px wide. A pixel inside it is blue; one to its right is
	// still the page background.
	if r, g, b, _ := pngImg.At(200, 90).RGBA(); r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatalf("outside the box = %#x,%#x,%#x, want white", r, g, b)
	}
	if r, g, b, _ := pngImg.At(90, 90).RGBA(); r != 0 || g != 0 || b != 0xffff {
		t.Fatalf("inside the box = %#x,%#x,%#x, want blue", r, g, b)
	}
}
