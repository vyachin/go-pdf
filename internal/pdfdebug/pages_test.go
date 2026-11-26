package pdfdebug_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
	"github.com/vyachin/go-pdf/pdfgen"
)

// onePixelPNG is a 1x1 red pixel. A page that only draws it has no text at all,
// which is the case Pages was blind to.
const onePixelPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC"

// bundledFont is the repository's test font. Passing it explicitly keeps the
// page model independent of the developer machine: the package reads no
// installed host font on its own.
const bundledFont = "../../testdata/fonts/DejaVuSans.ttf"

func convertHTML(t *testing.T, src string) []byte {
	t.Helper()
	res, err := pdfgen.Convert(context.Background(), []byte(src), pdfgen.Options{
		FallbackFont: bundledFont,
	})
	if err != nil {
		t.Fatalf("converting %q: %v", src, err)
	}
	return res.PDF
}

func trimmed(texts []string) []string {
	out := make([]string, len(texts))
	for i, s := range texts {
		out[i] = strings.TrimSpace(s)
	}
	return out
}

func TestPageTextsKeepsPageOrder(t *testing.T) {
	pdf := convertHTML(t, `<p>one</p><p style="break-before: page">two</p><p style="break-before: page">three</p>`)

	texts, err := pdfdebug.PageTexts(pdf)
	if err != nil {
		t.Fatalf("PageTexts: %v", err)
	}
	want := []string{"one", "two", "three"}
	got := trimmed(texts)
	if len(got) != len(want) {
		t.Fatalf("PageTexts = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("page %d = %q, want %q (all: %q)", i+1, got[i], want[i], got)
		}
	}

	count, err := pdfdebug.PageCount(pdf)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if count != len(want) {
		t.Fatalf("PageCount = %d, want %d", count, len(want))
	}
}

func TestPageTextsCountsAPageThatDrawsOnlyAnImage(t *testing.T) {
	pdf := convertHTML(t, `<p>cover</p><div style="break-before: page"><img src="`+onePixelPNG+`" style="width: 10pt; height: 10pt"></div>`)

	texts, err := pdfdebug.PageTexts(pdf)
	if err != nil {
		t.Fatalf("PageTexts: %v", err)
	}
	got := trimmed(texts)
	if len(got) != 2 || got[0] != "cover" || got[1] != "" {
		t.Fatalf("PageTexts = %q, want [cover \"\"]", got)
	}

	count, err := pdfdebug.PageCount(pdf)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("PageCount = %d, want 2", count)
	}

	// Pages follows the page tree, so the image-only page is an empty text
	// page rather than invisible.
	pages, err := pdfdebug.Pages(pdf)
	if err != nil {
		t.Fatalf("Pages: %v", err)
	}
	if len(pages) != 2 || pages[1] != "" {
		t.Fatalf("Pages = %d pages %q, want 2 with an empty image-only page", len(pages), pages)
	}
}

func TestPageTextsCountsAnEmptyPage(t *testing.T) {
	// A forced break no longer leaves a page with nothing on it, so the
	// text-free page is made by a background-only block: it draws no text, but
	// it is a real page that PageTexts has to count.
	pdf := convertHTML(t, `<div style="height: 20pt; background-color: #eeeeee"></div><p style="break-before: page">after</p>`)

	texts, err := pdfdebug.PageTexts(pdf)
	if err != nil {
		t.Fatalf("PageTexts: %v", err)
	}
	got := trimmed(texts)
	if len(got) != 2 || got[0] != "" || got[1] != "after" {
		t.Fatalf("PageTexts = %q, want [\"\" after]", got)
	}

	count, err := pdfdebug.PageCount(pdf)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("PageCount = %d, want 2", count)
	}
}
