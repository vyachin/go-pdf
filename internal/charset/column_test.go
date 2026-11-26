package charset

import "testing"

// TestColumn pins the character-column rule: the column counts Unicode code
// points from the line start, and a leading UTF-8 byte order mark is not a
// character, so adding or removing it never shifts a column.
func TestColumn(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	cases := []struct {
		name      string
		src       []byte
		lineStart int
		offset    int
		want      int
	}{
		// ASCII: one byte is one character.
		{name: "ascii start", src: []byte("abc"), offset: 0, want: 1},
		{name: "ascii middle", src: []byte("abc"), offset: 1, want: 2},
		{name: "ascii end", src: []byte("abc"), offset: 3, want: 4},
		// Cyrillic: two bytes are one character.
		{name: "cyrillic start", src: []byte("Счёт"), offset: 0, want: 1},
		{name: "cyrillic second", src: []byte("Счёт"), offset: 2, want: 2},
		{name: "cyrillic third", src: []byte("Счёт"), offset: 4, want: 3},
		{name: "cyrillic end", src: []byte("Счёт"), offset: 8, want: 5},
		// A mark before the text does not count, so the same offsets report
		// the same columns as the text without it.
		{name: "bom start", src: append(append([]byte(nil), bom...), []byte("Счёт")...), offset: 0, want: 1},
		{name: "bom first byte", src: append(append([]byte(nil), bom...), []byte("Счёт")...), offset: 1, want: 1},
		{name: "bom second byte", src: append(append([]byte(nil), bom...), []byte("Счёт")...), offset: 2, want: 1},
		{name: "bom after mark", src: append(append([]byte(nil), bom...), []byte("Счёт")...), offset: 3, want: 1},
		{name: "bom second character", src: append(append([]byte(nil), bom...), []byte("Счёт")...), offset: 5, want: 2},
		{name: "bom end", src: append(append([]byte(nil), bom...), []byte("Счёт")...), offset: 11, want: 5},
		// A line start after the first line ignores the mark entirely.
		{name: "second line", src: []byte("a\nСчёт"), lineStart: 2, offset: 4, want: 2},
		// An offset before the line start, or past the source, is clamped.
		{name: "before line start", src: []byte("abc"), lineStart: 2, offset: 1, want: 1},
		{name: "past end", src: []byte("abc"), offset: 100, want: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Column(tc.src, tc.lineStart, tc.offset); got != tc.want {
				t.Fatalf("Column(%q, %d, %d) = %d, want %d", tc.src, tc.lineStart, tc.offset, got, tc.want)
			}
		})
	}

	// The literal promise of the mark rule: every offset of the text reports
	// the same column with and without the mark.
	plain := []byte("Счёт")
	marked := append(append([]byte(nil), bom...), plain...)
	for offset := 0; offset <= len(plain); offset++ {
		want := Column(plain, 0, offset)
		if got := Column(marked, 0, offset+len(bom)); got != want {
			t.Fatalf("column at %q offset %d = %d with the mark and %d without", plain, offset, got, want)
		}
	}
}
