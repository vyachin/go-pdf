package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
	"github.com/vyachin/go-pdf/internal/text"
)

func testFontPath(t testing.TB) string {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
	// The bundled font keeps results identical on every platform and in CI.
	candidates = append(candidates, filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		)
	}
	candidates = append(candidates,
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
	)
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no TrueType font available; set PDFTEST_FONT")
	return ""
}

func TestConvertProducesValidPDF(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<html><head><title>Счёт № 42</title>
	<style>
	  @page { size: A4; margin: 20mm }
	  body { font-family: "PT Serif"; font-size: 12pt }
	  h1 { font-size: 18pt }
	</style></head>
	<body><h1>Счёт № 42</h1><p>Итого к оплате: 1 200,00 руб.</p></body></html>`
	res, err := Convert(context.Background(), []byte(src), Options{
		FallbackFont: fontPath,
		File:         "invoice.html",
	})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.HasPrefix(res.PDF, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF: %q", res.PDF[:min(16, len(res.PDF))])
	}
	text, err := extractText(res.PDF)
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	for _, want := range []string{"Счёт", "42", "Итого"} {
		if !strings.Contains(text, want) {
			t.Fatalf("extracted text %q does not contain %q", text, want)
		}
	}
	validateWithPDFCPU(t, res.PDF)
}

func TestConvertDeterministic(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<html><head><title>T</title></head><body><p>Проверка детерминизма</p></body></html>`
	opts := Options{FallbackFont: fontPath, File: "d.html"}
	a, err := Convert(context.Background(), []byte(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Convert(context.Background(), []byte(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.PDF, b.PDF) {
		t.Fatal("two runs produced different bytes")
	}
}

func TestConvertReportsValidationErrors(t *testing.T) {
	src := `<style>p { display: flex }</style><p>x</p>`
	res, err := Convert(context.Background(), []byte(src), Options{File: "bad.html"})
	if err == nil {
		t.Fatal("expected an error for an unsupported value")
	}
	if len(res.Diagnostics) == 0 {
		t.Fatal("expected diagnostics")
	}
	if !strings.Contains(res.Diagnostics[0].String(), "css/unsupported-value") {
		t.Fatalf("unexpected diagnostic: %s", res.Diagnostics[0].String())
	}
}

// pdfcpuPath finds the independent validator. CI sets PDF_REQUIRE_VALIDATOR=1
// and installs pdfcpu, so a missing binary fails the build there; locally the
// check is reported as skipped instead.
func pdfcpuPath(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("PDFCPU"); bin != "" {
		return bin
	}
	if bin, err := exec.LookPath("pdfcpu"); err == nil {
		return bin
	}
	// Strict mode means "discoverable through PATH or PDFCPU"; the local
	// fallback below is a convenience that must not mask a missing tool in CI.
	if os.Getenv("PDF_REQUIRE_VALIDATOR") == "1" {
		t.Fatal("pdfcpu is required (PDF_REQUIRE_VALIDATOR=1) but was not found in PATH")
	}
	if st, err := os.Stat("/tmp/gobin/pdfcpu"); err == nil && !st.IsDir() {
		return "/tmp/gobin/pdfcpu"
	}
	t.Log("pdfcpu not available, skipping independent validation")
	return ""
}

func validateWithPDFCPU(t *testing.T, data []byte) {
	t.Helper()
	bin := pdfcpuPath(t)
	if bin == "" {
		return
	}
	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "validate", path)
	// pdfcpu writes a configuration file into the user config directory; point
	// HOME at the temporary directory so the test stays self contained.
	cmd.Env = append(os.Environ(), "HOME="+filepath.Dir(path))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfcpu validate failed: %v\n%s", err, out)
	}
}

// extractText decodes the ToUnicode maps and the text shown in content streams
// of a PDF produced by this package.
func TestRenderPNG(t *testing.T) {
	fontPath := testFontPath(t)
	res, err := Convert(context.Background(), []byte(`<p>Проверка PNG</p>`), Options{FallbackFont: fontPath, File: "png.html", RenderPNG: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.PNG) == 0 {
		t.Fatal("no PNG was produced")
	}
	img, err := png.Decode(bytes.NewReader(res.PNG))
	if err != nil {
		t.Fatalf("PNG does not decode: %v", err)
	}
	if img.Bounds().Dx() < 700 || img.Bounds().Dy() < 1000 {
		t.Fatalf("unexpected PNG size %v", img.Bounds())
	}
}

func TestImageEmbedding(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	pngPath := filepath.Join(dir, "logo.png")
	f, err := os.Create(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	jpgPath := filepath.Join(dir, "photo.jpg")
	jf, err := os.Create(jpgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(jf, img, nil); err != nil {
		t.Fatal(err)
	}
	jf.Close()

	src := `<p><img src="logo.png" width="20mm"></p><p><img src="photo.jpg" width="20mm"></p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, BaseDir: dir, File: "img.html"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.PDF)
	if !strings.Contains(out, "/Subtype /Image") {
		t.Fatal("no image object in the PDF")
	}
	if !strings.Contains(out, "/DCTDecode") {
		t.Fatal("jpeg was not embedded with DCTDecode")
	}
	validateWithPDFCPU(t, res.PDF)
}

func TestMetadata(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<html><head><title>Счёт № 42</title><meta name="author" content="ООО Ромашка"></head><body><p>x</p></body></html>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "m.html"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.PDF)
	if !strings.Contains(out, "/Title <FEFF") {
		t.Fatal("the Cyrillic title is not stored as UTF-16")
	}
	if !strings.Contains(out, "/Author <FEFF") {
		t.Fatal("the author is not stored")
	}
}

func TestFontFaceIsUsed(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<style>
		@font-face { font-family: "Company"; src: url("` + fontPath + `") }
		body { font-family: "Company" }
	</style><p>Проверка</p>`
	res, err := Convert(context.Background(), []byte(src), Options{File: "f.html"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	declared, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := "/BaseFont /" + strings.ReplaceAll(declared.Family(), " ", "-")
	if !bytes.Contains(res.PDF, []byte(want)) {
		t.Fatalf("the declared font %q is not embedded: %q", want, res.PDF[:min(200, len(res.PDF))])
	}
}

func TestContentStreamOperators(t *testing.T) {
	fontPath := testFontPath(t)
	res, err := Convert(context.Background(), []byte(`<div style="background-color:#eee">текст</div>`), Options{FallbackFont: fontPath, File: "ops.html"})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, stream := range pdfdebug.ContentStreams(res.PDF) {
		s := string(stream)
		showsText := strings.Contains(s, "Tj") || strings.Contains(s, "TJ")
		if strings.Contains(s, "BT") && strings.Contains(s, "Tf") && showsText && strings.Contains(s, "re") {
			found = true
		}
	}
	if !found {
		t.Fatal("no content stream with text and rectangle operators")
	}
}

func TestOutlineIsWritten(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<h1>Договор</h1><h2>Предмет</h2><p>текст</p><h2>Оплата</h2>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "outline.html"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.PDF)
	for _, want := range []string{"/Outlines", "/PageMode /UseOutlines", "/First", "/Dest"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the PDF does not contain %q", want)
		}
	}
	validateWithPDFCPU(t, res.PDF)
}

func TestTableOfContentsNumbers(t *testing.T) {
	fontPath := testFontPath(t)
	var body strings.Builder
	body.WriteString(`<style>
		@page { size: 90mm 60mm; margin: 6mm }
		p { margin: 0 0 2mm 0; font-size: 10pt }
		h2 { font-size: 12pt; margin: 2mm 0 }
		a.toc::after { content: " →" target-counter(attr(href), page) }
	</style>`)
	body.WriteString(`<div>`)
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&body, `<p><a class="toc" href="#s%d">Раздел %d</a></p>`, i, i)
	}
	body.WriteString(`</div>`)
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&body, `<h2 id="s%d">Статья %d</h2>`, i, i)
		for j := 0; j < 6; j++ {
			body.WriteString(`<p>Текст раздела для заполнения страницы и проверки оглавления.</p>`)
		}
	}
	res, err := Convert(context.Background(), []byte(body.String()), Options{FallbackFont: fontPath, File: "toc.html"})
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	pages, err := extractPageTexts(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 2 {
		t.Fatalf("got %d pages, want the sections to land on later pages", len(pages))
	}
	for i := 1; i <= 3; i++ {
		heading := fmt.Sprintf("Статья %d", i)
		actual := 0
		for p, text := range pages {
			if strings.Contains(text, heading) {
				actual = p + 1
				break
			}
		}
		if actual == 0 {
			t.Fatalf("heading %q was not found on any page", heading)
		}
		want := fmt.Sprintf("Раздел %d →%d", i, actual)
		if !strings.Contains(pages[0], want) {
			t.Fatalf("table of contents does not contain %q; page 1 text: %q", want, pages[0])
		}
	}
	if !strings.Contains(string(res.PDF), "/Outlines") {
		t.Fatal("the document has no bookmarks")
	}
	validateWithPDFCPU(t, res.PDF)
}

func TestUnknownTargetWarns(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<style>a.toc::after { content: target-counter(attr(href), page) }</style><p><a class="toc" href="#missing">Ссылка</a></p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "u.html"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "pagination/unknown-target" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an unknown target warning, got %v", res.Diagnostics.Lines())
	}
}

