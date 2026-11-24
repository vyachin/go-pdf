package engine

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

func testFontPath(t testing.TB) string {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
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

func validateWithPDFCPU(t *testing.T, data []byte) {
	t.Helper()
	bin := os.Getenv("PDFCPU")
	if bin == "" {
		bin = "/tmp/gobin/pdfcpu"
	}
	if _, err := os.Stat(bin); err != nil {
		t.Log("pdfcpu not available, skipping independent validation")
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
	if !bytes.Contains(res.PDF, []byte("/BaseFont /Arial")) && !bytes.Contains(res.PDF, []byte("/BaseFont /DejaVuSans")) && !bytes.Contains(res.PDF, []byte("/BaseFont /LiberationSerif")) {
		t.Fatalf("the declared font is not embedded: %q", res.PDF[:min(200, len(res.PDF))])
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
		if strings.Contains(s, "BT") && strings.Contains(s, "Tf") && strings.Contains(s, "Tj") && strings.Contains(s, "re") {
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
