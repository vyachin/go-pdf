package css

import (
	"strconv"
	"strings"
	"unicode"
)

// TokenKind classifies CSS tokens.
type TokenKind int

const (
	// IdentToken is an identifier such as a property name or keyword.
	IdentToken TokenKind = iota
	// HashToken is a "#name" token, used by colours and id selectors.
	HashToken
	// StringToken is a quoted string. Text keeps the surrounding quotes.
	StringToken
	// NumberToken is a plain number.
	NumberToken
	// PercentageToken is a number followed by '%'.
	PercentageToken
	// DimensionToken is a number followed by a unit, for example "12pt".
	DimensionToken
	// DelimToken is a single punctuation character such as "." or ">".
	DelimToken
	// WhitespaceToken is a run of whitespace, normalised to a single space.
	WhitespaceToken
	// ColonToken is ":".
	ColonToken
	// SemicolonToken is ";".
	SemicolonToken
	// CommaToken is ",".
	CommaToken
	// LBraceToken is "{".
	LBraceToken
	// RBraceToken is "}".
	RBraceToken
	// LParenToken is "(".
	LParenToken
	// RParenToken is ")".
	RParenToken
	// LBracketToken is "[".
	LBracketToken
	// RBracketToken is "]".
	RBracketToken
	// URLToken is an unquoted url(...) value; Text holds the raw contents.
	URLToken
	// AtKeywordToken is "@name".
	AtKeywordToken
	// EOFToken marks the end of the input.
	EOFToken
)

func (k TokenKind) String() string {
	switch k {
	case IdentToken:
		return "ident"
	case HashToken:
		return "hash"
	case StringToken:
		return "string"
	case NumberToken:
		return "number"
	case PercentageToken:
		return "percentage"
	case DimensionToken:
		return "dimension"
	case DelimToken:
		return "delim"
	case WhitespaceToken:
		return "whitespace"
	case ColonToken:
		return ":"
	case SemicolonToken:
		return ";"
	case CommaToken:
		return ","
	case LBraceToken:
		return "{"
	case RBraceToken:
		return "}"
	case LParenToken:
		return "("
	case RParenToken:
		return ")"
	case LBracketToken:
		return "["
	case RBracketToken:
		return "]"
	case URLToken:
		return "url"
	case AtKeywordToken:
		return "at-keyword"
	case EOFToken:
		return "eof"
	default:
		return "unknown"
	}
}

// Token is a single CSS token. Text always holds the raw source spelling so
// that selectors and values can be reconstructed exactly.
type Token struct {
	Kind   TokenKind
	Text   string
	Num    float64
	Unit   string
	Offset int
	// Bad marks a numeric token whose value does not fit in a float64. The
	// spelling is valid CSS, so the token is still produced; every consumer
	// must reject it instead of using a substituted zero.
	Bad bool
}

// Tokenize splits CSS source into tokens.
func Tokenize(src []byte) []Token {
	var out []Token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case isSpace(c):
			start := i
			for i < len(src) && isSpace(src[i]) {
				i++
			}
			out = append(out, Token{Kind: WhitespaceToken, Text: " ", Offset: start})
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			if i+1 < len(src) {
				i += 2
			} else {
				i = len(src)
			}
		case c == '"' || c == '\'':
			start := i
			quote := c
			i++
			for i < len(src) && src[i] != quote {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				i++
			}
			if i < len(src) {
				i++
			}
			out = append(out, Token{Kind: StringToken, Text: string(src[start:i]), Offset: start})
		case c == '#':
			start := i
			i++
			for i < len(src) && isNameChar(src[i]) {
				i++
			}
			out = append(out, Token{Kind: HashToken, Text: string(src[start:i]), Offset: start})
		case c == '@':
			start := i
			i++
			for i < len(src) && isNameChar(src[i]) {
				i++
			}
			out = append(out, Token{Kind: AtKeywordToken, Text: string(src[start:i]), Offset: start})
		case isDigit(c) || (c == '.' && i+1 < len(src) && isDigit(src[i+1])) || ((c == '+' || c == '-') && i+1 < len(src) && (isDigit(src[i+1]) || src[i+1] == '.')):
			out = append(out, tokenizeNumber(src, &i))
		case isNameStart(c) || c == '-' || c == '_':
			start := i
			i++
			for i < len(src) && isNameChar(src[i]) {
				i++
			}
			name := string(src[start:i])
			// url(...) is a single token in CSS; parsing it here keeps paths
			// with slashes and dots intact.
			if strings.EqualFold(name, "url") && i < len(src) && src[i] == '(' {
				i++
				for i < len(src) && isSpace(src[i]) {
					i++
				}
				vStart := i
				for i < len(src) && src[i] != ')' {
					i++
				}
				raw := strings.TrimSpace(string(src[vStart:i]))
				if i < len(src) {
					i++
				}
				out = append(out, Token{Kind: URLToken, Text: raw, Offset: start})
				continue
			}
			out = append(out, Token{Kind: IdentToken, Text: name, Offset: start})
		default:
			kind := DelimToken
			switch c {
			case ':':
				kind = ColonToken
			case ';':
				kind = SemicolonToken
			case ',':
				kind = CommaToken
			case '{':
				kind = LBraceToken
			case '}':
				kind = RBraceToken
			case '(':
				kind = LParenToken
			case ')':
				kind = RParenToken
			case '[':
				kind = LBracketToken
			case ']':
				kind = RBracketToken
			}
			out = append(out, Token{Kind: kind, Text: string(c), Offset: i})
			i++
		}
	}
	out = append(out, Token{Kind: EOFToken, Offset: len(src)})
	return out
}

