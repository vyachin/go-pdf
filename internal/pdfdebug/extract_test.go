package pdfdebug

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyachin/go-pdf/internal/font"
	"github.com/vyachin/go-pdf/internal/pdf"
)

func TestDecodeUTF16Hex(t *testing.T) {
	cases := []struct {
		in   string
		want []rune
	}{
		{"0041", []rune{'A'}},
		{"0416", []rune{'Ж'}},
		// A surrogate pair is one code point, not two halves of one.
		{"D83DDE00", []rune{'\U0001F600'}},
		{"DBFFDFFF", []rune{'\U0010FFFF'}},
		{"0041D83DDE00", []rune{'A', '\U0001F600'}},
	}
	for _, tc := range cases {
		got := decodeUTF16Hex(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("decodeUTF16Hex(%q) = %U, want %U", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("decodeUTF16Hex(%q) = %U, want %U", tc.in, got, tc.want)
			}
		}
	}
}

// TestNonBMPTextExtractsAsOneRune is the regression test for the surrogate bug:
// the extracted text of a character outside the BMP must be that character, not
// two replacement characters.
func TestNonBMPTextExtractsAsOneRune(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fonts", "DejaVuSans.ttf"))
	if err != nil {
		t.Skipf("no bundled font: %v", err)
	}
	f, err := font.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	const emoji = '\U0001F600'
	gid, ok := f.GlyphIndex(emoji)
	if !ok {
		t.Skip("the bundled font has no non-BMP glyph")
	}
	d := pdf.New(pdf.Options{})
	d.AddPage(595276, 841890, []pdf.Op{{Kind: pdf.TextOpKind, Text: &pdf.Text{
		X: 1000, Y: 2000, Font: f, Size: 12000,
		Glyphs: []pdf.Glyph{{GID: gid, Rune: emoji}},
	}}})
	out, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	pages, err := Pages(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0] != string(emoji) {
		t.Fatalf("pages = %q, want one character %q", pages, string(emoji))
	}
}
