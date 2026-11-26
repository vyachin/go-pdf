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
		// The standard library reports this both for a document that declares
		// windows-1252 and for a document that declares nothing and is not valid
		// UTF-8. A declaration is rejected whatever its bytes happen to be: only
		// the no-declaration fallback may be accepted as UTF-8.
		if label, offset := declaredCharset(src); label != "" {
			return nil, newError(src, 0, offset, utf8.Valid(src), "unsupported document encoding %q: supported encodings are %s (set Options.Charset to name the encoding explicitly)", label, supportedClause())
		}
		// ASCII is identical in every supported encoding, so a document that
		// declares nothing and decodes as UTF-8 is accepted; anything else is
		// exactly the failure this package exists to name, reported at its
		// first invalid byte.
		if utf8.Valid(src) {
			return &Encoding{name: UTF8, utf8: true}, nil
		}
		off := invalidOffset(src)
		// Everything before the first invalid byte is valid UTF-8, so the
		// character column of that byte is defined: count it as one.
		return nil, newError(src, 0, off, true, "%s", undecodableMessage())
	default:
		if strings.HasPrefix(name, "utf-16") {
			skip, off := byteOrderMark(src)
			return nil, newError(src, skip, off, false, "UTF-16 input is not supported: convert the document to one of %s", supportedClause())
		}
		// A declaration the set does not accept. The prescan names the <meta>
		// that carries it; when it does not recognise one, the diagnostic has
		// no position rather than pointing at an ignored tag.
		_, offset := declaredCharset(src)
		return nil, newError(src, 0, offset, utf8.Valid(src), "unsupported document encoding %q: supported encodings are %s (set Options.Charset to name the encoding explicitly)", name, supportedClause())
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

// utf8ByteOrderMark is the UTF-8 byte order mark. Decode strips it from the
// text, but a position still maps back to source bytes that include it.
var utf8ByteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// Column returns the 1-based character column of offset within the line that
// starts at lineStart. Both are byte offsets into src, which is UTF-8 text.
// The count is of Unicode code points, not bytes, so a diagnostic after a
// Cyrillic word names the column the author sees in an editor.
//
// A leading UTF-8 byte order mark is not a character: the first line starts
// after it, so an offset before or inside the mark names column 1 and adding
// or removing the mark never shifts a column. An offset before lineStart or
// past the end of src is clamped.
func Column(src []byte, lineStart, offset int) int {
	if lineStart < 0 {
		lineStart = 0
	}
	if lineStart > len(src) {
		lineStart = len(src)
	}
	if offset < lineStart {
		offset = lineStart
	}
	if offset > len(src) {
		offset = len(src)
	}
	if lineStart < len(utf8ByteOrderMark) && bytes.HasPrefix(src, utf8ByteOrderMark) {
		lineStart = len(utf8ByteOrderMark)
		if offset < lineStart {
			offset = lineStart
		}
	}
	return utf8.RuneCount(src[lineStart:offset]) + 1
}

// newError builds an Error with the position of offset in src. skip is the
// number of bytes covered by a stripped byte order mark, which a position does
// not count: the mark is invisible in the document. utf8Source reports whether
// src is UTF-8; a single-byte encoding has one character per byte, so its
// column is the byte count and existing numbers do not move.
func newError(src []byte, skip, offset int, utf8Source bool, format string, args ...any) *Error {
	e := &Error{Msg: fmt.Sprintf(format, args...), Offset: offset}
	if offset >= 0 {
		e.Line, e.Col = sourcePosition(src, skip, offset, utf8Source)
	}
	return e
}

