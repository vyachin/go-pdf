package pdfdebug

import (
	"bytes"
	"fmt"
	"strconv"
)

// Validate checks the structure of a PDF: the startxref pointer, every
// cross-reference entry and every stream's /Length. It parses the bytes it is
// given and shares no bookkeeping with the writer, so a wrong length or a
// shifted offset is reported instead of being read through the same assumption
// that produced it.
func Validate(data []byte) error {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return fmt.Errorf("not a PDF: missing header")
	}
	start, err := startxrefOffset(data)
	if err != nil {
		return err
	}
	entries, err := xrefEntries(data, start)
	if err != nil {
		return err
	}
	if err := checkObjects(data, entries); err != nil {
		return err
	}
	return checkStreamLengths(data)
}

// startxrefOffset returns the offset named by the last startxref.
func startxrefOffset(data []byte) (int, error) {
	idx := bytes.LastIndex(data, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("no startxref")
	}
	rest := data[idx+len("startxref"):]
	rest = bytes.TrimLeft(rest, " \t\r\n")
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, fmt.Errorf("startxref names no offset")
	}
	offset, err := strconv.Atoi(string(rest[:end]))
	if err != nil {
		return 0, fmt.Errorf("startxref offset: %w", err)
	}
	if offset < 0 || offset >= len(data) {
		return 0, fmt.Errorf("startxref offset %d is outside the file", offset)
	}
	return offset, nil
}

// xrefEntry is one cross-reference record: the object number and its byte
// offset, or a free entry when free is true.
type xrefEntry struct {
	number int
	offset int
	free   bool
}

// xrefEntries parses the cross-reference table at offset, including its
// subsections.
func xrefEntries(data []byte, offset int) ([]xrefEntry, error) {
	rest := data[offset:]
	if !bytes.HasPrefix(rest, []byte("xref")) {
		return nil, fmt.Errorf("startxref points at %q, not at a cross-reference table", snippet(rest))
	}
	rest = rest[len("xref"):]
	var out []xrefEntry
	for {
		rest = bytes.TrimLeft(rest, " \t\r\n")
		if bytes.HasPrefix(rest, []byte("trailer")) {
			return out, nil
		}
		line, after, ok := bytes.Cut(rest, []byte("\n"))
		if !ok {
			return nil, fmt.Errorf("the cross-reference header is truncated")
		}
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("bad cross-reference subsection %q", line)
		}
		first, err := strconv.Atoi(string(fields[0]))
		if err != nil {
			return nil, fmt.Errorf("bad cross-reference subsection: %w", err)
		}
		count, err := strconv.Atoi(string(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("bad cross-reference count: %w", err)
		}
		rest = after
		for i := 0; i < count; i++ {
			if len(rest) < 20 {
				return nil, fmt.Errorf("the cross-reference table is truncated")
			}
			entry := rest[:20]
			rest = rest[20:]
			if entry[17] == 'f' {
				out = append(out, xrefEntry{number: first + i, free: true})
				continue
			}
			if entry[17] != 'n' {
				return nil, fmt.Errorf("bad cross-reference entry %q", entry)
			}
			objOffset, err := strconv.Atoi(string(bytes.TrimSpace(entry[0:10])))
			if err != nil || objOffset <= 0 || objOffset >= len(data) {
				return nil, fmt.Errorf("cross-reference entry %d points at offset %q", first+i, entry[0:10])
			}
			out = append(out, xrefEntry{number: first + i, offset: objOffset})
		}
	}
}

// checkObjects verifies that every in-use cross-reference entry points at the
// header of the object it names.
func checkObjects(data []byte, entries []xrefEntry) error {
	for _, e := range entries {
		if e.free {
			continue
		}
		want := fmt.Sprintf("%d 0 obj", e.number)
		if !bytes.HasPrefix(data[e.offset:], []byte(want)) {
			return fmt.Errorf("cross-reference entry %d points at %q, not at the object", e.number, snippet(data[e.offset:]))
		}
	}
	return nil
}

// checkStreamLengths verifies every stream's /Length against the bytes the
// stream really holds.
func checkStreamLengths(data []byte) error {
	for pos := 0; ; {
		// The keyword is the one that follows a dictionary, not the "stream"
		// inside "endstream".
		idx := bytes.Index(data[pos:], []byte("\nstream\n"))
		if idx < 0 {
			return nil
		}
		header := data[pos : pos+idx]
		start := pos + idx + len("\nstream\n")
		length, err := dictLength(header)
		if err != nil {
			return err
		}
		end := start + length
		if end > len(data) {
			return fmt.Errorf("a stream declares %d bytes, the file ends first", length)
		}
		if !bytes.HasPrefix(data[end:], []byte("\nendstream")) && !bytes.HasPrefix(data[end:], []byte("\r\nendstream")) {
			return fmt.Errorf("a stream declares %d bytes, but %q follows", length, snippet(data[end:]))
		}
		pos = end
	}
}

// dictLength returns the /Length of the stream whose dictionary precedes the
// stream keyword.
func dictLength(header []byte) (int, error) {
	// The dictionary starts at the last "<<": the object header may follow a
	// previous stream.
	start := bytes.LastIndex(header, []byte("<<"))
	if start < 0 {
		return 0, fmt.Errorf("a stream has no dictionary")
	}
	dict := header[start:]
	i := bytes.Index(dict, []byte("/Length"))
	if i < 0 {
		return 0, fmt.Errorf("a stream has no /Length")
	}
	rest := dict[i+len("/Length"):]
	rest = bytes.TrimLeft(rest, " \t\r\n")
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, fmt.Errorf("a stream's /Length is not a number")
	}
	return strconv.Atoi(string(rest[:end]))
}

// snippet renders the first bytes of b for a message.
func snippet(b []byte) string {
	if len(b) > 24 {
		b = b[:24]
	}
	return string(bytes.TrimSpace(b))
}
