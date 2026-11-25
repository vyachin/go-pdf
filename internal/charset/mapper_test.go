package charset

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// decodedOffsets returns the decoded offset of every rune of s, in order.
func decodedOffsets(s string) []int {
	out := make([]int, 0, len(s))
	for i := range s {
		out = append(out, i)
	}
	return out
}

func TestMapperPosition(t *testing.T) {
	// Source lines: ASCII, Cyrillic, an empty line, mixed text, a CRLF pair,
	// and a last line without a newline.
	text := "abc\nСчёт\n\na Счёт b\r\nконец"
	src := cp1251(t, text)
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec.Text) != text {
		t.Fatalf("decoded text = %q, want %q", dec.Text, text)
	}
	m := NewMapper(src, dec.Text, dec.Widths, dec.Skip)

	// Every decoded rune maps back to its own source character: one source
	// character per decoded rune, so the column is the rune index in its line
	// plus one, including the position of a newline.
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lineStart := 0
		for j := 0; j < i; j++ {
			lineStart += len(lines[j])
		}
		for k, off := range decodedOffsets(line) {
			gotLine, gotCol := m.Position(lineStart + off)
			if gotLine != i+1 || gotCol != k+1 {
				t.Fatalf("Position(%q at %d) = %d:%d, want %d:%d", line, off, gotLine, gotCol, i+1, k+1)
			}
		}
	}
}

func TestMapperPositionExamples(t *testing.T) {
	// "<p>Привет <b>" in windows-1251: the Cyrillic word is six source bytes
	// and twelve decoded bytes, so the <b> sits at decoded column 17 and
	// source column 11.
	src := cp1251(t, "<p>Привет <b>")
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMapper(src, dec.Text, dec.Widths, dec.Skip)
	bStart := strings.Index(string(dec.Text), "<b>")
	cases := []struct {
		offset int
		line   int
		col    int
	}{
		{0, 1, 1},
		{3, 1, 4},       // the < of the word's first Cyrillic letter
		{bStart, 1, 11}, // "<b>" starts at source column 11
		{bStart + 1, 1, 12},
		{len(dec.Text), 1, 14}, // end of the text
	}
	for _, tc := range cases {
		line, col := m.Position(tc.offset)
		if line != tc.line || col != tc.col {
			t.Fatalf("Position(%d) = %d:%d, want %d:%d", tc.offset, line, col, tc.line, tc.col)
		}
	}
	// An offset inside a two-byte decoded character names where it starts.
	line, col := m.Position(4)
	if line != 1 || col != 4 {
		t.Fatalf("Position inside a character = %d:%d, want 1:4", line, col)
	}
}

func TestMapperPositionWithByteOrderMark(t *testing.T) {
	body := "abc\nСчёт"
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte(body)...)
	enc, err := Validate(UTF8)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMapper(src, dec.Text, dec.Widths, dec.Skip)
	cases := []struct {
		offset int
		line   int
		col    int
	}{
		{0, 1, 1}, // the mark is invisible: the first character is column 1
		{3, 1, 4}, // after "abc"
		{4, 2, 1}, // first character of the second line
	}
	for _, tc := range cases {
		line, col := m.Position(tc.offset)
		if line != tc.line || col != tc.col {
			t.Fatalf("Position(%d) = %d:%d, want %d:%d", tc.offset, line, col, tc.line, tc.col)
		}
	}
}

func TestMapperPositionEmptyAndOutOfRange(t *testing.T) {
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMapper(nil, dec.Text, dec.Widths, dec.Skip)
	if line, col := m.Position(0); line != 1 || col != 1 {
		t.Fatalf("Position(0) of an empty document = %d:%d, want 1:1", line, col)
	}
	if line, col := m.Position(100); line != 1 || col != 1 {
		t.Fatalf("Position past the end = %d:%d, want 1:1", line, col)
	}
	if line, col := m.Position(-5); line != 1 || col != 1 {
		t.Fatalf("Position(-5) = %d:%d, want 1:1", line, col)
	}
}

// TestMapperPositionLongLineIsLinear guards the hostile case: a document that
// is one very long line and reports a position for every character. The cursor
// keeps the total work linear; a walk from the line start for every position
// would not finish.
func TestMapperPositionLongLineIsLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("long line")
	}
	const reps = 20_000
	text := strings.Repeat("Счёт ", reps)
	src := cp1251(t, text)
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMapper(src, dec.Text, dec.Widths, dec.Skip)

	start := time.Now()
	lastCol := 0
	for off := 0; off < len(dec.Text); off++ {
		line, col := m.Position(off)
		if line != 1 || col < lastCol {
			t.Fatalf("Position(%d) = %d:%d, previous column %d", off, line, col, lastCol)
		}
		lastCol = col
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("%d positions took %v; the walk is not resuming from its cursor", len(dec.Text), elapsed)
	}
}

// TestMapperReportsDecodedWidths checks a width the mapper depends on: a
// Cyrillic letter is two decoded bytes and one source byte.
func TestMapperReportsDecodedWidths(t *testing.T) {
	src := cp1251(t, "Счёт")
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(dec.Text) != 2*len(src) {
		t.Fatalf("decoded %d bytes from %d source bytes, want %d", len(dec.Text), len(src), 2*len(src))
	}
	if utf8.RuneCount(dec.Text) != len(src) {
		t.Fatalf("%d runes from %d source bytes", utf8.RuneCount(dec.Text), len(src))
	}
}