// sourcePosition converts a byte offset into a 1-based line and column, with
// line 1 starting after the bytes a byte order mark covered.
func sourcePosition(src []byte, skip, offset int, utf8Source bool) (line, col int) {
	if offset > len(src) {
		offset = len(src)
	}
	line, start := 1, skip
	for i := skip; i < offset; i++ {
		if src[i] == '\n' {
			line, start = line+1, i+1
		}
	}
	if utf8Source {
		col = Column(src, start, offset)
	} else {
		col = offset - start + 1
	}
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

// prescanWindow bounds the declaration prescan: the HTML standard reads a
// declaration only from the first 1024 bytes.
const prescanWindow = 1024

// declaredCharset returns the label of the document's own encoding declaration
// and the offset of the <meta> tag that carries it, or "" and -1 when the
// document declares none. It is the prescan of the HTML standard restricted to
// what this package can decide: a real <meta charset> attribute or a
// <meta http-equiv="Content-Type" content="...charset=..."> element.
//
// Comments, doctypes, processing instructions, non-meta elements and the other
// attribute values are not declarations, so they are skipped. The scan works
// on the original bytes and folds ASCII only where it compares, so the offset
// is the author's byte offset and a diagnostic column does not move.
func declaredCharset(src []byte) (label string, offset int) {
	skip, _ := byteOrderMark(src)
	if skip == 2 {
		// A UTF-16 mark decides the encoding before the prescan; its bytes are
		// not ASCII markup.
		return "", -1
	}
	window := src[skip:]
	if len(window) > prescanWindow {
		window = window[:prescanWindow]
	}
	for i := 0; i < len(window); {
		idx := bytes.IndexByte(window[i:], '<')
		if idx < 0 {
			return "", -1
		}
		start := i + idx
		if start+1 >= len(window) {
			// An unterminated tag consumes the rest of the window.
			return "", -1
		}
		switch c := window[start+1]; {
		case c == '!':
			if bytes.HasPrefix(window[start:], []byte("<!--")) {
				end := bytes.Index(window[start+4:], []byte("-->"))
				if end < 0 {
					return "", -1
				}
				i = start + 4 + end + 3
				continue
			}
			// <!DOCTYPE ...> and the other markup declarations.
			end := bytes.IndexByte(window[start:], '>')
			if end < 0 {
				return "", -1
			}
			i = start + end + 1
		case c == '?':
			// A processing instruction or an XML declaration.
			end := bytes.IndexByte(window[start:], '>')
			if end < 0 {
				return "", -1
			}
			i = start + end + 1
		case c == '/':
			end := skipTag(window, start+2)
			if end < 0 {
				return "", -1
			}
			i = end
		case isASCIIAlpha(c):
			nameEnd := start + 1
			for nameEnd < len(window) && !isTagNameEnd(window[nameEnd]) {
				nameEnd++
			}
			if nameEnd == len(window) {
				// The tag never ends: no declaration, and no fallback to a
				// substring search.
				return "", -1
			}
			if !equalFoldASCII(window[start+1:nameEnd], "meta") {
				end := skipTag(window, nameEnd)
				if end < 0 {
					return "", -1
				}
				i = end
				continue
			}
			label, end := metaDeclaration(window, start, nameEnd)
			if end < 0 {
				return "", -1
			}
			if label != "" {
				return label, skip + start
			}
			i = end
		default:
			// A '<' that is not markup, as in "a < b": the scan resumes after
			// it.
			i = start + 1
		}
	}
	return "", -1
}

// metaDeclaration parses the attributes of the <meta> tag that starts at start,
// whose name ends at pos. It returns the declared label, or the offset just
// after the tag when it declares nothing, or -1 when the tag is unterminated.
// A '>' inside a quoted value does not end the tag.
func metaDeclaration(window []byte, start, pos int) (label string, end int) {
	var charsetValue, httpEquivValue, contentValue []byte
	var haveCharset, haveHTTPEquiv, haveContent bool
	for {
		for pos < len(window) && isHTMLSpace(window[pos]) {
			pos++
		}
		if pos >= len(window) {
			return "", -1
		}
		if window[pos] == '>' {
			pos++
			break
		}
		if window[pos] == '/' {
			pos++
			continue
		}
		nameStart := pos
		for pos < len(window) && !isHTMLSpace(window[pos]) && window[pos] != '=' && window[pos] != '>' && window[pos] != '/' {
			pos++
		}
		name := window[nameStart:pos]
		for pos < len(window) && isHTMLSpace(window[pos]) {
			pos++
		}
		var value []byte
		if pos < len(window) && window[pos] == '=' {
			pos++
			for pos < len(window) && isHTMLSpace(window[pos]) {
				pos++
			}
			switch {
			case pos < len(window) && (window[pos] == '"' || window[pos] == '\''):
				quote := window[pos]
				pos++
				valueStart := pos
				for pos < len(window) && window[pos] != quote {
					pos++
				}
				value = window[valueStart:pos]
				if pos < len(window) {
					pos++
				}
			default:
				valueStart := pos
				for pos < len(window) && !isHTMLSpace(window[pos]) && window[pos] != '>' {
					pos++
				}
				value = window[valueStart:pos]
			}
		}
		// The first of a duplicated attribute wins, as in the HTML tokenizer.
		switch {
		case !haveCharset && equalFoldASCII(name, "charset"):
			haveCharset, charsetValue = true, value
		case !haveHTTPEquiv && equalFoldASCII(name, "http-equiv"):
			haveHTTPEquiv, httpEquivValue = true, value
		case !haveContent && equalFoldASCII(name, "content"):
			haveContent, contentValue = true, value
		}
	}
	if haveCharset {
		return encodingLabel(charsetValue), pos
	}
	if haveHTTPEquiv && haveContent && equalFoldASCII(trimHTMLSpace(httpEquivValue), "content-type") {
		return contentCharsetLabel(contentValue), pos
	}
	return "", pos
}

// contentCharsetLabel returns the label of the first charset= parameter in a
// meta content attribute, or "" when it names none.
func contentCharsetLabel(content []byte) string {
	for i := 0; i < len(content); {
		idx := indexFoldASCII(content[i:], "charset")
		if idx < 0 {
			return ""
		}
		pos := i + idx + len("charset")
		i = pos
		for pos < len(content) && isHTMLSpace(content[pos]) {
			pos++
		}
		if pos >= len(content) || content[pos] != '=' {
			continue
		}
		pos++
		for pos < len(content) && isHTMLSpace(content[pos]) {
			pos++
		}
		var value []byte
		switch {
		case pos < len(content) && (content[pos] == '"' || content[pos] == '\''):
			quote := content[pos]
			pos++
			valueStart := pos
			for pos < len(content) && content[pos] != quote {
				pos++
			}
			if pos >= len(content) {
				return ""
			}
			value = content[valueStart:pos]
		default:
			valueStart := pos
			for pos < len(content) && content[pos] != ';' && !isHTMLSpace(content[pos]) {
				pos++
			}
			value = content[valueStart:pos]
		}
		if label := encodingLabel(value); label != "" {
			return label
		}
	}
	return ""
}

// encodingLabel returns value without the ASCII whitespace the standard strips
// around an encoding label, or "" when it names no label. A label has at least
// one ASCII alphanumeric character, which keeps a stray "<" or "-" from being
// reported as an encoding.
func encodingLabel(value []byte) string {
	label := string(trimHTMLSpace(value))
	if label == "" {
		return ""
	}
	for i := 0; i < len(label); i++ {
		if c := label[i]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return label
		}
	}
	return ""
}

