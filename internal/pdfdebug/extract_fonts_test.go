package pdfdebug_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/pdf"
	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

func TestTextMapsAreScopedToTheActiveFont(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Skipf("no bundled font: %v", err)
	}
	first, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	gidA, ok := first.GlyphIndex('A')
	if !ok {
		t.Skip("no A")
	}
	gidB, ok := second.GlyphIndex('Б')
	if !ok {
		t.Skip("no Б")
	}
	d := pdf.New(pdf.Options{})
	d.AddPage(595276, 841890, []pdf.Op{{Kind: pdf.TextOpKind, Text: &pdf.Text{
		X: 1000, Y: 2000, Font: first, Size: 12000,
		Glyphs: []pdf.Glyph{{GID: gidA, Rune: 'A'}},
	}}})
	d.AddPage(595276, 841890, []pdf.Op{{Kind: pdf.TextOpKind, Text: &pdf.Text{
		X: 1000, Y: 2000, Font: second, Size: 12000,
		Glyphs: []pdf.Glyph{{GID: gidB, Rune: 'Б'}},
	}}})
	out, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	pages, err := pdfdebug.Pages(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0] != "A" || pages[1] != "Б" {
		t.Fatalf("pages = %q, want [A Б]", pages)
	}
}