// extractText and extractPageTexts delegate to the shared reader so that the
// engine tests and the mPDF compatibility corpus use one implementation.
func extractText(data []byte) (string, error) { return pdfdebug.Text(data) }

func extractPageTexts(data []byte) ([]string, error) { return pdfdebug.Pages(data) }

func TestLandscapePage(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<style>
		@page { size: A4 landscape; margin: 15mm; @bottom-center { content: "Стр. " counter(page) } }
		body { font-size: 12pt }
	</style><p>Альбомная страница</p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "landscape.html"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.PDF)
	if !strings.Contains(out, "/MediaBox [0 0 841.89 595.276]") {
		t.Fatalf("the page is not A4 landscape: %s", firstMediaBox(out))
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Стр. 1") {
		t.Fatalf("the footer is missing: %q", text)
	}
	validateWithPDFCPU(t, res.PDF)
}

func firstMediaBox(pdf string) string {
	i := strings.Index(pdf, "/MediaBox")
	if i < 0 {
		return "(no MediaBox)"
	}
	end := i + 60
	if end > len(pdf) {
		end = len(pdf)
	}
	return pdf[i:end]
}

func TestExternalStylesheetApplied(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte(".box { background-color: #ff0000 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `<link rel="stylesheet" href="style.css"><div class="box">внешний стиль</div>`
	res, err := Convert(context.Background(), []byte(src), Options{
		FallbackFont: fontPath,
		BaseDir:      dir,
		File:         "external.html",
	})
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	var content strings.Builder
	for _, stream := range pdfdebug.ContentStreams(res.PDF) {
		content.Write(stream)
	}
	painted := content.String()
	if !strings.Contains(painted, "1 0 0 rg") {
		t.Fatalf("the background colour from the external stylesheet was not applied: %q", painted)
	}
	// The background must be painted before the text so that it stays behind it.
	if fill, text := strings.Index(painted, " re f"), strings.Index(painted, "BT"); fill < 0 || text < 0 || fill > text {
		t.Fatalf("paint order is wrong: background at %d, text at %d", fill, text)
	}
	validateWithPDFCPU(t, res.PDF)
}

func TestRemoteStylesheetRejected(t *testing.T) {
	src := `<link rel="stylesheet" href="https://example.com/site.css"><p>x</p>`
	res, err := Convert(context.Background(), []byte(src), Options{File: "remote.html"})
	if err == nil {
		t.Fatal("expected an error for a remote stylesheet")
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "css/link-remote" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %v", res.Diagnostics.Lines())
	}
}

func TestScriptIgnoredButContentNotRendered(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<script>var secret = "НЕ РЕНДЕРИТЬ";</script><p>видимый текст</p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "script.html"})
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !hasCode(res.Diagnostics, "html/ignored-script") {
		t.Fatalf("expected a script warning, got %v", res.Diagnostics.Lines())
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "НЕ РЕНДЕРИТЬ") {
		t.Fatalf("script content leaked into the PDF: %q", text)
	}
	if !strings.Contains(text, "видимый текст") {
		t.Fatalf("regular text is missing: %q", text)
	}
}

