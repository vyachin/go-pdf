package charset_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/html"
)

// TestPositionPathsCountCharacters checks the four columns that a diagnostic
// can derive from a UTF-8 source with Cyrillic text: the document, the mapper,
// a linked stylesheet and an encoding error all count the same characters. The
// document carries a byte order mark so the document and the mapper take their
// decoded paths.
func TestPositionPathsCountCharacters(t *testing.T) {
	const body = "x\n<p>Счёт"
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte(body)...)
	enc, err := charset.Validate(charset.UTF8)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := charset.Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	offset := bytes.Index(dec.Text, []byte("Счёт"))
	if offset < 0 {
		t.Fatal("the fixture has no Cyrillic word")
	}
	// "Счёт" starts after "<p>" on the second line.
	const wantLine, wantCol = 2, 4

	m := charset.NewMapper(src, dec.Text, dec.Widths, dec.Skip)
	if line, col := m.Position(offset); line != wantLine || col != wantCol {
		t.Fatalf("mapper position = %d:%d, want %d:%d", line, col, wantLine, wantCol)
	}

	doc, err := html.Parse(src, "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	if pos := doc.Position(offset); pos.Line != wantLine || pos.Col != wantCol {
		t.Fatalf("document position = %d:%d, want %d:%d", pos.Line, pos.Col, wantLine, wantCol)
	}

	sheet := css.ParseSheet(css.SheetOptions{File: "styles.css"}, []byte(body))
	if pos := sheet.Position(offset); pos.Line != wantLine || pos.Col != wantCol {
		t.Fatalf("stylesheet position = %d:%d, want %d:%d", pos.Line, pos.Col, wantLine, wantCol)
	}

	// An encoding error after the word reports the character column after the
	// four Cyrillic characters, not their eight bytes.
	bad := append(append([]byte(nil), src...), 0xFF)
	_, err = charset.Decode(enc, bad)
	var encErr *charset.Error
	if !errors.As(err, &encErr) {
		t.Fatalf("decoding an invalid byte returned %v", err)
	}
	if encErr.Line != wantLine || encErr.Col != wantCol+4 {
		t.Fatalf("encoding error position = %d:%d, want %d:%d", encErr.Line, encErr.Col, wantLine, wantCol+4)
	}
}
