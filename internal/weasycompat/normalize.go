// Package weasycompat runs the WeasyPrint sample documents through this engine
// and checks each of them against the state its manifest declares.
//
// The corpus is third-party material: full documents written for a
// browser-grade paged-media engine, each shipped with the PDF that engine
// produced. A sample that this profile rejects must be rejected with the
// committed diagnostic code; a sample that converts is frozen page by page and
// measured against the reference text. The corpus is committed, so the suite
// runs offline and without poppler.
package weasycompat

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// normalize folds a text layer into the form the fidelity comparison sees:
// NFKC, space-like characters folded to one space, and the invisible characters
// a line-breaking engine may insert removed. Punctuation and case survive: a
// lost comma or an unapplied text-transform is a defect, and tokens() drops
// punctuation explicitly rather than this function hiding it.
func normalize(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		switch r {
		case '\u00ad', // soft hyphen: a break point, never content
			'\u200b', '\u200c', '\u200d', // zero-width space, non-joiner, joiner
			'\u2060', // word joiner
			'\ufeff': // byte-order mark
			continue
		}
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return b.String()
}

// tokens returns the tokens of a text in order, with duplicates: a token is a
// maximal run of Unicode letters or digits after normalization. Order is kept
// here for the per-page goldens; the oracle compares the multiset, because two
// engines break lines differently without either being wrong.
func tokens(s string) []string {
	var out []string
	var run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			out = append(out, run.String())
			run.Reset()
		}
	}
	for _, r := range normalize(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			run.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// tokenCounts counts a token multiset.
func tokenCounts(toks []string) map[string]int {
	counts := make(map[string]int, len(toks))
	for _, tok := range toks {
		counts[tok]++
	}
	return counts
}

// countDiff lists the tokens that want has and got does not, and the ones got
// has more of than want, as "token" or "token xN" in sorted order. A token that
// appears more often in want than in got is reported as missing, not as a
// separate category: either way the counts disagree.
func countDiff(want, got map[string]int) (missing, extra []string) {
	seen := make(map[string]bool, len(want)+len(got))
	for tok := range want {
		seen[tok] = true
	}
	for tok := range got {
		seen[tok] = true
	}
	all := make([]string, 0, len(seen))
	for tok := range seen {
		all = append(all, tok)
	}
	slices.Sort(all)
	for _, tok := range all {
		switch {
		case got[tok] < want[tok]:
			missing = append(missing, formatCount(tok, want[tok]-got[tok]))
		case got[tok] > want[tok]:
			extra = append(extra, formatCount(tok, got[tok]-want[tok]))
		}
	}
	return missing, extra
}

func formatCount(tok string, n int) string {
	if n == 1 {
		return tok
	}
	return tok + " x" + strconv.Itoa(n)
}
