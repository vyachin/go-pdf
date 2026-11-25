package charset

import "slices"

// Mapper translates an offset in a decoded text back to the line and column of
// the source bytes it came from. A single-byte encoding changes neither the
// line structure nor the order of the bytes, so the line number is the decoded
// one; only the column has to be recomputed, by walking the source line and
// adding up the decoded widths until the decoded column is reached.
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

	srcLines []int
	decLines []int

	cached bool
	line   int
	decCol int
	srcCol int
}

// NewMapper builds the mapper of a decoded source. widths is the Decoded
// widths, and skip is the number of bytes a stripped byte order mark covered.
func NewMapper(src, text []byte, widths []uint8, skip int) *Mapper {
	m := &Mapper{src: src, text: text, widths: widths, skip: skip}
	m.srcLines = lineStarts(src)
	m.srcLines[0] = skip // the mark is not part of the document
	m.decLines = lineStarts(text)
	return m
}

// Position converts an offset in the decoded text into a 1-based line and
// column of the source. An offset inside a decoded character reports the column
// where that character starts.
func (m *Mapper) Position(offset int) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(m.text) {
		offset = len(m.text)
	}
	i := lineIndex(m.decLines, offset)
	decCol := offset - m.decLines[i]
	if !m.cached || m.line != i || decCol < m.decCol {
		m.cached, m.line, m.decCol, m.srcCol = true, i, 0, 0
	}
	srcBase := m.srcLines[i]
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
	return i + 1, srcCol + 1
}

// widthAt returns the width in the decoded text of one source byte.
func (m *Mapper) widthAt(i int) int {
	if m.widths == nil {
		return 1
	}
	return int(m.widths[i])
}

// lineIndex returns the index of the line that contains offset.
func lineIndex(lines []int, offset int) int {
	i, _ := slices.BinarySearch(lines, offset+1)
	i--
	if i < 0 {
		return 0
	}
	return i
}