// skipTag returns the offset just after the tag whose name ends at pos, or -1
// when the tag never ends. A '>' inside a quoted value does not end it.
func skipTag(window []byte, pos int) int {
	quote := byte(0)
	for pos < len(window) {
		switch c := window[pos]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return pos + 1
		}
		pos++
	}
	return -1
}

// equalFoldASCII compares b with an ASCII-lowercase string without changing
// the byte length of either.
func equalFoldASCII(b []byte, lower string) bool {
	if len(b) != len(lower) {
		return false
	}
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

// indexFoldASCII returns the offset of the first ASCII-case-insensitive match
// of lower in b, or -1.
func indexFoldASCII(b []byte, lower string) int {
	for i := 0; i+len(lower) <= len(b); i++ {
		if equalFoldASCII(b[i:i+len(lower)], lower) {
			return i
		}
	}
	return -1
}

// trimHTMLSpace returns b without leading and trailing HTML whitespace.
func trimHTMLSpace(b []byte) []byte {
	for len(b) > 0 && isHTMLSpace(b[0]) {
		b = b[1:]
	}
	for len(b) > 0 && isHTMLSpace(b[len(b)-1]) {
		b = b[:len(b)-1]
	}
	return b
}

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isTagNameEnd reports whether c ends an HTML tag name.
func isTagNameEnd(c byte) bool {
	return isHTMLSpace(c) || c == '/' || c == '>'
}
