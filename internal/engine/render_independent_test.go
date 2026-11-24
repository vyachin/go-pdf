package engine

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// independentTools locates pdftoppm and pdftotext. CI installs poppler-utils
// and sets PDF_REQUIRE_RENDER=1; locally the check is skipped when the tools
// are missing.
func independentTools(t *testing.T) (string, string) {
	t.Helper()
	ppm, errPPM := exec.LookPath("pdftoppm")
	txt, errTXT := exec.LookPath("pdftotext")
	if errPPM != nil || errTXT != nil {
		if os.Getenv("PDF_REQUIRE_RENDER") == "1" {
			t.Fatalf("poppler tools are required (PDF_REQUIRE_RENDER=1): pdftoppm=%v pdftotext=%v", errPPM, errTXT)
		}
		t.Skip("poppler tools are not installed")
	}
	return ppm, txt
}

func convertTestdata(t *testing.T, name string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), src, Options{FallbackFont: testFontPath(t), File: name})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res.PDF
}

func TestIndependentRendererSeesContent(t *testing.T) {
	ppm, txt := independentTools(t)
	dir := t.TempDir()
	corpus := []struct {
		name  string
		marks []string
	}{
		{"invoice.html", []string{"Счёт", "Итого", "534"}},
		{"act.html", []string{"Акт", "Проектирование"}},
		{"contract.html", []string{"Договор", "Ответственность"}},
	}
	for _, item := range corpus {
		name := item.name
		pdf := convertTestdata(t, name)
		base := filepath.Join(dir, strings.TrimSuffix(name, ".html"))
		if err := os.WriteFile(base+".pdf", pdf, 0o644); err != nil {
			t.Fatal(err)
		}
		// The independent text extractor must find the document text.
		out, err := exec.Command(txt, "-q", base+".pdf", "-").Output()
		if err != nil {
			t.Fatalf("%s: pdftotext: %v", name, err)
		}
		if len(strings.TrimSpace(string(out))) < 20 {
			t.Fatalf("%s: independent extraction returned almost nothing: %q", name, out)
		}
		for _, mark := range item.marks {
			if !strings.Contains(string(out), mark) {
				t.Fatalf("%s: independent extraction does not contain %q", name, mark)
			}
		}
		// The independent renderer must produce a page with ink on it.
		cmd := exec.Command(ppm, "-png", "-r", "72", "-f", "1", "-singlefile", base+".pdf", base)
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: pdftoppm: %v", name, err)
		}
		f, err := os.Open(base + ".png")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: decoded render: %v", name, err)
		}
		bounds := img.Bounds()
		// A4 at 72 dpi is 595x842 pixels; allow one pixel for rounding.
		if bounds.Dx() < 594 || bounds.Dx() > 596 || bounds.Dy() < 841 || bounds.Dy() > 843 {
			t.Fatalf("%s: page is %dx%d, want A4 at 72 dpi", name, bounds.Dx(), bounds.Dy())
		}
		dark, total := 0, bounds.Dx()*bounds.Dy()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 0x8000 || g < 0x8000 || b < 0x8000 {
					dark++
				}
			}
		}
		if dark == 0 {
			t.Fatalf("%s: the rendered page is blank", name)
		}
		if dark*100/total > 60 {
			t.Fatalf("%s: %d%% of the page is covered; the render looks wrong", name, dark*100/total)
		}
	}
}

// renderFirstPage rasterizes the first page of a PDF with poppler and returns
// the decoded image.
func renderFirstPage(t *testing.T, ppm, pdfPath string) image.Image {
	t.Helper()
	base := strings.TrimSuffix(pdfPath, ".pdf") + "-render"
	if err := exec.Command(ppm, "-png", "-r", "72", "-f", "1", "-singlefile", pdfPath, base).Run(); err != nil {
		t.Fatalf("pdftoppm: %v", err)
	}
	f, err := os.Open(base + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decoding the render: %v", err)
	}
	return img
}

