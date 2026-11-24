package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// browserCertificateSource returns the committed browser certificate. It is
// the acceptance document for engine.Options.ImportBrowser: the file is not
// edited for the conversion.
func browserCertificateSource(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(repoRoot(), "testdata", "browser", "certificate.html")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the browser fixture: %v", err)
	}
	return src
}

func TestBrowserCertificateImport(t *testing.T) {
	src := browserCertificateSource(t)
	opts := Options{
		ImportBrowser: true,
		File:          "certificate.html",
		FallbackFont:  testFontPath(t),
	}
	res, err := Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatalf("browser import failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if !bytes.HasPrefix(res.PDF, []byte("%PDF-")) {
		t.Fatal("the imported document did not produce a PDF")
	}
	if !bytes.Contains(res.PDF, []byte("/MediaBox [0 0 595.276 841.89]")) {
		t.Fatal("the imported certificate is not A4")
	}
	if n := bytes.Count(res.PDF, []byte("/Subtype /Image")); n < 3 {
		t.Fatalf("the imported certificate embeds %d images, want at least 3", n)
	}
	pages, err := pdfdebug.Pages(res.PDF)
	if err != nil {
		t.Fatalf("reading the produced pages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("the certificate produced %d pages, want 1", len(pages))
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatalf("extracting text: %v", err)
	}
	for _, want := range []string{"ПОДАРОЧНЫЙ", "СЕРТИФИКАТ", "9NZ22W6P", "Количество посещений"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the imported text does not contain %q:\n%s", want, text)
		}
	}
	codes := map[string]bool{}
	for _, d := range res.Diagnostics {
		codes[d.Code] = true
	}
	for _, want := range []string{"browser/flex", "browser/background", "browser/media"} {
		if !codes[want] {
			t.Fatalf("the import did not report %q: %s", want, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
	}
	if res.Diagnostics.HasErrors() {
		t.Fatalf("the import reported errors: %s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}

	// The same file without the compatibility layer must still be rejected by
	// the strict profile: the import is an opt-in lowering, not a relaxed
	// validator.
	plain, err := Convert(context.Background(), src, Options{
		File:         "certificate.html",
		FallbackFont: testFontPath(t),
	})
	if err == nil {
		t.Fatal("the strict profile accepted a browser document without ImportBrowser")
	}
	if !plain.Diagnostics.HasErrors() {
		t.Fatal("the strict rejection carries no error diagnostics")
	}

	again, err := Convert(context.Background(), src, opts)
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}
	if !bytes.Equal(res.PDF, again.PDF) {
		t.Fatal("two imports of the same browser document differ")
	}
}

func TestBrowserImportFailureKeepsProfileDiagnostics(t *testing.T) {
	// A flex row cannot be lowered. The import reports why, and the original
	// profile diagnostics must survive so the author sees the source problem.
	src := []byte(`<style>div { display: flex }</style><div>row</div>`)
	res, err := Convert(context.Background(), src, Options{
		File:          "row.html",
		ImportBrowser: true,
	})
	if err == nil {
		t.Fatal("a flex row must not be imported")
	}
	codes := map[string]bool{}
	for _, d := range res.Diagnostics {
		codes[d.Code] = true
	}
	for _, want := range []string{"browser/unsupported-flex", "css/unsupported-value"} {
		if !codes[want] {
			t.Fatalf("missing %q in the diagnostics:\n%s", want, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
	}
}

func TestBrowserImportIsDisabledInStrictMode(t *testing.T) {
	// The compatibility layer is a lowering, not a relaxed validator: strict
	// mode must keep the profile diagnostics even when the caller also asks
	// for the import.
	src := browserCertificateSource(t)
	res, err := Convert(context.Background(), src, Options{
		File:          "certificate.html",
		FallbackFont:  testFontPath(t),
		Strict:        true,
		ImportBrowser: true,
	})
	if err == nil {
		t.Fatal("strict mode imported a browser document")
	}
	if !res.Diagnostics.HasErrors() {
		t.Fatal("strict rejection carries no errors")
	}
	for _, d := range res.Diagnostics {
		if strings.HasPrefix(d.Code, "browser/") {
			t.Fatalf("strict mode reported an import diagnostic: %s", d)
		}
	}
}

func TestBrowserImportRejectsFlexGap(t *testing.T) {
	src := []byte(`<style>div { display: flex; flex-direction: column; gap: 10px }</style><div><p>x</p></div>`)
	res, err := Convert(context.Background(), src, Options{
		File:          "gap.html",
		ImportBrowser: true,
	})
	if err == nil {
		t.Fatal("a flex column with a gap must not be imported")
	}
	codes := map[string]bool{}
	for _, d := range res.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["browser/unsupported-flex"] {
		t.Fatalf("missing browser/unsupported-flex:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}
