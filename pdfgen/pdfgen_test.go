package pdfgen_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/engine"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
	"github.com/vyachin/go-pdf/pdfgen"
)

const invoiceHTML = `<html><head><title>Счёт № 42</title>
<style>
  @page { size: A4; margin: 20mm }
  body { font-family: "PT Serif"; font-size: 12pt }
  h1 { font-size: 18pt }
</style></head>
<body><h1>Счёт № 42</h1><p>Итого к оплате: 1 200,00 руб.</p></body></html>`

// testFontPath finds a TrueType font for templates that do not ship one, so the
// tests run on every machine and in CI.
func testFontPath(t testing.TB) string {
	t.Helper()
	candidates := []string{}
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, filepath.Join("..", "testdata", "fonts", "DejaVuSans.ttf"))
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no TrueType font available; set PDFTEST_FONT")
	return ""
}

func testOptions(t testing.TB) pdfgen.Options {
	return pdfgen.Options{FallbackFont: testFontPath(t), File: "invoice.html"}
}

func TestConvertProducesPDF(t *testing.T) {
	res, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), testOptions(t))
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.HasPrefix(res.PDF, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF: %q", res.PDF[:min(16, len(res.PDF))])
	}
	if res.Diagnostics.HasErrors() {
		t.Fatalf("unexpected errors: %v", res.Diagnostics)
	}
}

func TestConvertIsDeterministic(t *testing.T) {
	opts := testOptions(t)
	first, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), opts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	second, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), opts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !bytes.Equal(first.PDF, second.PDF) {
		t.Fatal("two runs produced different PDFs")
	}

	opts.Date = time.Date(2026, time.November, 24, 12, 0, 0, 0, time.UTC)
	dated, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), opts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if bytes.Equal(dated.PDF, first.PDF) {
		t.Fatal("Options.Date did not reach the PDF")
	}
}

func TestConvertRendersPNG(t *testing.T) {
	opts := testOptions(t)
	opts.RenderPNG = true
	res, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), opts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !bytes.HasPrefix(res.PNG, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("result PNG is not a PNG: %q", res.PNG[:min(8, len(res.PNG))])
	}
}

func TestConvertInvalidTemplateReturnsError(t *testing.T) {
	src := `<html><body><p style="flex: 1">x</p></body></html>`
	res, err := pdfgen.Convert(context.Background(), []byte(src), testOptions(t))
	if err == nil {
		t.Fatal("Convert accepted a property outside the profile")
	}
	var verr *pdfgen.Error
	if !errors.As(err, &verr) {
		t.Fatalf("error is %T, want *pdfgen.Error", err)
	}
	if len(verr.Diagnostics) == 0 || !verr.Diagnostics.HasErrors() {
		t.Fatalf("no error diagnostics: %v", verr.Diagnostics)
	}
	if res.PDF != nil {
		t.Fatal("invalid template produced a PDF")
	}
	// The message names the property, so a user can act on it.
	if !strings.Contains(err.Error(), "flex") {
		t.Fatalf("error %q does not mention the offending property", err)
	}
}

func TestConvertReportsWarnings(t *testing.T) {
	src := `<html><body><p>x</p><script>1</script></body></html>`
	res, err := pdfgen.Convert(context.Background(), []byte(src), testOptions(t))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "html/ignored-script" && d.Severity == pdfgen.SeverityWarning {
			found = true
			if d.File != "invoice.html" || d.Line == 0 {
				t.Fatalf("warning lost its position: %+v", d)
			}
		}
	}
	if !found {
		t.Fatalf("no ignored-script warning in %v", res.Diagnostics)
	}
}

func TestStrictTurnsWarningsIntoErrors(t *testing.T) {
	src := `<html><body><p>x</p><script>1</script></body></html>`
	opts := testOptions(t)
	opts.Strict = true
	_, err := pdfgen.Convert(context.Background(), []byte(src), opts)
	var verr *pdfgen.Error
	if !errors.As(err, &verr) {
		t.Fatalf("strict mode returned %v, want *pdfgen.Error", err)
	}
}

