package pdfgen_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/vyachin/go-pdf/pdfgen"
)

// Example shows the shape of a conversion: read a template, render it, write
// the PDF. The paths are relative to the caller's working directory.
func Example() {
	src, err := os.ReadFile("invoice.html")
	if err != nil {
		log.Fatal(err)
	}

	opts := pdfgen.Options{
		File:     "invoice.html", // appears in diagnostics
		PageSize: "A4",           // fallback when @page has no size
		Margin:   "20mm",
		BaseDir:  "assets", // relative images stay inside this directory
		FontDir:  "fonts",  // relative @font-face paths
	}

	res, err := pdfgen.Convert(context.Background(), src, opts)
	var verr *pdfgen.Error
	if errors.As(err, &verr) {
		for _, d := range verr.Diagnostics {
			log.Print(d) // invoice.html:7:12: error[css/unsupported-value]: ...
		}
		return
	}
	if err != nil {
		log.Fatal(err) // cancelled context or an unexpected failure
	}

	// Warnings: the PDF exists, but the caller may want to log them.
	for _, d := range res.Diagnostics {
		log.Print(d)
	}

	if err := os.WriteFile("invoice.pdf", res.PDF, 0o644); err != nil {
		log.Fatal(err)
	}
}

// ExampleValidate shows the same diagnostics as Convert without writing a PDF,
// which is what a CI check needs.
func ExampleValidate() {
	src, err := os.ReadFile("invoice.html")
	if err != nil {
		log.Fatal(err)
	}

	diags, err := pdfgen.Validate(context.Background(), src, pdfgen.Options{File: "invoice.html"})
	if err != nil {
		var verr *pdfgen.Error
		if errors.As(err, &verr) {
			for _, d := range verr.Diagnostics {
				log.Print(d)
			}
			return
		}
		log.Fatal(err)
	}
	if diags.HasErrors() {
		log.Fatal("the template is invalid")
	}
}

// ExampleError shows the shape of a rejected document: the error carries every
// diagnostic, each with a stable code, a position and the element's path.
func ExampleError() {
	_, err := pdfgen.Convert(context.Background(),
		[]byte(`<style>p{color:notacolour}</style><p>x</p>`),
		pdfgen.Options{File: "invoice.html"})

	var verr *pdfgen.Error
	if errors.As(err, &verr) {
		for _, d := range verr.Diagnostics {
			fmt.Println(d.Severity, d.Code)
		}
	}
	// Output: error css/unsupported-value
}
