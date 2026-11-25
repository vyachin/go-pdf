package charset

import "bytes"

// Decoded is a source decoded to UTF-8 together with what a Mapper needs to
// translate a decoded offset back to the bytes it came from.
type Decoded struct {
	// Text is the UTF-8 text to parse.
	Text []byte
	// Widths holds the width in Text of every source byte. Nil means one byte
	// each, which is what UTF-8 without a byte order mark keeps.
	Widths []uint8
	// Skip is the number of source bytes a stripped byte order mark covered.
	// A position does not count them: the mark is invisible in the document.
	Skip int
}

// Decode decodes src with enc. A UTF-8 source is validated, not re-encoded, and
// a leading byte order mark is removed; a single-byte source is translated byte
// by byte through the encoding's table, so the width of every decoded byte is
// known.
//
// The returned error is an *Error at the first byte the encoding does not
// define, or at the first byte that is not valid UTF-8. A byte is never
// replaced with U+FFFD and passed on.
func Decode(enc *Encoding, src []byte) (*Decoded, error) {
	if enc == nil || enc.utf8 {
		return decodeUTF8(src)
	}
	// An explicit legacy encoding must not turn a UTF-8 byte order mark into
	// visible "п»ї" text: the mark is not part of the document.
	skip := 0
	if bytes.HasPrefix(src, []byte{0xEF, 0xBB, 0xBF}) {
		skip = 3
	}
	body := src[skip:]
	text := make([]byte, 0, len(body)+len(body)/4)
	widths := make([]uint8, len(src))
	for i := 0; i < len(body); i++ {
		b := body[i]
		r := enc.table[b]
		if r == nil {
			return nil, newError(src, skip, skip+i, "byte %#02x is not defined in %s", b, enc.name)
		}
		text = append(text, r...)
		widths[skip+i] = uint8(len(r))
	}
	return &Decoded{Text: text, Widths: widths, Skip: skip}, nil
}

// decodeUTF8 validates src and removes a leading byte order mark.
func decodeUTF8(src []byte) (*Decoded, error) {
	skip, _ := byteOrderMark(src)
	body := src[skip:]
	if off := invalidOffset(body); off >= 0 {
		return nil, newError(src, skip, skip+off, "%s", invalidUTF8Message())
	}
	if skip == 0 {
		return &Decoded{Text: src}, nil
	}
	return &Decoded{Text: body, Skip: skip}, nil
}

// lineStarts returns the offset of every line start in src.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}
