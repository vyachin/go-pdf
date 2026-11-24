package engine

import (
	"context"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
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
	for _, name := range []string{"invoice.html", "act.html", "contract.html"} {
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
		if bounds.Dx() < 100 || bounds.Dy() < 100 {
			t.Fatalf("%s: page is %dx%d", name, bounds.Dx(), bounds.Dy())
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
