package profile

import (
	"context"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/html"
)

// TestValidatePositionQuadratic guards the minified-document case: 8,000
// validation diagnostics on one line must fit a budget sized for the multiline
// equivalent plus one linear pass over the line. The multiline run measures the
// work of the same element tree when every position starts its own line; the
// one-line run may add only the bytes its single line is scanned for. A scan
// from the line start for every position charges far more than one pass, so the
// charged-work assertion below fails for the quadratic implementation.
func TestValidatePositionQuadratic(t *testing.T) {
	const (
		count   = 8_000
		element = `<p data-x>t</p>`
	)
	var oneLine, multiline strings.Builder
	for i := 0; i < count; i++ {
		oneLine.WriteString(element)
		multiline.WriteString(element)
		multiline.WriteByte('\n')
	}
	srcOne, srcMulti := []byte(oneLine.String()), []byte(multiline.String())

	// The multiline equivalent: same elements, one per line, so every position
	// queried during validation starts its line and costs no extra scan.
	multiGuard := budget.New(context.Background(), 0)
	multiDoc, err := html.ParseWithOptions(srcMulti, html.ParseOptions{File: "multi.html", Budget: multiGuard})
	if err != nil {
		t.Fatal(err)
	}
	multiDiags := validateDoc(t, multiDoc, Options{Budget: multiGuard})
	if len(multiDiags) != count {
		t.Fatalf("multiline diagnostics = %d, want %d", len(multiDiags), count)
	}
	for i, d := range multiDiags {
		if d.Code != "html/unsupported-attribute" {
			t.Fatalf("multiline diagnostic %d = %s, want html/unsupported-attribute", i, d.String())
		}
		if d.Pos.Line != i+1 || d.Pos.Col != 1 {
			t.Fatalf("multiline diagnostic %d at %d:%d, want %d:1", i, d.Pos.Line, d.Pos.Col, i+1)
		}
	}

	// The one line is scanned once, so its budget is the multiline spend plus
	// its own length.
	oneGuard := budget.New(context.Background(), int(multiGuard.Used())+len(srcOne))
	oneDoc, err := html.ParseWithOptions(srcOne, html.ParseOptions{File: "one.html", Budget: oneGuard})
	if err != nil {
		t.Fatal(err)
	}
	oneDiags := validateDoc(t, oneDoc, Options{Budget: oneGuard})
	if err := oneGuard.Err(); err != nil {
		t.Fatalf("one-line validation did not fit the multiline budget: %v", err)
	}
	if len(oneDiags) != count {
		t.Fatalf("one-line diagnostics = %d, want %d", len(oneDiags), count)
	}
	lastOffset := (count - 1) * len(element)
	if got, want := oneGuard.Used(), uint64(lastOffset); got < want {
		t.Fatalf("one-line positions charged %d units, want at least the %d bytes of the line they scan", got, want)
	}
	for i, d := range oneDiags {
		if d.Code != multiDiags[i].Code || d.Message != multiDiags[i].Message || d.Element != multiDiags[i].Element {
			t.Fatalf("one-line diagnostic %d = %s, multiline = %s", i, d.String(), multiDiags[i].String())
		}
		if want := i*len(element) + 1; d.Pos.Line != 1 || d.Pos.Col != want {
			t.Fatalf("one-line diagnostic %d at %d:%d, want 1:%d", i, d.Pos.Line, d.Pos.Col, want)
		}
	}
}
