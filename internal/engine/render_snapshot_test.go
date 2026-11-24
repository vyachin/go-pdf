package engine

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/backend/raster"
)

// renderDPI is fixed because the goldens are byte-for-byte pixel comparisons.
const renderDPI = 72

// bundledFont is the font every render golden is produced with. PDFTEST_FONT
// is ignored on purpose: pixels must not depend on the developer machine.
func bundledFont(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRoot(), "testdata", "fonts", "DejaVuSans.ttf")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("bundled test font is missing: %v", err)
	}
	return path
}

type renderer struct {
	bin     string
	version string
}

// newRenderer locates pdftoppm and records its version. CI sets
// PDF_REQUIRE_RENDER=1, so a missing tool fails the build there.
func newRenderer(t *testing.T) renderer {
	t.Helper()
	bin, err := exec.LookPath("pdftoppm")
	if err != nil {
		if os.Getenv("PDF_REQUIRE_RENDER") == "1" {
			t.Fatalf("pdftoppm is required (PDF_REQUIRE_RENDER=1) but was not found in PATH")
		}
		t.Skip("pdftoppm is not installed")
	}
	out, err := exec.Command(bin, "-v").CombinedOutput()
	if err != nil && len(out) == 0 {
		t.Fatalf("pdftoppm -v: %v", err)
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return renderer{bin: bin, version: version}
}

// renderPages rasterizes every page of a PDF at renderDPI and decodes it.
func (r renderer) renderPages(t *testing.T, pdf []byte) []image.Image {
	t.Helper()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "page.pdf")
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(dir, "page")
	cmd := exec.Command(r.bin, "-png", "-r", strconv.Itoa(renderDPI), pdfPath, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pdftoppm: %v\n%s", err, out)
	}
	paths, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("pdftoppm produced no pages")
	}
	sort.Slice(paths, func(i, j int) bool {
		return pageNumber(t, paths[i]) < pageNumber(t, paths[j])
	})
	pages := make([]image.Image, 0, len(paths))
	for i, path := range paths {
		if got := pageNumber(t, path); got != i+1 {
			t.Fatalf("rendered pages are not contiguous: %s is page %d, want %d", path, got, i+1)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("decoding %s: %v", path, err)
		}
		pages = append(pages, img)
	}
	return pages
}

// pageNumber extracts the page suffix pdftoppm adds to the output name.
func pageNumber(t *testing.T, path string) int {
	t.Helper()
	base := strings.TrimSuffix(filepath.Base(path), ".png")
	dash := strings.LastIndex(base, "-")
	if dash < 0 {
		t.Fatalf("unexpected render name: %s", path)
	}
	n, err := strconv.Atoi(base[dash+1:])
	if err != nil {
		t.Fatalf("unexpected render name %s: %v", path, err)
	}
	return n
}

// renderGoldenDir holds the committed page images and the recorded rasterizer.
func renderGoldenDir() string {
	return filepath.Join(repoRoot(), "testdata", "golden", "render")
}

func rendererRecordPath() string {
	return filepath.Join(renderGoldenDir(), "renderer.txt")
}

