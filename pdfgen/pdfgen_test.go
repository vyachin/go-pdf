package pdfgen_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