func TestValidateAcceptsAndRejects(t *testing.T) {
	diags, err := pdfgen.Validate(context.Background(), []byte(invoiceHTML), testOptions(t))
	if err != nil {
		t.Fatalf("Validate: %v\n%s", err, strings.Join(diags.Lines(), "\n"))
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	_, err = pdfgen.Validate(context.Background(), []byte(`<html><body><p style="flex: 1">x</p>`), testOptions(t))
	var verr *pdfgen.Error
	if !errors.As(err, &verr) {
		t.Fatalf("Validate returned %v, want *pdfgen.Error", err)
	}
}

func TestLimitsAreEnforced(t *testing.T) {
	opts := testOptions(t)
	opts.Limits.MaxInputBytes = 16
	_, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), opts)
	var verr *pdfgen.Error
	if !errors.As(err, &verr) {
		t.Fatalf("Convert returned %v, want *pdfgen.Error", err)
	}
	if verr.Diagnostics[0].Code != "resource/limit" {
		t.Fatalf("diagnostic code is %q, want resource/limit", verr.Diagnostics[0].Code)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pdfgen.Convert(ctx, []byte(invoiceHTML), testOptions(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Convert returned %v, want context.Canceled", err)
	}
	if _, err := pdfgen.Validate(ctx, []byte(invoiceHTML), testOptions(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Validate returned %v, want context.Canceled", err)
	}
}

func TestDiagnosticFormat(t *testing.T) {
	d := pdfgen.Diagnostic{
		Severity: pdfgen.SeverityError,
		Code:     "css/unsupported-value",
		Message:  `unsupported value "1fr"`,
		File:     "invoice.html",
		Line:     7,
		Column:   12,
		Element:  "body > table",
	}
	want := `invoice.html:7:12: error[css/unsupported-value]: unsupported value "1fr" (at body > table)`
	if got := d.String(); got != want {
		t.Fatalf("Format:\n got %s\nwant %s", got, want)
	}
}

func TestValidationHonoursCancellationAfterStart(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString("<p>Строка документа для проверки отмены.</p>")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Millisecond)
		cancel()
	}()
	if _, err := pdfgen.Validate(ctx, []byte(b.String()), testOptions(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Validate returned %v, want context.Canceled", err)
	}
}

func TestValidateReportsMissingImages(t *testing.T) {
	opts := testOptions(t)
	opts.BaseDir = t.TempDir()
	_, err := pdfgen.Validate(context.Background(), []byte(`<p>A</p><img src="missing.png">`), opts)
	var verr *pdfgen.Error
	if !errors.As(err, &verr) {
		t.Fatalf("Validate returned %v, want *pdfgen.Error for a missing image", err)
	}
	if verr.Diagnostics[0].Code != "img/load" {
		t.Fatalf("diagnostic code is %q, want img/load", verr.Diagnostics[0].Code)
	}
}

func TestPageSizeWithSharedUnit(t *testing.T) {
	opts := testOptions(t)
	opts.PageSize = "210x297mm"
	res, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), opts)
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.Contains(res.PDF, []byte("/MediaBox [0 0 595.27")) {
		t.Fatal("the documented page size did not reach the PDF")
	}
}

// countingContext reports context.Canceled once Err has been called limit
// times, which lets the test cancel at a chosen pipeline stage without timers.
type countingContext struct {
	calls, limit int
}

func (c *countingContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *countingContext) Done() <-chan struct{}       { return nil }
func (c *countingContext) Value(any) any               { return nil }
func (c *countingContext) Err() error {
	c.calls++
	if c.calls >= c.limit {
		return context.Canceled
	}
	return nil
}

// TestCancellationBeatsTemplateErrors guards RV2-21: when the template had
// already produced validation errors, Convert returned *pdfgen.Error and hid
// the fact that the caller cancelled the conversion.
func TestCancellationBeatsTemplateErrors(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString(`<p style="flex:1">x</p>`)
	}
	src := []byte(b.String())
	opts := pdfgen.Options{File: "cancel.html"}
	// Count how often the pipeline asks for cancellation, then cancel halfway
	// through: by then the profile errors already exist, which used to mask the
	// cancellation.
	counter := &countingContext{limit: 1 << 30}
	if _, err := pdfgen.Convert(counter, src, opts); err == nil {
		t.Fatal("the invalid template validated successfully")
	}
	if counter.calls < 4 {
		t.Fatalf("the pipeline checked cancellation only %d times", counter.calls)
	}
	for _, limit := range []int{counter.calls / 2, counter.calls - 1} {
		ctx := &countingContext{limit: limit}
		_, err := pdfgen.Convert(ctx, src, opts)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("limit %d of %d: Convert returned %T (%v), want context.Canceled", limit, counter.calls, err, err)
		}
	}
	// Without cancellation the same template still reports the profile errors.
	if _, err := pdfgen.Convert(context.Background(), src, opts); err == nil {
		t.Fatal("an invalid template without cancellation must fail")
	}
}

