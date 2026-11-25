package text

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Normalize applies Unicode canonical composition (NFC). A base character
// followed by combining marks becomes the precomposed character where Unicode
// defines one, so the text renders as one glyph and extracts back as one code
// point. Marks that cannot be composed are kept.
//
// Canonical decomposition runs first, so a sequence that only composes after it
// — a precomposed letter followed by a mark, or a compatibility ideograph —
// also reaches its canonical form. The standard library's implementation is
// used rather than a private table, so the result tracks the Unicode version of
// golang.org/x/text.
func Normalize(s string) string {
	return norm.NFC.String(s)
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
			case '\u00ad':
				// A soft hyphen is not drawn; it only allows a break.
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
		case r == '\u00ad':
			// A soft hyphen allows a break after the current word but is never
			// drawn: rendering its glyph would show a hyphen inside a word that
			// fits on the line.
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
