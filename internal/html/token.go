package html

import (
	"bytes"
	"strconv"
	"strings"
)

// TokenKind classifies tokens produced by the tokenizer.
type TokenKind int

const (
	// ErrorToken is a tokenization failure.
	ErrorToken TokenKind = iota
	// TextToken is a run of character data with entities decoded.
	TextToken
	// StartTagToken opens an element.
	StartTagToken
	// EndTagToken closes an element.
	EndTagToken
	// CommentToken is a comment.
	CommentToken
	// DoctypeToken is a doctype declaration.
	DoctypeToken
	// EOFToken marks the end of the input.
	EOFToken
)

func (k TokenKind) String() string {
	switch k {
	case TextToken:
		return "text"
	case StartTagToken:
		return "start"
	case EndTagToken:
		return "end"
	case CommentToken:
		return "comment"
	case DoctypeToken:
		return "doctype"
	case EOFToken:
		return "eof"
	default:
		return "error"
	}
}

// Token is a single lexical unit of an HTML document.
type Token struct {
	Kind        TokenKind
	Name        string
	Attrs       []Attr
	Data        string
	Offset      int
	End         int
	SelfClosing bool
}

// Tokenizer turns HTML source into tokens. It is a pragmatic subset of the
// WHATWG tokenizer: it handles tags, attributes, comments, doctypes, numeric
// and named character references and raw text elements. It deliberately does
// not implement scripting, CDATA or foreign content.
type Tokenizer struct {
	src    []byte
	pos    int
	rawTag string
}

// NewTokenizer creates a tokenizer over src.
func NewTokenizer(src []byte) *Tokenizer { return &Tokenizer{src: src} }

// SetRawTag makes the tokenizer treat everything up to the matching end tag as
// text. It is used for script and style elements.
func (z *Tokenizer) SetRawTag(name string) { z.rawTag = strings.ToLower(name) }

// Next returns the next token.
func (z *Tokenizer) Next() Token {
	if z.rawTag != "" {
		return z.readRawText()
	}
	if z.pos >= len(z.src) {
		return Token{Kind: EOFToken, Offset: z.pos, End: z.pos}
	}
	if z.src[z.pos] != '<' {
		return z.readText()
	}
	rest := z.src[z.pos:]
	switch {
	case hasPrefixFold(rest, "<!--"):
		return z.readComment()
	case hasPrefixFold(rest, "<!doctype"):
		return z.readDoctype()
	case len(rest) > 1 && rest[1] == '!':
		return z.readBogusComment()
	case len(rest) > 2 && rest[1] == '/' && isLetter(rest[2]):
		return z.readEndTag()
	case len(rest) > 1 && isLetter(rest[1]):
		return z.readStartTag()
	default:
		// A stray '<' is text.
		z.pos++
		return Token{Kind: TextToken, Data: "<", Offset: z.pos - 1, End: z.pos}
	}
}

func (z *Tokenizer) readText() Token {
	start := z.pos
	for z.pos < len(z.src) && z.src[z.pos] != '<' {
		z.pos++
	}
	return Token{Kind: TextToken, Data: decodeEntities(string(z.src[start:z.pos])), Offset: start, End: z.pos}
}

func (z *Tokenizer) readComment() Token {
	start := z.pos
	z.pos += 4 // <!--
	bodyStart := z.pos
	if i := indexFold(z.src[z.pos:], "-->"); i >= 0 {
		z.pos += i
		data := string(z.src[bodyStart:z.pos])
		z.pos += 3
		return Token{Kind: CommentToken, Data: data, Offset: start, End: z.pos}
	}
	z.pos = len(z.src)
	return Token{Kind: CommentToken, Data: string(z.src[bodyStart:]), Offset: start, End: z.pos}
}

