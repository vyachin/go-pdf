package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// TestCombiningMarkKeepsExtractedSequence guards the independent extraction of
// shaped text with a mark: body text and a margin box both keep the combining
// sequence, so a copy from the PDF carries the original characters.
func TestCombiningMarkKeepsExtractedSequence(t *testing.T) {
	const word = "сло\u0301во"
	src := []byte(`<style>@page { @bottom-center { content: "` + word + `" } } p { font-size: 10pt }</style><p>` + word + `</p>`)
	res, err := Convert(context.Background(), src, Options{File: "marks.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfdebug.Pages(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(pages, "\n")
	if got := strings.Count(text, word); got != 2 {
		t.Fatalf("the extracted text contains the combining sequence %d times, want 2: %q", got, text)
	}
	validateWithPDFCPU(t, res.PDF)
}