// visualFixtures are the documents the render layer freezes: the corpus and
// the fixtures whose purpose is painting.
func visualFixtures(t *testing.T) []snapshotFixture {
	t.Helper()
	root := repoRoot()
	var out []snapshotFixture
	for _, name := range corpusFixtures {
		out = append(out, snapshotFixture{
			name:    strings.TrimSuffix(name, ".html"),
			path:    filepath.Join(root, "testdata", name),
			baseDir: filepath.Join(root, "testdata"),
		})
	}
	paths, err := filepath.Glob(filepath.Join(root, "testdata", "fixtures", "visual-*.html"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		out = append(out, snapshotFixture{
			name:    strings.TrimSuffix(filepath.Base(path), ".html"),
			path:    path,
			baseDir: filepath.Join(root, "testdata", "fixtures"),
		})
	}
	return out
}

// convertFixture converts a fixture with the bundled font and returns the PDF.
func convertFixture(t *testing.T, f snapshotFixture) []byte {
	t.Helper()
	src, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), src, Options{
		FallbackFont: bundledFont(t),
		File:         filepath.Base(f.path),
		BaseDir:      f.baseDir,
		FontDir:      filepath.Join(repoRoot(), "testdata", "fonts"),
	})
	if err != nil {
		t.Fatalf("convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	return res.PDF
}

// TestRenderHelper checks the rasterization path itself before any golden
// depends on it.
func TestRenderHelper(t *testing.T) {
	r := newRenderer(t)
	f := snapshotFixture{
		name:    "invoice",
		path:    filepath.Join(repoRoot(), "testdata", "invoice.html"),
		baseDir: filepath.Join(repoRoot(), "testdata"),
	}
	pages := r.renderPages(t, convertFixture(t, f))
	if len(pages) != 1 {
		t.Fatalf("invoice renders %d pages, want 1", len(pages))
	}
	// A4 at 72 dpi is 595.276 x 841.89 pixels; poppler rounds each side up.
	b := pages[0].Bounds()
	if b.Dx() < 594 || b.Dx() > 596 || b.Dy() < 841 || b.Dy() > 843 {
		t.Fatalf("invoice page is %dx%d, want A4 at %d dpi", b.Dx(), b.Dy(), renderDPI)
	}
	if got := rgbaOf(pages[0]).Pix[0:3]; got[0] != 255 || got[1] != 255 || got[2] != 255 {
		t.Fatalf("page corner is not white: %v", got)
	}
}

// TestRenderSnapshots freezes the painted pages. Comparison is pixel exact:
// a measured one millimetre margin change moves 12.4% of the pixels, while an
// added glyph moves 34 pixels, so any tolerance would hide real regressions.
func TestRenderSnapshots(t *testing.T) {
	r := newRenderer(t)
	dir := renderGoldenDir()
	if *updateGolden {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		recordRenderer(t, r)
	} else {
		checkRendererRecord(t, r)
	}
	fixtures := visualFixtures(t)
	seen := map[string]bool{}
	for _, f := range fixtures {
		f := f
		t.Run(f.name, func(t *testing.T) {
			seen[f.name] = true
			pages := r.renderPages(t, convertFixture(t, f))
			if *updateGolden {
				removeFixtureGoldens(t, dir, f.name)
			}
			for i, page := range pages {
				path := filepath.Join(dir, fmt.Sprintf("%s-%d.png", f.name, i+1))
				if *updateGolden {
					if err := writePNG(path, rgbaOf(page)); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := readPNG(path)
				if err != nil {
					t.Fatalf("missing golden %s\nrun go test ./internal/engine -run TestRenderSnapshots -update", path)
				}
				actual := rgbaOf(page)
				diff, err := raster.Diff(actual, want, 0)
				if err != nil {
					t.Fatalf("page %d: %v", i+1, err)
				}
				if diff == 0 {
					continue
				}
				count, box, _ := pixelDiff(actual, want)
				reportRenderMismatch(t, dir, f.name, i+1, path, actual, want, count, box)
			}
		})
	}
	// A filtered run processes a subset; the rest are not stale.
	if *updateGolden || len(seen) != len(fixtures) {
		return
	}
	checkStaleGoldens(t, dir, seen)
}

// checkRendererRecord fails in strict CI mode when the rasterizer that produced
// the goldens is not the one running now. Elsewhere the layer is skipped,
// because a renderer difference would look like a layout regression.
func checkRendererRecord(t *testing.T, r renderer) {
	t.Helper()
	data, err := os.ReadFile(rendererRecordPath())
	if err != nil {
		if os.Getenv("PDF_REQUIRE_RENDER") == "1" {
			t.Fatalf("no recorded rasterizer; run go test ./internal/engine -run TestRenderSnapshots -update")
		}
		t.Skipf("render goldens have no recorded rasterizer (%v)", err)
	}
	recorded := strings.TrimSpace(string(data))
	if recorded == r.version {
		return
	}
	if os.Getenv("PDF_REQUIRE_RENDER") == "1" {
		t.Fatalf("rasterizer mismatch: goldens were produced with %q, current is %q\nregenerate with go test ./internal/engine -run TestRenderSnapshots -update", recorded, r.version)
	}
	t.Skipf("rasterizer %q differs from the recorded %q", r.version, recorded)
}

func recordRenderer(t *testing.T, r renderer) {
	t.Helper()
	if err := os.WriteFile(rendererRecordPath(), []byte(r.version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// removeFixtureGoldens deletes the pages of one fixture before an update, so a
// document that loses a page does not leave an orphan image behind.
func removeFixtureGoldens(t *testing.T, dir, name string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, name+"-*.png"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func writePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func readPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return rgbaOf(img), nil
}

// rgbaOf converts any image to the RGBA form raster.Diff works on.
func rgbaOf(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// pixelDiff counts differing pixels and returns the bounding box that contains
// them, so a failure says where to look.
func pixelDiff(a, b *image.RGBA) (int, image.Rectangle, bool) {
	count := 0
	box := image.Rectangle{}
	for y := 0; y < a.Rect.Dy(); y++ {
		for x := 0; x < a.Rect.Dx(); x++ {
			i := a.PixOffset(x, y)
			if a.Pix[i] == b.Pix[i] && a.Pix[i+1] == b.Pix[i+1] && a.Pix[i+2] == b.Pix[i+2] {
				continue
			}
			count++
			p := image.Pt(x, y)
			if count == 1 {
				box = image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))}
			} else {
				box = box.Union(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
			}
		}
	}
	return count, box, count > 0
}

// diffImage paints differing pixels red over a faded copy of the produced page.
func diffImage(a, b *image.RGBA) *image.RGBA {
	out := image.NewRGBA(a.Rect)
	copy(out.Pix, a.Pix)
	gray := color.RGBA{R: 210, G: 210, B: 210, A: 255}
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	for y := 0; y < a.Rect.Dy(); y++ {
		for x := 0; x < a.Rect.Dx(); x++ {
			i := out.PixOffset(x, y)
			j := b.PixOffset(x, y)
			if a.Pix[j] == b.Pix[j] && a.Pix[j+1] == b.Pix[j+1] && a.Pix[j+2] == b.Pix[j+2] {
				out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = gray.R, gray.G, gray.B, gray.A
				continue
			}
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = red.R, red.G, red.B, red.A
		}
	}
	return out
}

func reportRenderMismatch(t *testing.T, dir, name string, page int, golden string, actual, want *image.RGBA, count int, box image.Rectangle) {
	t.Helper()
	artifacts := artifactDir(t)
	base := filepath.Join(artifacts, fmt.Sprintf("%s-%d", name, page))
	paths := map[string]string{
		"produced": base + ".actual.png",
		"expected": base + ".expected.png",
		"diff":     base + ".diff.png",
	}
	if err := writePNG(paths["produced"], actual); err != nil {
		t.Fatal(err)
	}
	if err := writePNG(paths["expected"], want); err != nil {
		t.Fatal(err)
	}
	if err := writePNG(paths["diff"], diffImage(actual, want)); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("page %d differs from %s\n%d pixels differ inside %v\nproduced: %s\nexpected: %s\ndiff: %s",
		page, golden, count, box, paths["produced"], paths["expected"], paths["diff"])
}

func checkStaleGoldens(t *testing.T, dir string, seen map[string]bool) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*-*.png"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		base := strings.TrimSuffix(filepath.Base(path), ".png")
		dash := strings.LastIndex(base, "-")
		if dash < 0 {
			continue
		}
		name := base[:dash]
		if !seen[name] {
			t.Errorf("stale render golden without a fixture: %s", path)
		}
	}
}