func TestStrictModeRejectsScripts(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<script>x()</script><p>текст</p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "strict.html", Strict: true})
	if err == nil {
		t.Fatal("strict mode must reject a template with a script")
	}
	if len(res.PDF) != 0 {
		t.Fatal("no PDF must be produced in strict mode")
	}
	if !hasCode(res.Diagnostics, "html/ignored-script") {
		t.Fatalf("diagnostics = %v", res.Diagnostics.Lines())
	}
}

func hasCode(l diag.List, code string) bool {
	for _, d := range l {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestConvertClipsAnOversizedLine(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<style>
		@page { size: 100pt 100pt; margin: 10pt }
		p { font-size: 10pt; line-height: 100pt; margin: 0 }
	</style><p>IMPORTANT</p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "overflow.html"})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !hasCode(res.Diagnostics, "layout/overflow") {
		t.Fatalf("diagnostics = %v", res.Diagnostics.Lines())
	}
	if len(res.PDF) == 0 {
		t.Fatal("a clipped line must still produce a PDF")
	}
	// Strict mode turns the overflow warning into an error.
	strict := Options{FallbackFont: fontPath, File: "overflow.html", Strict: true}
	if _, err := Convert(context.Background(), []byte(src), strict); err == nil {
		t.Fatal("strict mode accepted an overflowing document")
	}
}

func TestKerningIsPreservedInPDF(t *testing.T) {
	fontPath := testFontPath(t)
	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	left, ok1 := f.GlyphIndex('A')
	right, ok2 := f.GlyphIndex('V')
	if !ok1 || !ok2 {
		t.Skip("test font has no A/V")
	}
	if f.Kerning(left, right) == 0 {
		t.Skip("test font has no kerning for AV")
	}
	run, err := text.Shape(f, 12000, "AV")
	if err != nil {
		t.Fatal(err)
	}
	expected := run.Glyphs[0].Advance

	res, err := Convert(context.Background(), []byte(`<p>AV</p>`), Options{FallbackFont: fontPath, File: "kern.html"})
	if err != nil {
		t.Fatal(err)
	}
	var stream string
	for _, s := range pdfdebug.ContentStreams(res.PDF) {
		if strings.Contains(string(s), "TJ") {
			stream = string(s)
		}
	}
	if stream == "" {
		t.Fatal("the text run is not written as a TJ array")
	}
	open := strings.Index(stream, "[")
	closeIdx := strings.Index(stream, "] TJ")
	if open < 0 || closeIdx <= open {
		t.Fatalf("malformed TJ array: %q", stream)
	}
	fields := strings.Fields(stream[open+1 : closeIdx])
	if len(fields) < 3 {
		t.Fatalf("TJ array = %q", stream[open:closeIdx])
	}
	adj, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("adjustment %q is not a number", fields[1])
	}
	w1000 := math.Round(float64(f.Advance(left)) * 1000 / float64(f.UnitsPerEm()))
	nominal := math.Round(w1000 * 12000 / 1000)
	effective := int32(math.Round(nominal - float64(adj)*12000/1000))
	if diff := effective - expected; diff > 12 || diff < -12 {
		t.Fatalf("PDF advance %d differs from layout %d by %d", effective, expected, diff)
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "AV") {
		t.Fatalf("extracted text = %q", text)
	}
}

