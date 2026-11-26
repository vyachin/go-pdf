package charset

import "github.com/vyachin/go-pdf/internal/budget"

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
// column of the source. An offset inside a decoded character reports the column
// where that character starts.
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
		m.cached, m.line, m.decCol, m.srcCol = true, i, 0, 0
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
	return i + 1, srcCol + 1
}

// widthAt returns the width in the decoded text of one source byte.
func (m *Mapper) widthAt(i int) int {
	if m.widths == nil {
		return 1
	}
	return int(m.widths[i])
}
