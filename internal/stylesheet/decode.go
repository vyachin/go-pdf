package stylesheet

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/diag"
)

// decode decodes one linked stylesheet and consumes a @charset rule at its
// start. A rule that names the document's encoding is accepted and blanked, so
// that the parser sees whitespace where the rule was and every later offset
// stays where it was; a rule that names another encoding is an error. The
// returned mapper is nil when the decoded text is the source byte for byte.
func decode(enc *charset.Encoding, path string, data []byte, guard *budget.Guard) ([]byte, *charset.Mapper, error) {
	dec, err := charset.DecodeWithGuard(enc, data, guard)
	if err != nil {
		var encErr *charset.Error
		if errors.As(err, &encErr) {
			pos := diag.Position{File: path, Line: encErr.Line, Col: encErr.Col}
			return nil, nil, &linkError{
				code: diag.CSSLinkEncoding,
				msg:  fmt.Sprintf("cannot decode stylesheet %q: %s", path, encErr.Msg),
				pos:  &pos,
			}
		}
		return nil, nil, err
	}
	documentEncoding := charset.UTF8
	if enc != nil {
		documentEncoding = enc.Name()
	}
	text := dec.Text
	if label, end, ok := charsetRule(text); ok {
		named, err := charset.Validate(label)
		if err != nil || named.Name() != documentEncoding {
			pos := diag.Position{File: path, Line: 1, Col: 1}
			return nil, nil, &linkError{
				code: diag.CSSLinkEncoding,
				msg:  fmt.Sprintf("stylesheet %q declares @charset %q but the document encoding is %q", path, label, documentEncoding),
				pos:  &pos,
			}
		}
		// Blank the rule instead of removing it: same length, same line
		// structure, the parser sees whitespace, and every later offset stays
		// where the author wrote it.
		text = bytes.Clone(text)
		for i := 0; i < end; i++ {
			if text[i] != '\n' && text[i] != '\r' {
				text[i] = ' '
			}
		}
	}
	if dec.Widths == nil && dec.Skip == 0 {
		// The decoded text is the source byte for byte, so offsets already
		// point into the file the author edits.
		return text, nil, nil
	}
	return text, charset.NewMapperWithGuard(data, text, dec.Widths, dec.Skip, guard), nil
}

// charsetRule returns the encoding label of a @charset rule at the very start
// of a stylesheet and the offset just past the rule. The rule must be the first
// thing in the file, as CSS requires.
func charsetRule(text []byte) (label string, end int, ok bool) {
	const prefix = "@charset"
	if len(text) < len(prefix) || !strings.EqualFold(string(text[:len(prefix)]), prefix) {
		return "", 0, false
	}
	i := len(prefix)
	for i < len(text) && isSpace(text[i]) {
		i++
	}
	if i >= len(text) || (text[i] != '"' && text[i] != '\'') {
		return "", 0, false
	}
	quote := text[i]
	j := i + 1
	for j < len(text) && text[j] != quote {
		j++
	}
	if j >= len(text) {
		return "", 0, false
	}
	label = string(text[i+1 : j])
	j++
	for j < len(text) && isSpace(text[j]) {
		j++
	}
	if j >= len(text) || text[j] != ';' {
		return "", 0, false
	}
	return label, j + 1, true
}

// isSpace reports whether b is CSS whitespace.
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f'
}
