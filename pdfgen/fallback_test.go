package pdfgen_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/vyachin/go-pdf/pdfgen"
)

// hostFallbackFont returns the first conventional system font present on this
// machine, or the empty string when the host has none of them.
func hostFallbackFont() string {
	for _, path := range pdfgen.SystemFallbackFonts() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// TestFallbackFontsConvertDeterministically guards the ordered list: the first
// entry that exists is used, a missing entry is skipped, and the same input
// produces the same bytes as the single FallbackFont option and on every run.
func TestFallbackFontsConvertDeterministically(t *testing.T) {
	font := testFontPath(t)
	doc := []byte(`<p>Deterministic fallback</p>`)
	opts := pdfgen.Options{
		File: "fallback.html",
		FallbackFonts: []string{
			filepath.Join("..", "testdata", "fonts", "missing.ttf"),
			font,
		},
	}
	first, err := pdfgen.Convert(context.Background(), doc, opts)
	if err != nil {
		t.Fatalf("Convert: %v\n%s", err, strings.Join(first.Diagnostics.Lines(), "\n"))
	}
	second, err := pdfgen.Convert(context.Background(), doc, opts)
	if err != nil {
		t.Fatalf("Convert again: %v", err)
	}
	if !bytes.Equal(first.PDF, second.PDF) {
		t.Fatal("two runs of the same FallbackFonts input produced different PDFs")
	}
	single, err := pdfgen.Convert(context.Background(), doc, pdfgen.Options{
		File:         "fallback.html",
		FallbackFont: font,
	})
	if err != nil {
		t.Fatalf("Convert with FallbackFont: %v", err)
	}
	if !bytes.Equal(first.PDF, single.PDF) {
		t.Fatal("one explicit path and a one-entry FallbackFonts list produced different PDFs")
	}
}

// TestFallbackFontsComeFromSource guards the source-aware behaviour: a
// fallback path read through Options.Source is a path inside that source, and
// the host filesystem is not consulted.
func TestFallbackFontsComeFromSource(t *testing.T) {
	font := embeddedAsset(t, "pacifico.ttf")
	source := fstest.MapFS{"fonts/body.ttf": {Data: font}}
	res, err := pdfgen.Convert(context.Background(), []byte(`<p>Hello</p>`), pdfgen.Options{
		File:          "fallback.html",
		Source:        source,
		FallbackFonts: []string{"fonts/body.ttf"},
	})
	if err != nil {
		t.Fatalf("the source fallback list was not used: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("no PDF was produced")
	}

	// A host path cannot name a fallback while a source is set.
	hostPath, err := filepath.Abs(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	res, err = pdfgen.Convert(context.Background(), []byte(`<p>Hello</p>`), pdfgen.Options{
		File:          "fallback.html",
		Source:        source,
		FallbackFonts: []string{hostPath},
	})
	if err == nil || diagnosticWithCode(res.Diagnostics, "font/unavailable") == nil {
		t.Fatalf("a host fallback font was read from the source conversion: %v", err)
	}
}

// TestSystemFallbackFontsIsACopy guards RV5-53's mutation scenario: every call
// returns a fresh slice, so a caller that overwrites or truncates it cannot
// change another call or a concurrent conversion.
func TestSystemFallbackFontsIsACopy(t *testing.T) {
	want := pdfgen.SystemFallbackFonts()
	if len(want) == 0 {
		t.Fatal("SystemFallbackFonts returned no conventional paths")
	}

	got := pdfgen.SystemFallbackFonts()
	for i := range got {
		got[i] = "/nonexistent/font.ttf"
	}
	_ = got[:0]
	if again := pdfgen.SystemFallbackFonts(); !slices.Equal(again, want) {
		t.Fatalf("mutating the returned slice changed the package's list: %q, want %q", again, want)
	}

	// Concurrent callers mutate the copy they received while other conversions
	// read the copy they received. Run with -race: a shared slice would be
	// reported here as a data race.
	ctx := context.Background()
	doc := []byte(`<p>concurrent fallback</p>`)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list := pdfgen.SystemFallbackFonts()
			if len(list) > 0 {
				list[0] = "/nonexistent/concurrent.ttf"
			}
			_, _ = pdfgen.Convert(ctx, doc, pdfgen.Options{File: "concurrent.html", FallbackFonts: list})
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list := pdfgen.SystemFallbackFonts()
			if len(list) > 0 {
				list[0] = "/nonexistent/truncated.ttf"
			}
			list = list[:0]
			_, _ = pdfgen.Convert(ctx, doc, pdfgen.Options{File: "concurrent.html", FallbackFonts: list})
		}()
	}
	wg.Wait()

	// The same conversion, run twice with a fresh list, selects the same font
	// bytes; only a host with a conventional font can render that way.
	if hostFallbackFont() == "" {
		t.Log("no conventional host font; the opt-in comparison is skipped")
		return
	}
	listA := pdfgen.SystemFallbackFonts()
	first, err := pdfgen.Convert(ctx, doc, pdfgen.Options{File: "host.html", FallbackFonts: listA})
	if err != nil {
		t.Fatalf("the opt-in list did not render: %v\n%s", err, strings.Join(first.Diagnostics.Lines(), "\n"))
	}
	_ = listA[:0]
	second, err := pdfgen.Convert(ctx, doc, pdfgen.Options{File: "host.html", FallbackFonts: pdfgen.SystemFallbackFonts()})
	if err != nil {
		t.Fatalf("Convert again: %v", err)
	}
	if !bytes.Equal(first.PDF, second.PDF) {
		t.Fatal("the same host list selected different font bytes on two calls")
	}
}

// TestWithoutFontFailsHostIndependently guards the default path of RV5-53: a
// document with no @font-face and no fallback fails with one font/unavailable
// diagnostic, whether or not the host has a conventional font, and the two
// runs agree. Only the explicit SystemFallbackFonts opt-in renders.
func TestWithoutFontFailsHostIndependently(t *testing.T) {
	ctx := context.Background()
	doc := []byte(`<p>no font declared</p>`)
	opts := pdfgen.Options{File: "no-font.html"}

	first, err := pdfgen.Convert(ctx, doc, opts)
	if err == nil {
		t.Fatal("a document without a font converted")
	}
	if len(first.PDF) != 0 {
		t.Fatalf("the failed conversion produced %d PDF bytes", len(first.PDF))
	}
	second, err := pdfgen.Convert(ctx, doc, opts)
	if err == nil {
		t.Fatal("the second conversion succeeded")
	}
	if got, want := strings.Join(second.Diagnostics.Lines(), "\n"), strings.Join(first.Diagnostics.Lines(), "\n"); got != want {
		t.Fatalf("the two runs disagree:\n got: %s\nwant: %s", got, want)
	}
	var count int
	for _, d := range first.Diagnostics {
		if d.Code == "font/unavailable" {
			count++
			if !strings.Contains(d.Message, "Options.FallbackFont") || !strings.Contains(d.Message, "Options.FallbackFonts") {
				t.Fatalf("the diagnostic does not name both options: %s", d.Message)
			}
		}
	}
	if count != 1 {
		t.Fatalf("%d font/unavailable diagnostics, want one:\n%s", count, strings.Join(first.Diagnostics.Lines(), "\n"))
	}

	// Validate shares the preparation and reaches the same conclusion.
	diags, err := pdfgen.Validate(ctx, doc, opts)
	if err == nil || diagnosticWithCode(diags, "font/unavailable") == nil {
		t.Fatalf("Validate: err=%v diags=%v", err, diags.Lines())
	}

	// The default reads no installed font even where one exists.
	if hostFallbackFont() != "" {
		res, err := pdfgen.Convert(ctx, doc, pdfgen.Options{
			File:          "opt-in.html",
			FallbackFonts: pdfgen.SystemFallbackFonts(),
		})
		if err != nil {
			t.Fatalf("the explicit opt-in did not render: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
		if len(res.PDF) == 0 {
			t.Fatal("the explicit opt-in produced no PDF")
		}
	}
}