// rgbAt returns the 8-bit channels at a pixel.
func rgbAt(img image.Image, x, y int) (int, int, int) {
	r, g, b, _ := img.At(x, y).RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func TestIndependentRendererColoursAndCoordinates(t *testing.T) {
	ppm, txt := independentTools(t)
	dir := t.TempDir()
	src := `<style>
		@page { size: 200pt 200pt; margin: 0 }
		body { margin: 0; font-size: 12pt; font-family: sans-serif }
		.red { width: 100pt; height: 50pt; background-color: #ff0000 }
	</style>
	<div class="red"></div>
	<p style="margin:0">ALPHA-TEXT</p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: testFontPath(t), File: "coordinates.html"})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	pdfPath := filepath.Join(dir, "coordinates.pdf")
	if err := os.WriteFile(pdfPath, res.PDF, 0o644); err != nil {
		t.Fatal(err)
	}

	// The independent text extractor must return the exact word and place it
	// just below the 50 pt red block, starting at the left page edge.
	bbox, err := exec.Command(txt, "-q", "-bbox", pdfPath, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext -bbox: %v", err)
	}
	word := regexp.MustCompile(`<word xMin="([0-9.]+)" yMin="([0-9.]+)"[^>]*>ALPHA-TEXT</word>`)
	m := word.FindStringSubmatch(string(bbox))
	if m == nil {
		t.Fatalf("ALPHA-TEXT is missing from the independent extraction:\n%s", bbox)
	}
	xMin, _ := strconv.ParseFloat(m[1], 64)
	yMin, _ := strconv.ParseFloat(m[2], 64)
	if xMin < -1 || xMin > 2 {
		t.Fatalf("text xMin = %.1f pt, want 0", xMin)
	}
	if yMin < 45 || yMin > 65 {
		t.Fatalf("text yMin = %.1f pt, want about the 50 pt block bottom", yMin)
	}

	img := renderFirstPage(t, ppm, pdfPath)
	b := img.Bounds()
	if b.Dx() != 200 || b.Dy() != 200 {
		t.Fatalf("render is %dx%d, want 200x200 at 72 dpi", b.Dx(), b.Dy())
	}
	// The red block covers the top-left quarter of the page.
	if r, g, bl := rgbAt(img, 50, 25); r < 230 || g > 30 || bl > 30 {
		t.Fatalf("block interior = %d,%d,%d, want red", r, g, bl)
	}
	// The rest of the page stays white.
	if r, g, bl := rgbAt(img, 160, 160); r < 240 || g < 240 || bl < 240 {
		t.Fatalf("page background = %d,%d,%d, want white", r, g, bl)
	}
	// There is ink below the block where the text sits, and none above it.
	inkIn := func(x0, y0, x1, y1 int) bool {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				r, g, bl := rgbAt(img, x, y)
				if r < 100 && g < 100 && bl < 100 {
					return true
				}
			}
		}
		return false
	}
	if !inkIn(0, 52, 100, 68) {
		t.Fatal("no text ink below the block")
	}
	if inkIn(110, 55, 200, 70) {
		t.Fatal("unexpected ink to the right of the text")
	}
}

func TestIndependentRendererCompositesTransparency(t *testing.T) {
	ppm, txt := independentTools(t)
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	f, err := os.Create(filepath.Join(dir, "half.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	src := `<style>
		@page { size: 200pt 200pt; margin: 0 }
		body { margin: 0 }
	</style>
	<div style="width:100pt;height:100pt;background-color:#ff0000">
		<img src="half.png" style="width:50pt;height:50pt">
	</div>
	<p style="margin:0">ALPHA</p>`
	res, err := Convert(context.Background(), []byte(src), Options{
		FallbackFont: testFontPath(t),
		BaseDir:      dir,
		File:         "alpha-render.html",
	})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	pdfPath := filepath.Join(dir, "alpha-render.pdf")
	if err := os.WriteFile(pdfPath, res.PDF, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(txt, "-q", pdfPath, "-").Output(); err != nil || !strings.Contains(string(out), "ALPHA") {
		t.Fatalf("independent extraction: %q, %v", out, err)
	}
	render := renderFirstPage(t, ppm, pdfPath)
	// The half-transparent red image must blend with the red block: a white
	// flattening would leave a pink rectangle here.
	if r, g, b := rgbAt(render, 25, 25); r < 230 || g > 60 || b > 60 {
		t.Fatalf("translucent image over red = %d,%d,%d; it was flattened instead of composited", r, g, b)
	}
	// Outside the image the block is still red.
	if r, g, b := rgbAt(render, 75, 75); r < 230 || g > 30 || b > 30 {
		t.Fatalf("red block outside the image = %d,%d,%d", r, g, b)
	}
}