func TestDocumentWithImagesIsDeterministic(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	for _, name := range []string{"a.png", "b.png"} {
		img := image.NewRGBA(image.Rect(0, 0, 6, 6))
		for i := range img.Pix {
			img.Pix[i] = 200
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	src := `<p>Первое <img src="a.png"> и второе <img src="b.png"></p>`
	opts := Options{FallbackFont: fontPath, BaseDir: dir, File: "images.html"}
	first, err := Convert(context.Background(), []byte(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		next, err := Convert(context.Background(), []byte(src), opts)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.PDF, next.PDF) {
			t.Fatalf("run %d produced different bytes for a document with two images", i+2)
		}
	}
}

func TestRepeatedImageIsEmbeddedOnce(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	path := filepath.Join(dir, "logo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	loader := &imageLoader{baseDir: dir}
	if _, err := loader.Load("logo.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load("logo.png"); err != nil {
		t.Fatal(err)
	}
	if loader.loads != 1 {
		t.Fatalf("the image was decoded %d times, want 1", loader.loads)
	}

	src := `<p><img src="logo.png"> и ещё раз <img src="logo.png"></p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, BaseDir: dir, File: "repeat.html"})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(res.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("the PDF contains %d image objects, want 1", n)
	}
}

func TestMissingStylesheetIsReportedOnce(t *testing.T) {
	dir := t.TempDir()
	src := `<link rel="stylesheet" href="missing.css"><p>x</p>`
	res, err := Convert(context.Background(), []byte(src), Options{BaseDir: dir, File: "missing.html"})
	if err == nil {
		t.Fatal("expected an error for a missing stylesheet")
	}
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == "css/link-load" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the missing stylesheet was reported %d times, want 1: %v", n, res.Diagnostics.Lines())
	}
}

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	for i := range img.Pix {
		img.Pix[i] = 120
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestImageBoundary(t *testing.T) {
	fontPath := testFontPath(t)
	base := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.png")
	writeTestPNG(t, outsideFile)
	insideFile := filepath.Join(base, "inside.png")
	writeTestPNG(t, insideFile)

	symlinkEscape := filepath.Join(base, "escape.png")
	symlinkInside := filepath.Join(base, "alias.png")
	if err := os.Symlink(outsideFile, symlinkEscape); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.Symlink(insideFile, symlinkInside); err != nil {
		t.Fatal(err)
	}

	convert := func(src string) (Result, error) {
		return Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, BaseDir: base, File: "img.html"})
	}

	t.Run("inside", func(t *testing.T) {
		res, err := convert(`<p><img src="inside.png"></p>`)
		if err != nil {
			t.Fatalf("an image inside the base directory must load: %v", err)
		}
		if !strings.Contains(string(res.PDF), "/Subtype /Image") {
			t.Fatal("the image was not embedded")
		}
	})
	t.Run("internal symlink", func(t *testing.T) {
		if _, err := convert(`<p><img src="alias.png"></p>`); err != nil {
			t.Fatalf("a symlink inside the base directory must load: %v", err)
		}
	})
	t.Run("absolute outside", func(t *testing.T) {
		_, err := convert(`<p><img src="` + outsideFile + `"></p>`)
		if err == nil || !strings.Contains(err.Error(), "outside the base directory") {
			t.Fatalf("absolute path outside the base directory: err = %v", err)
		}
	})
	t.Run("escaping symlink", func(t *testing.T) {
		_, err := convert(`<p><img src="escape.png"></p>`)
		if err == nil || !strings.Contains(err.Error(), "outside the base directory") {
			t.Fatalf("escaping symlink: err = %v", err)
		}
	})
}

func TestFontBoundary(t *testing.T) {
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	fontDir := t.TempDir()
	outside := t.TempDir()
	insideFont := filepath.Join(fontDir, "local.ttf")
	if err := os.WriteFile(insideFont, data, 0o644); err != nil {
		t.Fatal(err)
	}
	outsideFont := filepath.Join(outside, "far.ttf")
	if err := os.WriteFile(outsideFont, data, 0o644); err != nil {
		t.Fatal(err)
	}

	convert := func(src string) error {
		_, err := Convert(context.Background(), []byte(src), Options{FontDir: fontDir, File: "font.html"})
		return err
	}

	t.Run("inside the font root", func(t *testing.T) {
		src := `<style>@font-face { font-family: Local; src: url(local.ttf) } p { font-family: Local }</style><p>x</p>`
		if err := convert(src); err != nil {
			t.Fatalf("a font inside the font directory must load: %v", err)
		}
	})
	t.Run("outside the font root", func(t *testing.T) {
		src := `<style>@font-face { font-family: Far; src: url(` + outsideFont + `) } p { font-family: Far }</style><p>x</p>`
		err := convert(src)
		if err == nil || !strings.Contains(err.Error(), "outside the font directory") {
			t.Fatalf("a font outside the font directory: err = %v", err)
		}
	})
}

func TestImageLoaderOpensRootOnce(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		writeTestPNG(t, filepath.Join(dir, name))
	}
	loader := &imageLoader{baseDir: dir}
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		if _, err := loader.Load(name); err != nil {
			t.Fatal(err)
		}
	}
	if loader.rootOpens != 1 {
		t.Fatalf("the root was resolved %d times, want 1", loader.rootOpens)
	}
}

func TestBlockImageIsPainted(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	src := `<p>BEFORE</p><img style="display:block" src="a.png"><p>AFTER</p>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, BaseDir: dir, File: "block.html"})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(res.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("the block image is missing: %d image objects", n)
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "BEFORE") || !strings.Contains(text, "AFTER") {
		t.Fatalf("paragraphs around the image: %q", text)
	}

	only, err := Convert(context.Background(), []byte(`<img style="display:block" src="a.png">`), Options{FallbackFont: fontPath, BaseDir: dir, File: "only.html"})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(only.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("an image-only document must paint its image once, got %d", n)
	}
}

func TestCMYKJPEGIsRejected(t *testing.T) {
	fixture := filepath.Join(runtime.GOROOT(), "src", "image", "testdata", "video-001.cmyk.jpeg")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("no CMYK fixture in this Go distribution: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cmyk.jpg"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	src := `<p><img src="cmyk.jpg"></p>`
	_, err = Convert(context.Background(), []byte(src), Options{BaseDir: dir, File: "cmyk.html"})
	if err == nil || !strings.Contains(err.Error(), "CMYK") {
		t.Fatalf("a CMYK JPEG must be rejected explicitly, got %v", err)
	}
}

func TestFontFamilyAliasesShareOneFile(t *testing.T) {
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.ttf"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	sheet := css.ParseSheet(css.SheetOptions{File: "aliases.css"}, []byte(`
		@font-face { font-family: One; src: url(f.ttf) }
		@font-face { font-family: Two; src: url(f.ttf) }
	`))
	reg, err := newFontRegistry(context.Background(), nil, []*css.Stylesheet{sheet}, Options{BaseDir: dir, FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatal(err)
	}
	one, err := reg.Face([]string{"One"}, 400, false, 12000)
	if err != nil {
		t.Fatalf("family One: %v", err)
	}
	two, err := reg.Face([]string{"Two"}, 400, false, 12000)
	if err != nil {
		t.Fatalf("family Two: %v", err)
	}
	if one.Font != two.Font {
		t.Fatal("both aliases must resolve to the same parsed font")
	}
	if one.Font == reg.fallback {
		t.Fatal("the alias must not fall back to the system font")
	}
}

func countFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skipf("/dev/fd is not available: %v", err)
	}
	return len(entries)
}

func TestConversionsDoNotLeakDescriptors(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "i.png"))
	src := []byte(`<p><img src="i.png"></p>`)
	opts := Options{FallbackFont: fontPath, BaseDir: dir, File: "fd.html"}
	if _, err := Convert(context.Background(), src, opts); err != nil {
		t.Fatal(err)
	}
	before := countFDs(t)
	for i := 0; i < 20; i++ {
		if _, err := Convert(context.Background(), src, opts); err != nil {
			t.Fatal(err)
		}
	}
	after := countFDs(t)
	if after > before+2 {
		t.Fatalf("descriptors grew from %d to %d without GC", before, after)
	}
}

func TestValidateUsesTheSamePreparationAsConvert(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// A declared font that cannot be loaded.
	src := []byte(`<style>@font-face { font-family: Missing; src: url(missing.ttf) } body { font-family: Missing }</style><p>A</p>`)
	diags, err := Validate(ctx, src, Options{BaseDir: dir, File: "missing-font.html"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, "font/unavailable") {
		t.Fatalf("Validate missed an unloadable font: %v", diags.Lines())
	}

	// A span that layout would reject.
	diags, err = Validate(ctx, []byte(`<table><tr><td rowspan="4294967296">A</td></tr></table>`), Options{File: "span.html"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, "html/invalid-span") {
		t.Fatalf("Validate missed the span bound: %v", diags.Lines())
	}

	// Invalid page options.
	diags, err = Validate(ctx, []byte(`<p>A</p>`), Options{PageSize: "nonsense", File: "page.html"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, "options/page-size") {
		t.Fatalf("Validate missed an invalid page size: %v", diags.Lines())
	}

	// A clean template still validates cleanly.
	diags, err = Validate(ctx, []byte(`<p>A</p>`), Options{FallbackFont: testFontPath(t), File: "ok.html"})
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("clean template: %v", diags.Lines())
	}
}

func TestBlockImageInsideTableCell(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "i.png"))
	src := `<table><tr><td><img style="display:block;width:10pt;height:10pt" src="i.png"></td></tr></table>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, BaseDir: dir, File: "cell.html"})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(res.PDF), "/Subtype /Image"); n != 1 {
		t.Fatalf("the image inside the cell is missing: %d image objects", n)
	}
}

