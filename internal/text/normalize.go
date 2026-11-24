package text

import "strings"

// Hangul composition constants from the Unicode standard, section 3.12.
const (
	hangulSBase  = 0xAC00
	hangulLBase  = 0x1100
	hangulVBase  = 0x1161
	hangulTBase  = 0x11A7
	hangulLCount = 19
	hangulVCount = 21
	hangulTCount = 28
	hangulNCount = hangulVCount * hangulTCount
	hangulSCount = hangulLCount * hangulNCount
)

// Normalize applies canonical composition (Unicode NFC, composition only): a
// base character followed by combining marks becomes the precomposed character
// where Unicode defines one, so the text renders as one glyph and extracts back
// as one code point. Marks that cannot be composed are kept, and the caller can
// report them with HasCombiningMark.
func Normalize(s string) string {
	if !hasCombiningMark(s) {
		return s
	}
	runes := []rune(s)
	// Canonical ordering: inside a sequence of marks, a mark with a lower
	// combining class must come first.
	for i := 1; i < len(runes); i++ {
		c := combiningClass(runes[i])
		if c == 0 {
			continue
		}
		for j := i; j > 0; j-- {
			prev := combiningClass(runes[j-1])
			if prev == 0 || prev <= c {
				break
			}
			runes[j-1], runes[j] = runes[j], runes[j-1]
		}
	}
	out := make([]rune, 0, len(runes))
	starterPos := -1
	prevClass := uint8(0)
	for _, r := range runes {
		c := combiningClass(r)
		if starterPos >= 0 && (c == 0 && prevClass == 0 || c != 0 && prevClass < c) {
			if composed, ok := composePair(out[starterPos], r); ok {
				out[starterPos] = composed
				continue
			}
		}
		if c == 0 {
			starterPos = len(out)
		}
		prevClass = c
		out = append(out, r)
	}
	return string(out)
}

// composePair returns the precomposed character for a starter and a following
// character, or false when Unicode defines no composition for them.
func composePair(starter, mark rune) (rune, bool) {
	// Hangul syllables compose algorithmically: L+V -> LV, LV+T -> LVT.
	if starter >= hangulLBase && starter < hangulLBase+hangulLCount &&
		mark >= hangulVBase && mark < hangulVBase+hangulVCount {
		return hangulSBase + ((starter-hangulLBase)*hangulVCount+(mark-hangulVBase))*hangulTCount, true
	}
	if starter >= hangulSBase && starter < hangulSBase+hangulSCount &&
		(starter-hangulSBase)%hangulTCount == 0 &&
		mark > hangulTBase && mark < hangulTBase+hangulTCount {
		return starter + (mark - hangulTBase), true
	}
	composed, ok := compositionPairs[[2]rune{starter, mark}]
	return composed, ok
}

func combiningClass(r rune) uint8 {
	if r < 0x0300 {
		return 0
	}
	return combiningClasses[r]
}

func hasCombiningMark(s string) bool {
	for _, r := range s {
		if combiningClass(r) != 0 {
			return true
		}
		if r >= hangulLBase && r < hangulLBase+hangulLCount {
			return true
		}
	}
	return false
}

// HasCombiningMark reports whether the string still contains a combining mark
// after normalization, that is a mark Unicode cannot compose with its base.
func HasCombiningMark(s string) bool {
	for _, r := range Normalize(s) {
		if combiningClass(r) != 0 {
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
