// Package charset resolves the input encoding of a document and decodes its
// bytes to UTF-8 without losing the link to the source: a caller can still
// report a diagnostic at the byte, line and column of the original file.
//
// Detection is the HTML standard's: a byte order mark, then a <meta charset> or
// <meta http-equiv="Content-Type"> declaration in the first 1024 bytes, then
// UTF-8. The work itself is done by golang.org/x/net/html/charset, so the
// label table and the precedence rules are the Go project's, not this
// package's.
//
// The accepted set is closed: UTF-8, windows-1251, KOI8-R and IBM866. Those
// three legacy encodings are single-byte, ASCII-compatible and total enough to
// decode byte by byte, which is what keeps source positions exact. windows-1252
// is deliberately absent: the standard library reports it both for a document
// that declares it and for a document that declares nothing and is not valid
// UTF-8, so accepting it would read an undeclared windows-1251 document as
// Latin mojibake.
package charset

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
)

// The canonical names of the supported encodings, as html/charset reports them.
const (
	UTF8        = "utf-8"
	Windows1251 = "windows-1251"
	KOI8R       = "koi8-r"
	IBM866      = "ibm866"
)

// replacement is the UTF-8 encoding of U+FFFD, which a decoder produces for a
// byte its encoding does not define.
var replacement = []byte("\uFFFD")

// Supported returns the canonical names of the accepted input encodings.
func Supported() []string {
	return []string{UTF8, Windows1251, KOI8R, IBM866}
}

