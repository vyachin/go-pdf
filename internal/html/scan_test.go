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
