package budget

import "sort"

// MaxLineStarts bounds the line starts one index stores. A source with more
// lines keeps the first MaxLineStarts entries and resolves an offset past them
// with a charged scan from the last stored start, so a stylesheet made of
// newlines cannot allocate one index entry per input newline.
const MaxLineStarts = 65_536

// Lines is a bounded index of the line starts of one byte slice. It stores at
// most MaxLineStarts starts, charges one unit per stored entry to the
// conversion's guard before the slice grows, and charges the bytes it scans
// when a lookup falls past the stored part.
//
// A Lines belongs to one conversion: it carries a guard and a source slice, so
// it is not safe for concurrent use and the source must not change after the
// index is built.
type Lines struct {
	src    []byte
	starts []int
	guard  *Guard
	// capped reports that the index is incomplete: the source has more lines
	// than MaxLineStarts, or the guard refused a charge before the build
	// finished. Resolve and StartAt then scan from the last stored start.
	capped bool
}

// NewLines indexes the line starts of src under guard. Every stored start is
// charged before it is appended. A source with more lines than MaxLineStarts
// stops storing entries and reports Capped; the index then answers a lookup
// past the stored part with a charged scan, so a caller that cannot lose a
// position still has one. The returned error is the guard's: a refused charge
// leaves the partial index in the same capped state.
func NewLines(src []byte, guard *Guard) (*Lines, error) {
	l := &Lines{src: src, guard: guard}
	if err := guard.Add(1); err != nil {
		l.capped = true
		return l, err
	}
	l.starts = append(l.starts, 0)
	for i, b := range src {
		if b != '\n' {
			continue
		}
		if len(l.starts) >= MaxLineStarts {
			// The rest of the source is not indexed; a lookup past the last
			// stored start scans it, charging the bytes it reads.
			l.capped = true
			return l, nil
		}
		if err := guard.Add(1); err != nil {
			l.capped = true
			return l, err
		}
		l.starts = append(l.starts, i+1)
	}
	return l, nil
}

// Resolve returns the 0-based line that contains offset and the byte offset
// where that line starts. An offset past the stored part of a capped index is
// found by scanning from the last stored start; the scanned bytes are charged
// to the guard before the scan. Resolve has no error to return, so a scan the
// guard refuses still completes: the guard keeps the reason for the caller
// that ends the conversion.
func (l *Lines) Resolve(offset int) (line, start int) {
	if l == nil || len(l.starts) == 0 {
		return 0, 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(l.src) {
		offset = len(l.src)
	}
	i := sort.SearchInts(l.starts, offset+1) - 1
	if i < 0 {
		i = 0
	}
	if !l.capped || i < len(l.starts)-1 {
		return i, l.starts[i]
	}
	// The index stopped before this offset: scan from the last stored start.
	line = len(l.starts) - 1
	start = l.starts[line]
	if offset > start {
		_ = l.guard.Add(uint64(offset - start))
		for j := start; j < offset; j++ {
			if l.src[j] == '\n' {
				line++
				start = j + 1
			}
		}
	}
	return line, start
}

// StartAt returns the byte offset where the 0-based line starts. A stored line
// is answered in constant time; a line past the stored part of a capped index
// is found by a scan from the last stored start, and the scanned bytes are
// charged. The second result is false only when the line does not exist in the
// source.
func (l *Lines) StartAt(line int) (int, bool) {
	if l == nil || line < 0 {
		return 0, false
	}
	if line < len(l.starts) {
		return l.starts[line], true
	}
	if !l.capped || len(l.starts) == 0 {
		return len(l.src), false
	}
	last := len(l.starts) - 1
	start := l.starts[last]
	want := line - last
	// The scan length is not known before it runs; charge the remaining source
	// once, before the first byte is examined.
	_ = l.guard.Add(uint64(len(l.src) - start))
	count := 0
	for j := start; j < len(l.src); j++ {
		if l.src[j] != '\n' {
			continue
		}
		count++
		if count == want {
			return j + 1, true
		}
	}
	return len(l.src), false
}

// Len reports how many line starts are stored, capped at MaxLineStarts.
func (l *Lines) Len() int {
	if l == nil {
		return 0
	}
	return len(l.starts)
}

// Capped reports that the index is incomplete and lookups past the stored part
// scan the source.
func (l *Lines) Capped() bool {
	if l == nil {
		return false
	}
	return l.capped
}