// supportedClause lists the supported names inside a sentence.
func supportedClause() string {
	names := Supported()
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Encoding is one resolved input encoding. It is immutable and shared across
// conversions: two conversions that read the same encoding use the same value.
type Encoding struct {
	name  string
	utf8  bool
	table *[256][]byte
}

// Name returns the canonical encoding name, for example "windows-1251".
func (e *Encoding) Name() string { return e.name }

// IsUTF8 reports whether the encoding is UTF-8, which needs no table.
func (e *Encoding) IsUTF8() bool { return e.utf8 }

// Resolve returns the encoding of src. An explicit name, when given, wins over
// the document's own declaration and its byte order mark; otherwise the
// encoding is detected from the bytes.
//
// The returned error is an *Error that names the problem and, where the source
// makes it possible, the offending byte.
func Resolve(src []byte, explicit string) (*Encoding, error) {
	if explicit != "" {
		return Validate(explicit)
	}
	enc, name, _ := charset.DetermineEncoding(src, "")
	switch name {
	case UTF8:
		return &Encoding{name: UTF8, utf8: true}, nil
	case Windows1251, KOI8R, IBM866:
		return &Encoding{name: name, table: tableFor(name, enc)}, nil
	case "windows-1252":
		// The standard library reports this both for a pure ASCII document and
		// for a document that declares nothing and is not valid UTF-8. ASCII is
		// identical in every supported encoding, so the first is accepted as
		// UTF-8; the second is exactly the failure this package exists to name,
		// and it is reported at its first invalid byte.
		if utf8.Valid(src) {
			return &Encoding{name: UTF8, utf8: true}, nil
		}
		off := invalidOffset(src)
		return nil, newError(src, 0, off, "%s", undecodableMessage())
	default:
		if strings.HasPrefix(name, "utf-16") {
			skip, off := byteOrderMark(src)
			return nil, newError(src, skip, off, "UTF-16 input is not supported: convert the document to one of %s", supportedClause())
		}
		// A declaration the set does not accept. The standard library does not
		// expose where it found it, so the first <meta> of the window is named.
		return nil, newError(src, 0, firstMetaOffset(src), "unsupported document encoding %q: supported encodings are %s", name, supportedClause())
	}
}

// Validate resolves an encoding named by the caller.
func Validate(name string) (*Encoding, error) {
	enc, canonical := charset.Lookup(name)
	if enc == nil {
		return nil, &Error{Offset: -1, Msg: fmt.Sprintf("unknown charset %q: supported encodings are %s", name, supportedClause())}
	}
	switch canonical {
	case UTF8:
		return &Encoding{name: UTF8, utf8: true}, nil
	case Windows1251, KOI8R, IBM866:
		return &Encoding{name: canonical, table: tableFor(canonical, enc)}, nil
	}
	return nil, &Error{Offset: -1, Msg: fmt.Sprintf("unsupported charset %q: supported encodings are %s", canonical, supportedClause())}
}

// undecodableMessage explains a document that is not valid UTF-8 and does not
// declare an accepted encoding. The standard library cannot tell a windows-1252
// declaration from the fallback it reports for such a document, so both are
// answered here, and the message names the case a Windows user is most likely
// to have hit.
func undecodableMessage() string {
	return "the document is not valid UTF-8 and declares no supported encoding (windows-1252 is not supported): " +
		"its bytes must be valid UTF-8, or it must declare one of " + supportedClause() +
		" with <meta charset>, or the caller must name one with Options.Charset"
}

// invalidUTF8Message explains a document whose bytes are not UTF-8 even though
// that is the encoding it resolves to.
func invalidUTF8Message() string {
	return "the document is not valid UTF-8: its bytes must be valid UTF-8, or it must declare one of " +
		supportedClause() + " with <meta charset>, or the caller must name one with Options.Charset"
}

// tableCache holds one immutable 256-entry table per canonical encoding name,
// so that the 256 decoder calls happen once per process rather than once per
// conversion. The values never change after they are stored.
var tableCache sync.Map

// tableFor returns the byte table of an encoding, building it on first use.
func tableFor(name string, enc encoding.Encoding) *[256][]byte {
	if t, ok := tableCache.Load(name); ok {
		return t.(*[256][]byte)
	}
	t := buildTable(enc)
	actual, _ := tableCache.LoadOrStore(name, t)
	return actual.(*[256][]byte)
}

// buildTable decodes every possible byte once. Entry b holds the UTF-8 bytes
// byte b becomes, or is nil when the encoding does not define b. An undefined
// byte decodes to U+FFFD, which no encoding maps a byte to on purpose, so the
// replacement character is the undefined marker.
func buildTable(enc encoding.Encoding) *[256][]byte {
	t := new([256][]byte)
	dec := enc.NewDecoder()
	var buf [utf8.UTFMax]byte
	for b := 0; b < 256; b++ {
		n, nsrc, err := dec.Transform(buf[:], []byte{byte(b)}, true)
		if err != nil || nsrc != 1 || n == 0 || bytes.Equal(buf[:n], replacement) {
			continue
		}
		t[b] = bytes.Clone(buf[:n])
	}
	return t
}

// Error reports input that cannot be decoded. Offset is the offending byte in
// the original source, and Line and Col are that byte as a 1-based position;
// both are zero when the problem has no position, such as a caller-named
// encoding that does not exist.
type Error struct {
	// Msg explains the problem for a human.
	Msg string
	// Offset is the source byte the problem starts at, or -1 when unknown.
	Offset int
	// Line and Col are Offset as a 1-based source position, zero when unknown.
	Line int
	Col  int
}

func (e *Error) Error() string { return e.Msg }

// newError builds an Error with the position of offset in src. skip is the
// number of bytes covered by a stripped byte order mark, which a position does
// not count: the mark is invisible in the document.
func newError(src []byte, skip, offset int, format string, args ...any) *Error {
	e := &Error{Msg: fmt.Sprintf(format, args...), Offset: offset}
	if offset >= 0 {
		e.Line, e.Col = sourcePosition(src, skip, offset)
	}
	return e
}

// sourcePosition converts a byte offset into a 1-based line and column, with
// line 1 starting after the bytes a byte order mark covered.
func sourcePosition(src []byte, skip, offset int) (line, col int) {
	if offset > len(src) {
		offset = len(src)
	}
	line, start := 1, skip
	for i := skip; i < offset; i++ {
		if src[i] == '\n' {
			line, start = line+1, i+1
		}
	}
	col = offset - start + 1
	if col < 1 {
		col = 1
	}
	return line, col
}

// invalidOffset returns the offset of the first byte that does not start a
// valid UTF-8 sequence, or -1 when src is valid.
func invalidOffset(src []byte) int {
	for i := 0; i < len(src); {
		if src[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

// byteOrderMark returns the length of the byte order mark at the start of src
// and its offset, or zero and -1 when there is none.
func byteOrderMark(src []byte) (skip, offset int) {
	for _, bom := range [][]byte{
		{0xEF, 0xBB, 0xBF},
		{0xFF, 0xFE},
		{0xFE, 0xFF},
	} {
		if bytes.HasPrefix(src, bom) {
			return len(bom), 0
		}
	}
	return 0, -1
}

// firstMetaOffset returns the offset of the first <meta> tag in the window the
// standard library scans for a declaration, or -1 when there is none.
func firstMetaOffset(src []byte) int {
	window := src
	if len(window) > 1024 {
		window = window[:1024]
	}
	return bytes.Index(bytes.ToLower(window), []byte("<meta"))
}