func TestTransparentTextColourIsRejected(t *testing.T) {
	diags, err := Validate(context.Background(), []byte(`<p style="color:transparent">HIDDEN</p>`), Options{FallbackFont: testFontPath(t), File: "transparent.html"})
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() {
		t.Fatalf("color:transparent was accepted: %v", diags.Lines())
	}
}

func TestInlineBlockIsRejected(t *testing.T) {
	diags, err := Validate(context.Background(), []byte(`<p><span style="display:inline-block;width:100pt">A</span>B</p>`), Options{FallbackFont: testFontPath(t), File: "ib.html"})
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() {
		t.Fatalf("display:inline-block was accepted: %v", diags.Lines())
	}
}

func TestConversionHonoursCancellation(t *testing.T) {
	fontPath := testFontPath(t)
	var b strings.Builder
	for i := 0; i < 1500; i++ {
		b.WriteString("<p>Строка документа для проверки отмены.</p>")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		cancel()
	}()
	res, err := Convert(ctx, []byte(b.String()), Options{FallbackFont: fontPath, File: "cancel.html"})
	if err == nil {
		t.Fatalf("a cancelled conversion must fail, got a %d-byte PDF", len(res.PDF))
	}
	if len(res.PDF) != 0 {
		t.Fatal("no PDF may be written after cancellation")
	}
}

