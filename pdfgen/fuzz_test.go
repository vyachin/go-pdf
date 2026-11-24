package pdfgen_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyachin/go-pdf/pdfgen"
)

// FuzzConvert pins the panic-containment guarantee at the public boundary:
// arbitrary bytes end in a PDF or an error, never in a crash. The limits keep
// one iteration cheap enough for a short CI run.
func FuzzConvert(f *testing.F) {
	font, err := filepath.Abs(filepath.Join("..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		f.Fatal(err)
	}
	if _, err := os.Stat(font); err != nil {
		f.Skip("the bundled font is missing")
	}

	seeds := [][]byte{
		[]byte(invoiceHTML),
		[]byte(`<p>простой текст</p>`),
		[]byte(`<style>p{display:flex}</style><p>x</p>`),
		[]byte(`<html><body><table><tr><td>`),
		{0x00, 0x01, 0x02},
	}
	if corpus, err := os.ReadFile(filepath.Join("..", "testdata", "act.html")); err == nil {
		seeds = append(seeds, corpus)
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		opts := pdfgen.Options{
			File:         "fuzz.html",
			FallbackFont: font,
			Limits: pdfgen.Limits{
				MaxInputBytes: 64 << 10,
				MaxPages:      20,
				MaxOperations: 2_000_000,
			},
		}
		res, err := pdfgen.Convert(context.Background(), src, opts)
		if err != nil {
			return
		}
		if len(res.PDF) == 0 {
			t.Fatal("Convert returned neither an error nor a PDF")
		}
		if !bytes.HasPrefix(res.PDF, []byte("%PDF-")) {
			t.Fatalf("the result is not a PDF: %q", res.PDF[:min(16, len(res.PDF))])
		}
	})
}