// TestConvertConcurrent pins the concurrency guarantee the package documents:
// conversions share no mutable state, so parallel results equal sequential ones.
// CI runs the suite under the race detector.
func TestConvertConcurrent(t *testing.T) {
	opts := testOptions(t)
	docs := []string{
		invoiceHTML,
		`<style>p{font-size:10pt}</style><p>Второй документ для параллельной конвертации.</p>`,
		invoiceHTML,
	}
	sequential := make([][]byte, len(docs))
	for i, doc := range docs {
		res, err := pdfgen.Convert(context.Background(), []byte(doc), opts)
		if err != nil {
			t.Fatalf("sequential conversion %d: %v", i, err)
		}
		sequential[i] = res.PDF
	}

	const workers = 8
	results := make([][][]byte, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			results[w] = make([][]byte, len(docs))
			for i, doc := range docs {
				res, err := pdfgen.Convert(context.Background(), []byte(doc), opts)
				if err != nil {
					errs <- fmt.Errorf("worker %d, document %d: %w", w, i, err)
					return
				}
				results[w][i] = res.PDF
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for w := range results {
		for i := range docs {
			if !bytes.Equal(results[w][i], sequential[i]) {
				t.Fatalf("worker %d produced different bytes for document %d", w, i)
			}
		}
	}
}

// TestConvertWritesNothing pins the no-disk guarantee: a conversion reads the
// assets it needs and returns bytes, nothing more.
func TestConvertWritesNothing(t *testing.T) {
	font, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	opts := pdfgen.Options{FallbackFont: font, File: "invoice.html"}
	t.Chdir(t.TempDir())

	before := workingDirectoryNames(t)
	if _, err := pdfgen.Convert(context.Background(), []byte(invoiceHTML), opts); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if _, err := pdfgen.Validate(context.Background(), []byte(invoiceHTML), opts); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	after := workingDirectoryNames(t)
	if !slices.Equal(before, after) {
		t.Fatalf("the conversion changed the working directory:\nbefore: %v\nafter:  %v", before, after)
	}
}

func workingDirectoryNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

// TestDefaultOptions pins the documented defaults: the zero Options value is a
// complete request for a profile document that declares its font.
func TestDefaultOptions(t *testing.T) {
	font, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	doc := `<html><head><style>
  @font-face { font-family: "Bundled"; src: url(` + font + `) }
  body { font-family: "Bundled"; font-size: 12pt }
</style></head><body><p>Значения по умолчанию.</p></body></html>`

	res, err := pdfgen.Convert(context.Background(), []byte(doc), pdfgen.Options{})
	if err != nil {
		t.Fatalf("Convert with zero options: %v", err)
	}
	pdf := string(res.PDF)
	for _, want := range []string{
		"/MediaBox [0 0 595.276 841.89",   // A4, the documented default size.
		"/CreationDate (D:20000101000000", // The fixed default date.
	} {
		if !strings.Contains(pdf, want) {
			t.Fatalf("the default-options PDF does not contain %q", want)
		}
	}
}

// TestDefaultMargin pins the documented default margin at 20 mm: the content
// area starts one margin from the page edge, measured on the rendered raster.
func TestDefaultMargin(t *testing.T) {
	font, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	src := `<style>div{height:20mm;background-color:#ff0000}</style><div></div>`
	res, err := pdfgen.Convert(context.Background(), []byte(src), pdfgen.Options{
		FallbackFont: font,
		RenderPNG:    true,
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(res.PNG))
	if err != nil {
		t.Fatalf("decoding the preview: %v", err)
	}

	// The raster resolution follows the page size, so derive it from the image
	// width and A4's 210 mm rather than assuming a fixed dpi.
	bounds := img.Bounds()
	dpi := float64(bounds.Dx()) / (210.0 / 25.4)
	margin := int(math.Round(20.0 / 25.4 * dpi))
	if margin <= 0 || bounds.Dx() < 4*margin {
		t.Fatalf("unexpected raster %v for A4: margin would be %d px", bounds, margin)
	}

	isRed := func(x, y int) bool {
		r, g, _, _ := img.At(x, y).RGBA()
		return r>>8 > 200 && g>>8 < 100
	}
	probeY := margin + 20
	for _, tc := range []struct {
		name string
		x    int
		red  bool
	}{
		{"left margin is blank", margin - 10, false},
		{"content starts at the left margin", margin + 10, true},
		{"content ends at the right margin", bounds.Dx() - margin - 10, true},
		{"right margin is blank", bounds.Dx() - margin + 10, false},
	} {
		if got := isRed(tc.x, probeY); got != tc.red {
			t.Fatalf("%s: pixel (%d,%d) is %s, want %s", tc.name, tc.x, probeY, colorName(got), colorName(tc.red))
		}
	}
}

func colorName(red bool) string {
	if red {
		return "red"
	}
	return "blank"
}

// TestDefaultLimitsMatchesConversion guards the exported defaults: they are the
// values a zero Limits applies, and the conversion rejects a larger input.
func TestDefaultLimitsMatchesConversion(t *testing.T) {
	d := pdfgen.DefaultLimits()
	want := engine.DefaultLimits()
	if d.MaxInputBytes != want.MaxInputBytes || d.MaxStylesheetBytes != want.MaxStylesheetBytes ||
		d.MaxPages != want.MaxPages || d.MaxImagePixels != want.MaxImagePixels ||
		d.MaxImageBytes != want.MaxImageBytes || d.MaxRasterPixels != want.MaxRasterPixels ||
		d.MaxFontBytes != want.MaxFontBytes || d.MaxTotalImageBytes != want.MaxTotalImageBytes ||
		d.MaxTotalImagePixels != want.MaxTotalImagePixels || d.MaxTotalFontBytes != want.MaxTotalFontBytes ||
		d.MaxDOMDepth != want.MaxDOMDepth || d.MaxTableCells != want.MaxTableCells ||
		d.MaxOperations != want.MaxOperations {
		t.Fatalf("defaults = %+v, engine says %+v", d, want)
	}
	res, err := pdfgen.Convert(context.Background(), make([]byte, d.MaxInputBytes+1), pdfgen.Options{})
	if err == nil || len(res.Diagnostics) == 0 || res.Diagnostics[0].Code != "resource/limit" {
		t.Fatalf("an oversized input was not rejected: err=%v diags=%v", err, res.Diagnostics)
	}
}

// TestTotalAssetLimitReachesTheEngine guards the Limits mapping: a caller's
// aggregate font limit must reach the engine instead of being dropped in the
// pdfgen wrapper.
func TestTotalAssetLimitReachesTheEngine(t *testing.T) {
	res, err := pdfgen.Convert(context.Background(), []byte(`<p>x</p>`), pdfgen.Options{
		FallbackFont: filepath.Join("..", "testdata", "fonts", "DejaVuSans.ttf"),
		Limits:       pdfgen.Limits{MaxTotalFontBytes: 1},
	})
	if err == nil || len(res.Diagnostics) == 0 || res.Diagnostics[0].Code != "resource/limit" {
		t.Fatalf("the total font limit was not applied: err=%v diags=%v", err, res.Diagnostics)
	}
}

func findDiagnostic(list pdfgen.Diagnostics, code string) (pdfgen.Diagnostic, bool) {
	for _, d := range list {
		if d.Code == code {
			return d, true
		}
	}
	return pdfgen.Diagnostic{}, false
}

// TestDocumentDiagnosticsNameTheFile guards the diagnostic contract: a missing
// image, a resource limit and a linked stylesheet all identify the document or
// the stylesheet that caused them.
func TestDocumentDiagnosticsNameTheFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	res, err := pdfgen.Convert(ctx, []byte(`<img src="missing.png">`), pdfgen.Options{
		BaseDir: dir,
		File:    "invoice.html",
	})
	if err == nil {
		t.Fatal("a missing image was accepted")
	}
	missing, ok := findDiagnostic(res.Diagnostics, "img/load")
	if !ok || missing.File != "invoice.html" || missing.Line == 0 {
		t.Fatalf("img/load diagnostic = %+v", missing)
	}

	res, err = pdfgen.Convert(ctx, []byte(`<p>x</p>`), pdfgen.Options{
		File:   "invoice.html",
		Limits: pdfgen.Limits{MaxOperations: 1},
	})
	if err == nil {
		t.Fatal("the limit was not enforced")
	}
	limit, ok := findDiagnostic(res.Diagnostics, "resource/limit")
	if !ok || limit.File != "invoice.html" {
		t.Fatalf("resource/limit diagnostic = %+v", limit)
	}

	styleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(styleDir, "bad.css"), []byte("p { display: grid }"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = pdfgen.Convert(ctx, []byte(`<link rel="stylesheet" href="bad.css"><p>x</p>`), pdfgen.Options{
		BaseDir: styleDir,
		File:    "invoice.html",
	})
	if err == nil {
		t.Fatal("the bad stylesheet was accepted")
	}
	rule, ok := findDiagnostic(res.Diagnostics, "css/unsupported-value")
	if !ok || !strings.HasSuffix(rule.File, "bad.css") || rule.Line == 0 {
		t.Fatalf("stylesheet diagnostic = %+v", rule)
	}
}

// TestOptionAndDocumentErrorsTogether guards early option validation: a bad
// option and a bad inline declaration are both reported.
func TestOptionAndDocumentErrorsTogether(t *testing.T) {
	res, err := pdfgen.Convert(context.Background(), []byte(`<p style="display: grid">x</p>`), pdfgen.Options{
		File:     "doc.html",
		PageSize: "banana",
	})
	if err == nil {
		t.Fatal("the invalid page size was accepted")
	}
	if _, ok := findDiagnostic(res.Diagnostics, "options/page-size"); !ok {
		t.Fatalf("options/page-size is missing: %v", res.Diagnostics)
	}
	if _, ok := findDiagnostic(res.Diagnostics, "css/unsupported-value"); !ok {
		t.Fatalf("the CSS error is missing: %v", res.Diagnostics)
	}
}

// TestAnchorWarningsAreTargeted guards the anchor contract: href="#" and an
// unused href="#missing" warn nothing (and Strict accepts), while a real
// target-counter warning carries the anchor position.
func TestAnchorWarningsAreTargeted(t *testing.T) {
	ctx := context.Background()
	for _, src := range []string{
		`<p><a href="#">top</a></p>`,
		`<p><a href="#missing">top</a></p>`,
	} {
		res, err := pdfgen.Convert(ctx, []byte(src), pdfgen.Options{File: "links.html", Strict: true, FallbackFont: filepath.Join("..", "testdata", "fonts", "DejaVuSans.ttf")})
		if err != nil {
			t.Fatalf("%s: %v\n%v", src, err, res.Diagnostics)
		}
	}

	src := []byte(`<style>a.toc::after { content: target-counter(attr(href), page) }</style><p><a class="toc" href="#missing">x</a></p>`)
	res, err := pdfgen.Convert(ctx, []byte(src), pdfgen.Options{File: "toc.html", FallbackFont: filepath.Join("..", "testdata", "fonts", "DejaVuSans.ttf")})
	if err != nil {
		t.Fatal(err)
	}
	warn, ok := findDiagnostic(res.Diagnostics, "pagination/unknown-target")
	if !ok || warn.File != "toc.html" || warn.Line == 0 {
		t.Fatalf("unknown-target diagnostic = %+v", warn)
	}
}

// TestErrorListContainsOnlyErrors guards the split lists: the error value holds
// only the error entry while Result.Diagnostics keeps the warning.
func TestErrorListContainsOnlyErrors(t *testing.T) {
	res, err := pdfgen.Convert(context.Background(), []byte(`<video src="x"></video><script>1</script>`), pdfgen.Options{File: "mixed.html"})
	if err == nil {
		t.Fatal("the unsupported element was accepted")
	}
	var verr *pdfgen.Error
	if !errors.As(err, &verr) || len(verr.Diagnostics) != 1 || verr.Diagnostics[0].Code != "html/unsupported-element" {
		t.Fatalf("Error.Diagnostics = %+v", verr)
	}
	if len(res.Diagnostics) < 2 {
		t.Fatalf("Result.Diagnostics = %v, want both entries", res.Diagnostics)
	}
}

// TestAttributesAreValidatedAndHiddenHides guards the attribute contract.
func TestAttributesAreValidatedAndHiddenHides(t *testing.T) {
	ctx := context.Background()
	fontPath := filepath.Join("..", "testdata", "fonts", "DejaVuSans.ttf")
	res, err := pdfgen.Convert(ctx, []byte(`<div data-x="1" onclick="x">x</div>`), pdfgen.Options{File: "attrs.html", FallbackFont: fontPath})
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	for _, d := range res.Diagnostics {
		if d.Code == "html/unsupported-attribute" {
			seen++
		}
	}
	if seen != 2 {
		t.Fatalf("attribute diagnostics = %v", res.Diagnostics)
	}

	ok, err := pdfgen.Convert(ctx, []byte(`<table><tr><td colspan="2">x</td></tr></table><p><a href="#id">y</a></p>`), pdfgen.Options{File: "ok.html", FallbackFont: fontPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ok.Diagnostics {
		if d.Code == "html/unsupported-attribute" {
			t.Fatalf("a documented attribute was reported: %s", d)
		}
	}
}

// TestHiddenAttributeHidesContent guards the HTML hidden attribute.
func TestHiddenAttributeHidesContent(t *testing.T) {
	res, err := pdfgen.Convert(context.Background(), []byte(`<div hidden><p>SECRET</p></div><p>VISIBLE</p>`), pdfgen.Options{File: "hidden.html", FallbackFont: filepath.Join("..", "testdata", "fonts", "DejaVuSans.ttf")})
	if err != nil {
		t.Fatal(err)
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "SECRET") || !strings.Contains(text, "VISIBLE") {
		t.Fatalf("extracted text = %q", text)
	}
}