func TestResourceLimits(t *testing.T) {
	fontPath := testFontPath(t)
	res, err := Convert(context.Background(), []byte(`<p>A</p>`), Options{
		FallbackFont: fontPath,
		File:         "limit.html",
		Limits:       Limits{MaxInputBytes: 4},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("input limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}

	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString("<p>Строка для проверки лимита страниц.</p>")
	}
	res, err = Convert(context.Background(), []byte(b.String()), Options{
		FallbackFont: fontPath,
		File:         "pages.html",
		Limits:       Limits{MaxPages: 1},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("page limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}

	// Forced breaks create pages inside one block; the limit must stop them
	// at creation instead of after the whole layout.
	var forced strings.Builder
	for i := 0; i < 200; i++ {
		forced.WriteString(`<p style="break-after:page">Страница.</p>`)
	}
	forced.WriteString("<p>Последний абзац.</p>")
	res, err = Convert(context.Background(), []byte(forced.String()), Options{
		FallbackFont: fontPath,
		File:         "forced-pages.html",
		Limits:       Limits{MaxPages: 1},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("page limit did not stop forced breaks: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

func TestImageAndFontLimits(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "i.png"))
	res, err := Convert(context.Background(), []byte(`<p><img src="i.png"></p>`), Options{
		FallbackFont: fontPath, BaseDir: dir, File: "img.html",
		Limits: Limits{MaxImagePixels: 4},
	})
	if err == nil || len(res.PDF) != 0 {
		t.Fatalf("image limit was not enforced: err=%v", err)
	}

	sheet := `<style>@font-face { font-family: Big; src: url(f.ttf) } body { font-family: Big }</style><p>A</p>`
	data, readErr := os.ReadFile(fontPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.ttf"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Convert(context.Background(), []byte(sheet), Options{
		BaseDir: dir, File: "fontlimit.html",
		Limits: Limits{MaxFontBytes: 10},
	})
	if err == nil || len(res.PDF) != 0 {
		t.Fatalf("font limit was not enforced: err=%v", err)
	}
}

func TestCancellationStopsLayout(t *testing.T) {
	fontPath := testFontPath(t)
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		b.WriteString("<p>Строка документа для проверки отмены во время раскладки.</p>")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	res, err := Convert(ctx, []byte(b.String()), Options{FallbackFont: fontPath, File: "cancel-layout.html"})
	if err == nil {
		t.Fatalf("a cancelled layout must fail, got a %d-byte PDF", len(res.PDF))
	}
	if len(res.PDF) != 0 {
		t.Fatal("no PDF may be written after cancellation")
	}
}

func TestStageResourceBudgets(t *testing.T) {
	fontPath := testFontPath(t)
	cases := []struct {
		name   string
		src    string
		limits Limits
	}{
		{
			name:   "dom depth",
			src:    strings.Repeat("<div>", 40) + "x",
			limits: Limits{MaxDOMDepth: 8},
		},
		{
			name:   "table cells",
			src:    `<table>` + strings.Repeat(`<tr><td>A</td><td>B</td></tr>`, 10) + `</table>`,
			limits: Limits{MaxTableCells: 4},
		},
		{
			name:   "total operations",
			src:    strings.Repeat("<p>Строка для бюджета операций.</p>", 20),
			limits: Limits{MaxOperations: 20},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Convert(context.Background(), []byte(tc.src), Options{
				FallbackFont: fontPath,
				File:         tc.name + ".html",
				Limits:       tc.limits,
			})
			if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
				t.Fatalf("limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
			}
		})
	}
}

func TestStylesheetByteLimit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.css"), bytes.Repeat([]byte("p { color: #ff0000 }\n"), 40), 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`<link rel="stylesheet" href="big.css"><p>x</p>`)
	res, err := Convert(context.Background(), src, Options{
		FallbackFont: testFontPath(t),
		BaseDir:      dir,
		File:         "stylesheet.html",
		Limits:       Limits{MaxStylesheetBytes: 32},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("stylesheet size limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

func TestValidateHonoursCancellationAfterStart(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString("<p>Строка документа для проверки отмены проверки.</p>")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	_, err := Validate(ctx, []byte(b.String()), Options{FallbackFont: testFontPath(t), File: "cancel-validate.html"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Validate returned %v, want context.Canceled", err)
	}
}

func TestPageRasterLimit(t *testing.T) {
	fontPath := testFontPath(t)
	// 100,000 pt square at 96 dpi is about 133,333 x 133,333 pixels: 17.8
	// billion, far above the default raster budget. The debug raster is the
	// only place that allocates it, so the PDF itself must not be built by an
	// out-of-memory allocation.
	src := []byte(`<style>@page{size:100000pt 100000pt;margin:0}</style><p>A</p>`)
	res, err := Convert(context.Background(), src, Options{
		FallbackFont: fontPath,
		File:         "big-page.html",
		RenderPNG:    true,
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PNG) != 0 {
		t.Fatalf("page raster limit was not enforced: err=%v diags=%v png=%d", err, res.Diagnostics.Lines(), len(res.PNG))
	}
	// The same page with an explicit budget below one page pixel area.
	res, err = Convert(context.Background(), []byte(`<p>A</p>`), Options{
		FallbackFont: fontPath,
		File:         "small-raster.html",
		RenderPNG:    true,
		Limits:       Limits{MaxRasterPixels: 10},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PNG) != 0 {
		t.Fatalf("custom raster limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

func TestFallbackAndImageByteLimits(t *testing.T) {
	fontPath := testFontPath(t)
	// A fallback font is a font like any other: MaxFontBytes applies to it.
	res, err := Convert(context.Background(), []byte(`<p>A</p>`), Options{
		FallbackFont: fontPath,
		File:         "fallback-limit.html",
		Limits:       Limits{MaxFontBytes: 10},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("fallback font limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}

	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "i.png"))
	res, err = Convert(context.Background(), []byte(`<p><img src="i.png"></p>`), Options{
		FallbackFont: fontPath,
		BaseDir:      dir,
		File:         "image-bytes.html",
		Limits:       Limits{MaxImageBytes: 10},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("image byte limit was not enforced: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// diagSignature renders a diagnostic list as comparable strings.
func diagSignature(l diag.List) []string { return l.Lines() }

func TestValidateMatchesConvertOnAssetsAndGlyphs(t *testing.T) {
	fontPath := testFontPath(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.png"), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		src     string
		baseDir string
	}{
		{"missing image", `<p>A</p><img src="missing.png">`, dir},
		{"corrupt image", `<p>A</p><img src="broken.png">`, dir},
		{"missing glyph", `<p>&#x10ffff;</p>`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{FallbackFont: fontPath, BaseDir: tc.baseDir, File: tc.name + ".html"}
			converted, convertErr := Convert(context.Background(), []byte(tc.src), opts)
			if convertErr == nil {
				t.Fatalf("%s: Convert unexpectedly succeeded", tc.name)
			}
			validated, validateErr := Validate(context.Background(), []byte(tc.src), opts)
			if validateErr != nil {
				t.Fatalf("%s: Validate returned a context error: %v", tc.name, validateErr)
			}
			if !validated.HasErrors() {
				t.Fatalf("%s: Validate reported success for a document Convert rejects", tc.name)
			}
			got, want := diagSignature(validated), diagSignature(converted.Diagnostics)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("%s: Validate diagnostics differ from Convert\n got: %v\nwant: %v", tc.name, got, want)
			}
		})
	}
}

func TestValidateMatchesConvertOnCMYK(t *testing.T) {
	fixture := filepath.Join(runtime.GOROOT(), "src", "image", "testdata", "video-001.cmyk.jpeg")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("no CMYK fixture in this Go distribution: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cmyk.jpg"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte(`<p><img src="cmyk.jpg"></p>`)
	opts := Options{FallbackFont: testFontPath(t), BaseDir: dir, File: "cmyk.html"}
	converted, convertErr := Convert(context.Background(), src, opts)
	if convertErr == nil || !strings.Contains(convertErr.Error(), "CMYK") {
		t.Fatalf("Convert returned %v, want a CMYK rejection", convertErr)
	}
	validated, validateErr := Validate(context.Background(), src, opts)
	if validateErr != nil {
		t.Fatal(validateErr)
	}
	if strings.Join(validated.Lines(), "\n") != strings.Join(converted.Diagnostics.Lines(), "\n") {
		t.Fatalf("Validate diagnostics differ from Convert\n got: %v\nwant: %v", validated.Lines(), converted.Diagnostics.Lines())
	}
}

// pickWeight returns the weight of the face that pick selects for a request.
func pickWeight(t *testing.T, target int, italic bool, available ...int) int {
	t.Helper()
	r := &fontRegistry{}
	byWeight := map[int]*font.Font{}
	for _, w := range available {
		f := &font.Font{}
		byWeight[w] = f
		r.faces = append(r.faces, &registeredFace{family: "w", weight: w, font: f})
	}
	got := r.pick([]string{"w"}, target, italic)
	for w, f := range byWeight {
		if f == got {
			return w
		}
	}
	return -1
}

func TestFontWeightFallbackOrder(t *testing.T) {
	cases := []struct {
		name      string
		target    int
		available []int
		want      int
	}{
		{"closest below 400 wins over 100", 500, []int{100, 400}, 400},
		{"500 is preferred below 400 for 400", 400, []int{300, 500}, 500},
		{"lighter weights first below 400", 400, []int{100, 600}, 100},
		{"heavier weights first above 500", 700, []int{400, 800}, 800},
		{"lighter descending below 400 target", 300, []int{200, 400}, 200},
		{"band target..500 first", 450, []int{400, 500}, 500},
		{"exact match wins", 600, []int{500, 600, 700}, 600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickWeight(t, tc.target, false, tc.available...); got != tc.want {
				t.Fatalf("pick(%d) from %v = %d, want %d", tc.target, tc.available, got, tc.want)
			}
		})
	}
}

func TestFontWeightKeepsItalicPreference(t *testing.T) {
	r := &fontRegistry{}
	regular := &font.Font{}
	italic := &font.Font{}
	r.faces = append(r.faces,
		&registeredFace{family: "w", weight: 400, italic: false, font: regular},
		&registeredFace{family: "w", weight: 400, italic: true, font: italic},
	)
	if got := r.pick([]string{"w"}, 400, true); got != italic {
		t.Fatal("an italic request did not prefer the italic face")
	}
	if got := r.pick([]string{"w"}, 400, false); got != regular {
		t.Fatal("an upright request did not prefer the upright face")
	}
}

func TestTransparentPNGKeepsAlphaInPDF(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	data, err := os.Create(filepath.Join(dir, "t.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(data, img); err != nil {
		data.Close()
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	src := `<div style="background-color:red"><img src="t.png" style="width:20pt;height:20pt"></div>`
	res, err := Convert(context.Background(), []byte(src), Options{
		FallbackFont: testFontPath(t),
		BaseDir:      dir,
		File:         "alpha.html",
	})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.Contains(res.PDF, []byte("/SMask")) {
		t.Fatal("the translucent PNG was flattened instead of using a soft mask")
	}
	validateWithPDFCPU(t, res.PDF)
}

func TestKeywordsAreCopiedToThePDF(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<html><head><meta name="keywords" content="счёт, договор, test"></head><body><p>A</p></body></html>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "keywords.html"})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	pdf := string(res.PDF)
	if !strings.Contains(pdf, "/Keywords") {
		t.Fatal("the PDF has no /Keywords entry")
	}
	// The value contains Cyrillic, so it must be a UTF-16 string.
	var hex strings.Builder
	hex.WriteString("<FEFF")
	for _, r := range "счёт, договор, test" {
		hex.WriteString(fmt.Sprintf("%04X", r))
	}
	hex.WriteString(">")
	if !strings.Contains(pdf, "/Keywords "+hex.String()) {
		t.Fatalf("the PDF does not contain the UTF-16 keywords string %s", hex.String())
	}
	validateWithPDFCPU(t, res.PDF)

	// Without the meta tag no empty /Keywords is written.
	res, err = Convert(context.Background(), []byte(`<p>A</p>`), Options{FallbackFont: fontPath, File: "no-keywords.html"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.PDF), "/Keywords") {
		t.Fatal("an absent keywords meta produced a /Keywords entry")
	}
}

func TestTargetCounterResolvesInsideATable(t *testing.T) {
	fontPath := testFontPath(t)
	src := `<style>a::after{content:target-counter(attr(href),page)}</style>` +
		`<p style="margin:0"><a href="#target">PAGE</a></p>` +
		`<table><tr><td><h2 id="target">TITLE</h2></td></tr></table>`
	res, err := Convert(context.Background(), []byte(src), Options{FallbackFont: fontPath, File: "target-table.html"})
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if hasCode(res.Diagnostics, "pagination/unknown-target") {
		t.Fatalf("the id inside the cell was reported as unknown: %v", res.Diagnostics.Lines())
	}
	text, err := extractText(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "PAGE1") {
		t.Fatalf("target-counter did not show page 1: %q", text)
	}
}
