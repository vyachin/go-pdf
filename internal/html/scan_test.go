package html

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestScanSpansPartitionInput guards the invariant the position alignment rests
// on: the recorded ranges cover the document byte for byte, in order, with no
// gaps and no overlaps — including a truncated tag at the end of the input.
func TestScanSpansPartitionInput(t *testing.T) {
	cases := []struct{ name, src string }{
		{"empty", ""},
		{"one paragraph", "<p>x</p>"},
		{"two paragraphs", "<p>x</p>\n<p>y</p>"},
		{"unterminated tag", "<div"},
		{"stray less than", "a<b"},
		{"raw text", "<script>if (a<b) {}</script>"},
		{"indented table", "<table>\n  <tr>\n    <td>x</td>\n  </tr>\n</table>"},
		{"references", "&amp;&rarr;&#147;"},
		{"comment", "<!-- c -->x"},
		{"repeated", strings.Repeat("<p>a</p>", 10)},
		{"crlf", "<p>a\r\nb</p>\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc, err := scan([]byte(tc.src), ParseOptions{})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			off := 0
			for i, s := range sc.spans {
				if int(s.start) != off {
					t.Fatalf("span %d starts at %d, want %d: the ranges must be contiguous", i, s.start, off)
				}
				if int(s.end) < int(s.start) {
					t.Fatalf("span %d ends before it starts: %+v", i, s)
				}
				off = int(s.end)
			}
			if off != len(tc.src) {
				t.Fatalf("spans cover %d of %d bytes", off, len(tc.src))
			}
		})
	}
}

// TestScanRecordsDecodedText covers the token-pass decode: a token that decodes
// to different bytes names an interned entry, while a raw-text token keeps its
// literal ampersand and stores nothing.
func TestScanRecordsDecodedText(t *testing.T) {
	src := []byte("plain &amp; x\r\n<style>a&amp;b</style>")
	sc, err := scan(src, ParseOptions{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var texts []span
	for _, s := range sc.spans {
		if s.kind == spanText {
			texts = append(texts, s)
		}
	}
	if len(texts) != 2 {
		t.Fatalf("text spans = %d, want 2", len(texts))
	}
	if texts[0].name == 0 {
		t.Fatal("the entity text has no decoded entry")
	}
	if got, want := sc.textOf(src, texts[0]), "plain & x\n"; got != want {
		t.Fatalf("decoded text = %q, want %q", got, want)
	}
	if texts[1].name != 0 {
		t.Fatalf("the raw-text token was decoded to %q", sc.textOf(src, texts[1]))
	}
	if got, want := sc.textOf(src, texts[1]), "a&amp;b"; got != want {
		t.Fatalf("raw text = %q, want %q", got, want)
	}
}

// TestScanLeavesPlainTextUnindexed covers the cheap path: a text token with no
// trigger byte keeps a zero id and compares as its raw slice.
func TestScanLeavesPlainTextUnindexed(t *testing.T) {
	src := []byte("<p>plain</p><p>&amp;</p>")
	sc, err := scan(src, ParseOptions{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	ids := map[string]int32{}
	for _, s := range sc.spans {
		if s.kind == spanText {
			ids[sc.textOf(src, s)] = s.name
		}
	}
	if got, ok := ids["plain"]; !ok || got != 0 {
		t.Fatalf("plain text id = %d (present %v), want 0", got, ok)
	}
	if got, ok := ids["&"]; !ok || got == 0 {
		t.Fatalf("reference text id = %d (present %v), want a decoded entry", got, ok)
	}
}

// TestScanChargesOneUnitPerSpan pins the work unit: one budget unit per recorded
// span, so an operation budget keeps meaning what it meant before the change.
func TestScanChargesOneUnitPerSpan(t *testing.T) {
	src := []byte("<p>a</p><ul><li>x</li></ul>")
	sc, err := scan(src, ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	n := len(sc.spans)
	if n == 0 {
		t.Fatal("no spans recorded")
	}
	exact := budget.New(context.Background(), n)
	if _, err := scan(src, ParseOptions{Budget: exact}); err != nil {
		t.Fatalf("a budget of %d units failed: %v", n, err)
	}
	if got := exact.Used(); got != uint64(n) {
		t.Fatalf("scan spent %d units for %d spans, want one per span", got, n)
	}
	short := budget.New(context.Background(), n-1)
	if _, err := scan(src, ParseOptions{Budget: short}); !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("a budget of %d units returned %v, want a limit error", n-1, err)
	}
}

// TestScanDepthRejectedBeforeTree guards the ordering requirement: the nesting
// limit stops the token pass, so a deeply nested document never reaches the tree
// construction that would allocate one node per level.
func TestScanDepthRejectedBeforeTree(t *testing.T) {
	const levels = 40
	src := []byte(strings.Repeat("<div>", levels))
	guard := budget.New(context.Background(), 10000)
	_, err := ParseWithOptions(src, ParseOptions{MaxDepth: 8, Budget: guard})
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("deep nesting returned %v, want a limit error", err)
	}
	spent := guard.Used()
	if spent >= levels {
		t.Fatalf("the depth limit was decided after %d tokens, want fewer than the %d tokens the document holds", spent, levels)
	}
}
