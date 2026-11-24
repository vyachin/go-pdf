package engine

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
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
func extractText(data []byte) (string, error) {
	objects := splitObjects(data)
	toUni := map[uint16]rune{}
	// Collect the ToUnicode maps first.
	for _, obj := range objects {
		if !bytes.Contains(obj.body, []byte("/ToUnicode")) {
			continue
		}
		idx := bytes.Index(obj.body, []byte("/ToUnicode"))
		ref := parseIntToken(obj.body[idx+len("/ToUnicode"):])
		target, ok := objects[ref]
		if !ok {
			continue
		}
		stream, err := decodedStream(target.body)
		if err != nil {
			continue
		}
		parseBFChar(stream, toUni)
	}
	var out strings.Builder
	for _, obj := range objects {
		if bytes.Contains(obj.body, []byte("/ToUnicode")) && !bytes.Contains(obj.body, []byte("/Type /Font")) {
			continue
		}
		stream, err := decodedStream(obj.body)
		if err != nil {
			if !bytes.Contains(obj.body, []byte("Tj")) {
				continue
			}
			stream = obj.body
		}
		out.WriteString(textFromStream(stream, toUni))
	}
	return out.String(), nil
}

type pdfObject struct {
	body []byte
}

func splitObjects(data []byte) map[int]*pdfObject {
	objects := map[int]*pdfObject{}
	rest := data
	for {
		idx := bytes.Index(rest, []byte(" obj\n"))
		if idx < 0 {
			break
		}
		lineStart := bytes.LastIndexByte(rest[:idx], '\n') + 1
		fields := strings.Fields(string(rest[lineStart:idx]))
		if len(fields) < 1 {
			rest = rest[idx+len(" obj\n"):]
			continue
		}
		id := 0
		if _, err := fmt.Sscanf(fields[0], "%d", &id); err != nil {
			rest = rest[idx+len(" obj\n"):]
			continue
		}
		body := rest[idx+len(" obj\n"):]
		endIdx := bytes.Index(body, []byte("\nendobj"))
		if endIdx < 0 {
			break
		}
		objects[id] = &pdfObject{body: body[:endIdx]}
		rest = body[endIdx+len("\nendobj"):]
	}
	return objects
}

func decodedStream(body []byte) ([]byte, error) {
	idx := bytes.Index(body, []byte("stream\n"))
	if idx < 0 {
		return nil, fmt.Errorf("no stream")
	}
	start := idx + len("stream\n")
	end := bytes.Index(body[start:], []byte("\nendstream"))
	if end < 0 {
		return nil, fmt.Errorf("no endstream")
	}
	raw := body[start : start+end]
	if !bytes.Contains(body[:idx], []byte("FlateDecode")) {
		return raw, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(zr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseBFChar(stream []byte, out map[uint16]rune) {
	for _, line := range bytes.Split(stream, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) < 10 || line[0] != '<' {
			continue
		}
		endA := bytes.IndexByte(line, '>')
		if endA < 0 {
			continue
		}
		hexA := string(line[1:endA])
		rest := bytes.TrimSpace(line[endA+1:])
		if len(rest) < 3 || rest[0] != '<' {
			continue
		}
		endB := bytes.IndexByte(rest, '>')
		if endB < 0 {
			continue
		}
		hexB := string(rest[1:endB])
		gidBytes, err := hex.DecodeString(hexA)
		if err != nil || len(gidBytes) != 2 {
			continue
		}
		gid := uint16(gidBytes[0])<<8 | uint16(gidBytes[1])
		if runes := decodeUTF16Hex(hexB); len(runes) > 0 {
			out[gid] = runes[0]
		}
	}
}

func decodeUTF16Hex(s string) []rune {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw)%2 != 0 {
		return nil
	}
	var runes []rune
	for i := 0; i+1 < len(raw); i += 2 {
		runes = append(runes, rune(uint16(raw[i])<<8|uint16(raw[i+1])))
	}
	return runes
}

func textFromStream(stream []byte, toUni map[uint16]rune) string {
	var out strings.Builder
	s := string(stream)
	idx := 0
	for {
		rel := strings.Index(s[idx:], "<")
		if rel < 0 {
			break
		}
		open := idx + rel
		closeRel := strings.Index(s[open:], ">")
		if closeRel < 0 {
			break
		}
		closeIdx := open + closeRel
		hexStr := s[open+1 : closeIdx]
		after := s[closeIdx+1:]
		tail := after
		if len(tail) > 8 {
			tail = tail[:8]
		}
		if strings.Contains(tail, "Tj") {
			if raw, err := hex.DecodeString(hexStr); err == nil {
				for i := 0; i+1 < len(raw); i += 2 {
					gid := uint16(raw[i])<<8 | uint16(raw[i+1])
					if r, ok := toUni[gid]; ok {
						out.WriteRune(r)
					}
				}
			}
		}
		idx = closeIdx + 1
	}
	return out.String()
}

func parseIntToken(b []byte) int {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n') {
		i++
	}
	start := i
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	var id int
	fmt.Sscanf(string(b[start:i]), "%d", &id)
	return id
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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
	for _, obj := range splitObjects(res.PDF) {
		stream, err := decodedStream(obj.body)
		if err != nil {
			continue
		}
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

func buildToUni(objects map[int]*pdfObject) map[uint16]rune {
	toUni := map[uint16]rune{}
	for _, obj := range objects {
		if !bytes.Contains(obj.body, []byte("/ToUnicode")) {
			continue
		}
		idx := bytes.Index(obj.body, []byte("/ToUnicode"))
		ref := parseIntToken(obj.body[idx+len("/ToUnicode"):])
		target, ok := objects[ref]
		if !ok {
			continue
		}
		stream, err := decodedStream(target.body)
		if err != nil {
			continue
		}
		parseBFChar(stream, toUni)
	}
	return toUni
}

func extractPageTexts(data []byte) ([]string, error) {
	objects := splitObjects(data)
	toUni := buildToUni(objects)
	type entry struct {
		id   int
		text string
	}
	var pages []entry
	for id, obj := range objects {
		stream, err := decodedStream(obj.body)
		if err != nil {
			continue
		}
		if !bytes.Contains(stream, []byte("Tj")) {
			continue
		}
		pages = append(pages, entry{id: id, text: textFromStream(stream, toUni)})
	}
	// The writer allocates content streams in page order.
	for i := 0; i < len(pages); i++ {
		for j := i + 1; j < len(pages); j++ {
			if pages[j].id < pages[i].id {
				pages[i], pages[j] = pages[j], pages[i]
			}
		}
	}
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.text
	}
	return out, nil
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
