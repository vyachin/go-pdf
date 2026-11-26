package charset

import (
	"unicode/utf8"

	"github.com/vyachin/go-pdf/internal/budget"
)

// Mapper translates an offset in a decoded text back to the line and column of
// the source bytes it came from. The line number is the decoded one: a
// single-byte encoding changes neither the line structure nor the order of the
// bytes, and decoding UTF-8 only removes a leading byte order mark. The column
// counts characters, not bytes: a single-byte source walks the decoded widths
// of the source line, while a UTF-8 source counts code points in the decoded
// text.
//
// The walk resumes from a cursor kept for the last line answered. Diagnostics
// are ordered by line and column, so a document that reports many positions on
// one long line costs one pass over that line rather than one pass per
// position.
//
// A Mapper keeps that cursor, so one belongs to one conversion and is not used
// concurrently. Conversions on different documents share nothing but the
// immutable encoding tables.
type Mapper struct {
	src    []byte
	text   []byte
	widths []uint8 // nil means one decoded byte per source byte
	skip   int

	// srcLines and decLines are bounded line indexes of the two coordinate
	// spaces. A legacy document can be made of newlines, so the indexes must
	// not grow with one entry per input line; a lookup past the stored part
	// scans and charges the bytes.
	srcLines *budget.Lines
	decLines *budget.Lines

	cached bool
	line   int
	decCol int
	srcCol int
	// col is the character column of decCol for a UTF-8 source. It is unused
	// for a single-byte source, where one source byte is one character and
	// srcCol already is the column.
	col int
}

// NewMapper builds the mapper of a decoded source. widths is the Decoded
// widths, and skip is the number of bytes a stripped byte order mark covered.
// It builds the line indexes without a work limit; NewMapperWithGuard charges
// them to the conversion.
func NewMapper(src, text []byte, widths []uint8, skip int) *Mapper {
	return NewMapperWithGuard(src, text, widths, skip, nil)
}

// NewMapperWithGuard is NewMapper with the conversion's work guard. The
// bounded line indexes charge one unit per stored line start and the bytes a
// position past their cap scans.
func NewMapperWithGuard(src, text []byte, widths []uint8, skip int, guard *budget.Guard) *Mapper {
	m := &Mapper{src: src, text: text, widths: widths, skip: skip}
	m.srcLines, _ = budget.NewLines(src, guard)
	m.decLines, _ = budget.NewLines(text, guard)
	return m
}

// Position converts an offset in the decoded text into a 1-based line and
// column of the source. The column counts characters, not bytes. An offset
// inside a decoded character reports the column where that character starts.
func (m *Mapper) Position(offset int) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(m.text) {
		offset = len(m.text)
	}
	i, decStart := m.decLines.Resolve(offset)
	decCol := offset - decStart
	if !m.cached || m.line != i || decCol < m.decCol {
		m.cached, m.line, m.decCol, m.srcCol, m.col = true, i, 0, 0, 0
	}
	if m.widths == nil {
		// A UTF-8 source needs no byte translation: the decoded text is the
		// source minus a leading byte order mark, and one code point is one
		// source character. Count from the cursor, and stop at the character
		// boundary so an offset inside a character names its start.
		end := offset
		for end > decStart+m.decCol && end < len(m.text) && !utf8.RuneStart(m.text[end]) {
			end--
		}
		if end > decStart+m.decCol {
			m.col += Column(m.text, decStart+m.decCol, end) - 1
			m.decCol = end - decStart
		}
		return i + 1, m.col + 1
	}
	srcBase, ok := m.srcLines.StartAt(i)
	if !ok {
		srcBase = len(m.src)
	}
	if i == 0 {
		// The mark is not part of the document, so its first line starts after
		// it even though the source line index stores zero.
		srcBase = m.skip
	}
	srcCol := m.srcCol
	j := m.decCol
	for j < decCol {
		s := srcBase + srcCol
		if s >= len(m.src) {
			break
		}
		w := m.widthAt(s)
		if w == 0 {
			srcCol++
			continue
		}
		if j+w > decCol {
			// The offset falls inside this character: report where it starts.
			return i + 1, srcCol + 1
		}
		j += w
		srcCol++
	}
	m.decCol, m.srcCol = j, srcCol
	// A single-byte encoding decodes one character per source byte, so the
	// byte column is already the character column and the existing numbers do
	// not move.
	return i + 1, srcCol + 1
}

// widthAt returns the width in the decoded text of one source byte.
func (m *Mapper) widthAt(i int) int {
	if m.widths == nil {
		return 1
	}
	return int(m.widths[i])
}