func (z *Tokenizer) readBogusComment() Token {
	start := z.pos
	z.pos += 2 // <!
	bodyStart := z.pos
	if i := bytes.IndexByte(z.src[z.pos:], '>'); i >= 0 {
		z.pos += i
		data := string(z.src[bodyStart:z.pos])
		z.pos++
		return Token{Kind: CommentToken, Data: data, Offset: start, End: z.pos}
	}
	z.pos = len(z.src)
	return Token{Kind: CommentToken, Data: string(z.src[bodyStart:]), Offset: start, End: z.pos}
}

func (z *Tokenizer) readDoctype() Token {
	start := z.pos
	z.pos += 9 // <!doctype
	bodyStart := z.pos
	end := len(z.src)
	if i := bytes.IndexByte(z.src[z.pos:], '>'); i >= 0 {
		end = z.pos + i
	}
	data := strings.TrimSpace(string(z.src[bodyStart:end]))
	z.pos = end
	if z.pos < len(z.src) {
		z.pos++
	}
	return Token{Kind: DoctypeToken, Data: data, Offset: start, End: z.pos}
}

func (z *Tokenizer) readEndTag() Token {
	start := z.pos
	z.pos += 2 // </
	nameStart := z.pos
	for z.pos < len(z.src) && isTagNameChar(z.src[z.pos]) {
		z.pos++
	}
	name := strings.ToLower(string(z.src[nameStart:z.pos]))
	// Skip anything up to '>'.
	for z.pos < len(z.src) && z.src[z.pos] != '>' {
		z.pos++
	}
	if z.pos < len(z.src) {
		z.pos++
	}
	return Token{Kind: EndTagToken, Name: name, Offset: start, End: z.pos}
}

func (z *Tokenizer) readStartTag() Token {
	start := z.pos
	z.pos++ // <
	nameStart := z.pos
	for z.pos < len(z.src) && isTagNameChar(z.src[z.pos]) {
		z.pos++
	}
	name := strings.ToLower(string(z.src[nameStart:z.pos]))
	var attrs []Attr
	seen := map[string]bool{}
	selfClosing := false
	for {
		z.skipSpace()
		if z.pos >= len(z.src) {
			break
		}
		switch z.src[z.pos] {
		case '>':
			z.pos++
			return Token{Kind: StartTagToken, Name: name, Attrs: attrs, Offset: start, End: z.pos, SelfClosing: selfClosing}
		case '/':
			z.pos++
			if z.pos < len(z.src) && z.src[z.pos] == '>' {
				z.pos++
				return Token{Kind: StartTagToken, Name: name, Attrs: attrs, Offset: start, End: z.pos, SelfClosing: true}
			}
			continue
		}
		keyStart := z.pos
		for z.pos < len(z.src) && !isSpace(z.src[z.pos]) && z.src[z.pos] != '=' && z.src[z.pos] != '>' && z.src[z.pos] != '/' {
			z.pos++
		}
		key := strings.ToLower(string(z.src[keyStart:z.pos]))
		if key == "" {
			z.pos++
			continue
		}
		z.skipSpace()
		value := ""
		if z.pos < len(z.src) && z.src[z.pos] == '=' {
			z.pos++
			z.skipSpace()
			if z.pos < len(z.src) {
				switch q := z.src[z.pos]; q {
				case '"', '\'':
					z.pos++
					vStart := z.pos
					for z.pos < len(z.src) && z.src[z.pos] != q {
						z.pos++
					}
					value = string(z.src[vStart:z.pos])
					if z.pos < len(z.src) {
						z.pos++
					}
				default:
					vStart := z.pos
					for z.pos < len(z.src) && !isSpace(z.src[z.pos]) && z.src[z.pos] != '>' {
						z.pos++
					}
					value = string(z.src[vStart:z.pos])
				}
			}
		}
		if !seen[key] {
			seen[key] = true
			attrs = append(attrs, Attr{Key: key, Value: decodeEntities(value)})
		}
	}
	return Token{Kind: StartTagToken, Name: name, Attrs: attrs, Offset: start, End: z.pos, SelfClosing: selfClosing}
}