func tokenizeNumber(src []byte, i *int) Token {
	start := *i
	j := *i
	if j < len(src) && (src[j] == '+' || src[j] == '-') {
		j++
	}
	for j < len(src) && isDigit(src[j]) {
		j++
	}
	if j < len(src) && src[j] == '.' {
		j++
		for j < len(src) && isDigit(src[j]) {
			j++
		}
	}
	if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
		k := j + 1
		if k < len(src) && (src[k] == '+' || src[k] == '-') {
			k++
		}
		if k < len(src) && isDigit(src[k]) {
			j = k
			for j < len(src) && isDigit(src[j]) {
				j++
			}
		}
	}
	numText := string(src[start:j])
	num, err := strconv.ParseFloat(numText, 64)
	bad := err != nil
	if bad {
		num = 0
	}
	if j < len(src) && src[j] == '%' {
		j++
		*i = j
		return Token{Kind: PercentageToken, Text: string(src[start:j]), Num: num, Offset: start, Bad: bad}
	}
	if j < len(src) && isNameStart(src[j]) {
		unitStart := j
		for j < len(src) && isNameChar(src[j]) {
			j++
		}
		*i = j
		return Token{Kind: DimensionToken, Text: string(src[start:j]), Num: num, Unit: strings.ToLower(string(src[unitStart:j])), Offset: start, Bad: bad}
	}
	*i = j
	return Token{Kind: NumberToken, Text: numText, Num: num, Offset: start, Bad: bad}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isNameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func isNameChar(c byte) bool {
	return isNameStart(c) || isDigit(c) || c == '-'
}

// stringify reconstructs a source-like string from tokens, used for selectors
// and for messages.
func stringify(toks []Token) string {
	var b strings.Builder
	for _, t := range toks {
		if t.Kind == EOFToken {
			continue
		}
		b.WriteString(t.Text)
	}
	return strings.TrimSpace(b.String())
}

// unquote strips surrounding quotes and decodes the CSS escapes inside them.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return decodeEscapes(s[1 : len(s)-1])
	}
	return s
}

// decodeEscapes decodes CSS backslash escapes: one to six hexadecimal digits
// followed by an optional whitespace, a backslash-newline continuation, or a
// backslash before any other character, which stands for that character. The
// tokenizer keeps the raw spelling, so values decode here before they reach the
// renderer.
func decodeEscapes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case '\n':
			// An escaped newline is a continuation: it produces nothing.
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
		case '\f':
		default:
			if isHexDigit(s[i]) {
				v := 0
				n := 0
				for n < 6 && i+n < len(s) && isHexDigit(s[i+n]) {
					v = v*16 + hexValue(s[i+n])
					n++
				}
				i += n - 1
				// One whitespace after the digits is part of the escape.
				if i+1 < len(s) && isSpace(s[i+1]) {
					if s[i+1] == '\r' && i+2 < len(s) && s[i+2] == '\n' {
						i++
					}
					i++
				}
				switch {
				case v == 0 || v > unicode.MaxRune:
					b.WriteRune(unicode.ReplacementChar)
				default:
					b.WriteRune(rune(v))
				}
				continue
			}
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}
