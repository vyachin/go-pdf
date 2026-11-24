package text

import "strings"

// compositions maps a base rune plus a combining mark to its precomposed form.
// The table covers Cyrillic and the common Latin diacritics that appear in
// business documents; anything else is left untouched and will be reported by
// shaping if the font has no glyph for it.
var compositions = map[[2]rune]rune{
	// Cyrillic
	{'и', '\u0306'}: 'й',
	{'И', '\u0306'}: 'Й',
	{'е', '\u0308'}: 'ё',
	{'Е', '\u0308'}: 'Ё',
	{'і', '\u0308'}: 'ї',
	{'І', '\u0308'}: 'Ї',
	{'у', '\u0306'}: 'ў',
	{'У', '\u0306'}: 'Ў',
	{'г', '\u0301'}: 'ѓ',
	{'к', '\u0301'}: 'ќ',
	// Latin lowercase
	{'a', '\u0300'}: 'à', {'a', '\u0301'}: 'á', {'a', '\u0302'}: 'â',
	{'a', '\u0303'}: 'ã', {'a', '\u0308'}: 'ä', {'a', '\u030a'}: 'å',
	{'c', '\u0327'}: 'ç',
	{'e', '\u0300'}: 'è', {'e', '\u0301'}: 'é', {'e', '\u0302'}: 'ê', {'e', '\u0308'}: 'ë',
	{'i', '\u0300'}: 'ì', {'i', '\u0301'}: 'í', {'i', '\u0302'}: 'î', {'i', '\u0308'}: 'ï',
	{'n', '\u0303'}: 'ñ',
	{'o', '\u0300'}: 'ò', {'o', '\u0301'}: 'ó', {'o', '\u0302'}: 'ô',
	{'o', '\u0303'}: 'õ', {'o', '\u0308'}: 'ö',
	{'u', '\u0300'}: 'ù', {'u', '\u0301'}: 'ú', {'u', '\u0302'}: 'û', {'u', '\u0308'}: 'ü',
	{'y', '\u0301'}: 'ý', {'y', '\u0308'}: 'ÿ',
	// Latin uppercase
	{'A', '\u0300'}: 'À', {'A', '\u0301'}: 'Á', {'A', '\u0302'}: 'Â',
	{'A', '\u0303'}: 'Ã', {'A', '\u0308'}: 'Ä', {'A', '\u030a'}: 'Å',
	{'C', '\u0327'}: 'Ç',
	{'E', '\u0300'}: 'È', {'E', '\u0301'}: 'É', {'E', '\u0302'}: 'Ê', {'E', '\u0308'}: 'Ë',
	{'I', '\u0300'}: 'Ì', {'I', '\u0301'}: 'Í', {'I', '\u0302'}: 'Î', {'I', '\u0308'}: 'Ï',
	{'N', '\u0303'}: 'Ñ',
	{'O', '\u0300'}: 'Ò', {'O', '\u0301'}: 'Ó', {'O', '\u0302'}: 'Ô',
	{'O', '\u0303'}: 'Õ', {'O', '\u0308'}: 'Ö',
	{'U', '\u0300'}: 'Ù', {'U', '\u0301'}: 'Ú', {'U', '\u0302'}: 'Û', {'U', '\u0308'}: 'Ü',
	{'Y', '\u0301'}: 'Ý',
}

// Normalize composes combining marks so that a decomposed string renders as
// precomposed glyphs and extracts back as single characters.
func Normalize(s string) string {
	if !hasCombiningMark(s) {
		return s
	}
	runes := []rune(s)
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if i+1 < len(runes) {
			if composed, ok := compositions[[2]rune{runes[i], runes[i+1]}]; ok {
				out = append(out, composed)
				i++
				continue
			}
		}
		out = append(out, runes[i])
	}
	return string(out)
}

func hasCombiningMark(s string) bool {
	for _, r := range s {
		if r >= '\u0300' && r <= '\u036F' {
			return true
		}
	}
	return false
}

// HasCombiningMark reports whether the string contains a combining mark that
// the normalisation table cannot compose.
func HasCombiningMark(s string) bool {
	for _, r := range Normalize(s) {
		if r >= '\u0300' && r <= '\u036F' {
			return true
		}
	}
	return false
}

// isWordSpace reports whether r separates words.
func isWordSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// ItemKind classifies a breakable piece of text.
type ItemKind int

const (
	// Word is an unbreakable run of characters.
	Word ItemKind = iota
	// Space is a breakable word separator.
	Space
	// Newline is a forced line break in preformatted text.
	Newline
)

// Item is one piece of a text flow.
type Item struct {
	Kind ItemKind
	Text string
	// BreakAfter marks a word that already ends with a visible hyphen, so a
	// line may break after it without adding or dropping a character.
	BreakAfter bool
}

// Split breaks text into words and spaces. Whitespace collapses unless pre is
// true. A non-breaking space stays inside the word, and a hyphen ends a word so
// that a break is allowed after it.
func Split(s string, pre bool) []Item {
	if pre {
		var items []Item
		var b strings.Builder
		flush := func() {
			if b.Len() > 0 {
				items = append(items, Item{Kind: Word, Text: b.String()})
				b.Reset()
			}
		}
		runes := []rune(s)
		for i := 0; i < len(runes); i++ {
			switch r := runes[i]; r {
			case '\r':
				flush()
				if i+1 < len(runes) && runes[i+1] == '\n' {
					i++
				}
				items = append(items, Item{Kind: Newline})
			case '\n':
				flush()
				items = append(items, Item{Kind: Newline})
			case '\t', ' ':
				// Preformatted spaces are preserved one by one; a tab counts
				// as a space because the profile has no tab stops.
				flush()
				items = append(items, Item{Kind: Space, Text: " "})
			default:
				b.WriteRune(r)
			}
		}
		flush()
		return items
	}
	var items []Item
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			items = append(items, Item{Kind: Word, Text: b.String()})
			b.Reset()
		}
	}
	for _, r := range s {
		switch {
		case isWordSpace(r):
			flush()
			if len(items) > 0 && items[len(items)-1].Kind == Space {
				continue
			}
			items = append(items, Item{Kind: Space, Text: " "})
		case r == '\u00a0':
			b.WriteRune(' ')
		case r == '-' || r == '\u2010':
			// Keep the character as written: U+2010 is a different glyph with a
			// different advance in some fonts, and the text layer must copy it.
			b.WriteRune(r)
			flush()
			if n := len(items); n > 0 {
				items[n-1].BreakAfter = true
			}
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return items
}