func (z *Tokenizer) readRawText() Token {
	start := z.pos
	closing := "</" + z.rawTag
	if i := indexFold(z.src[z.pos:], closing); i >= 0 {
		data := string(z.src[z.pos : z.pos+i])
		z.pos += i
		z.rawTag = ""
		return Token{Kind: TextToken, Data: data, Offset: start, End: z.pos}
	}
	data := string(z.src[z.pos:])
	z.pos = len(z.src)
	z.rawTag = ""
	return Token{Kind: TextToken, Data: data, Offset: start, End: z.pos}
}

func (z *Tokenizer) skipSpace() {
	for z.pos < len(z.src) && isSpace(z.src[z.pos]) {
		z.pos++
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isTagNameChar(c byte) bool {
	return isLetter(c) || c >= '0' && c <= '9' || c == '-' || c == ':' || c == '_' || c == '.'
}

func hasPrefixFold(b []byte, prefix string) bool {
	return len(b) >= len(prefix) && strings.EqualFold(string(b[:len(prefix)]), prefix)
}

// indexFold finds needle in b, comparing ASCII letters case-insensitively.
// Every needle the tokenizer searches for is ASCII ("-->", "</style>",
// "<!doctype"), so a byte-wise scan with ASCII folding is both exact for those
// needles and allocation free. Lowercasing the whole remainder instead would
// copy the rest of the input once per comment, doctype and raw-text element,
// which makes the tokenizer quadratic.
func indexFold(b []byte, needle string) int {
	n := len(needle)
	if n == 0 {
		return 0
	}
	if len(b) < n {
		return -1
	}
	for i := 0; i+n <= len(b); i++ {
		if foldASCII(b[i]) != foldASCII(needle[0]) {
			continue
		}
		j := 1
		for ; j < n; j++ {
			if foldASCII(b[i+j]) != foldASCII(needle[j]) {
				break
			}
		}
		if j == n {
			return i
		}
	}
	return -1
}

func foldASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

var namedEntities = map[string]string{
	"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'",
	"nbsp": "\u00a0", "copy": "©", "reg": "®", "trade": "™",
	"mdash": "—", "ndash": "–", "hellip": "…", "laquo": "«", "raquo": "»",
	"lsquo": "‘", "rsquo": "’", "ldquo": "“", "rdquo": "”",
	"deg": "°", "plusmn": "±", "times": "×", "divide": "÷",
	"euro": "€", "pound": "£", "yen": "¥", "cent": "¢",
	"nbsp_": "", "shy": "\u00ad", "middot": "·", "bullet": "•",
	"sect": "§", "para": "¶", "dagger": "†", "permil": "‰",
	"frac12": "½", "frac14": "¼", "frac34": "¾", "sup2": "²", "sup3": "³",
	"micro": "µ", "numero": "№",
}

// decodeEntities replaces named and numeric character references. Unknown
// entities are left untouched so that they remain visible in diagnostics.
func decodeEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// Only a short window can hold an entity name. Searching the whole
		// remaining input for every ampersand made an unterminated run
		// quadratic.
		const maxEntityLen = 32
		end := i + 1 + maxEntityLen
		if end > len(s) {
			end = len(s)
		}
		semi := strings.IndexByte(s[i+1:end], ';')
		if semi < 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		entity := s[i+1 : i+1+semi]
		if entity == "" {
			b.WriteByte(s[i])
			i++
			continue
		}
		if entity[0] == '#' {
			var code int64
			var err error
			if len(entity) > 1 && (entity[1] == 'x' || entity[1] == 'X') {
				code, err = strconv.ParseInt(entity[2:], 16, 32)
			} else {
				code, err = strconv.ParseInt(entity[1:], 10, 32)
			}
			if err == nil && code > 0 && code <= 0x10FFFF {
				b.WriteRune(rune(code))
				i += semi + 2
				continue
			}
		} else if v, ok := namedEntities[entity]; ok {
			b.WriteString(v)
			i += semi + 2
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
