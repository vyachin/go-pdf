package html

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/diag"
)

// referencePosition is the previous, from-the-line-start implementation of
// Position. The cursor must return exactly the same line and column for every
// offset, whatever order the queries arrive in.
func referencePosition(d *Document, offset int) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(d.text) {
		offset = len(d.text)
	}
	i, _ := slices.BinarySearch(d.lines, offset+1)
	i--
	if i < 0 {
		i = 0
	}
	return i + 1, utf8.RuneCount(d.text[d.lines[i]:offset]) + 1
}

// TestPositionCursorMatchesFromLineStart covers the UTF-8 no-BOM path, which
// has no charset mapper: positions on one long line and across several lines
// must keep the previous line and column, for ascending, descending and
// repeated queries, including offsets that fall inside a multi-byte character.
func TestPositionCursorMatchesFromLineStart(t *testing.T) {
	sources := []string{
		"<p>" + strings.Repeat("Счёт ", 200) + "</p>",
		"<p>Счёт</p>\n<p>ещё счёт</p>\n<p>end</p>",
	}
	for _, src := range sources {
		doc, err := Parse([]byte(src), "utf8.html")
		if err != nil {
			t.Fatal(err)
		}
		if doc.mapper != nil {
			t.Fatal("UTF-8 without a byte order mark must not use the mapper")
		}
		check := func(off int) {
			t.Helper()
			got := doc.Position(off)
			line, col := referencePosition(doc, off)
			want := diag.Position{File: "utf8.html", Line: line, Col: col}
			if got != want {
				t.Fatalf("Position(%d) = %+v, want %+v", off, got, want)
			}
		}
		for off := 0; off <= len(doc.text); off++ {
			check(off)
		}
		for off := len(doc.text); off >= 0; off-- {
			check(off)
		}
		for off := 0; off <= len(doc.text); off += 7 {
			check(off)
			check(off)
		}
	}
}

// TestPositionChargesScannedBytes guards the work charge: one pass of ascending
// positions over a line costs exactly the bytes between the line start and the
// last position, and a budget below that stops the guard while every returned
// position stays exact.
func TestPositionChargesScannedBytes(t *testing.T) {
	src := []byte("<p>" + strings.Repeat("Счёт ", 2000) + "</p>")
	guard := budget.New(context.Background(), 0)
	doc, err := ParseWithOptions(src, ParseOptions{File: "utf8.html", Budget: guard})
	if err != nil {
		t.Fatal(err)
	}
	if doc.mapper != nil {
		t.Fatal("UTF-8 without a byte order mark must not use the mapper")
	}
	before := guard.Used()
	last := 0
	for off := 0; off < len(doc.text); off++ {
		if off > 0 && !utf8.RuneStart(doc.text[off]) {
			continue
		}
		doc.Position(off)
		last = off
	}
	if got, want := guard.Used()-before, uint64(last); got != want {
		t.Fatalf("positions charged %d units, want %d scanned bytes", got, want)
	}

	// The same line under a budget one unit short stops the guard, but the
	// position itself is still exact: Position has no error to return.
	short := budget.New(context.Background(), last-1)
	doc.guard = short
	doc.posValid = false
	for off := 0; off < len(doc.text); off++ {
		if off > 0 && !utf8.RuneStart(doc.text[off]) {
			continue
		}
		got := doc.Position(off)
		line, col := referencePosition(doc, off)
		if got.Line != line || got.Col != col {
			t.Fatalf("Position(%d) under a short budget = %+v, want line %d col %d", off, got, line, col)
		}
	}
	if !errors.Is(short.Err(), budget.ErrExceeded) {
		t.Fatalf("the short budget was not exhausted: %v", short.Err())
	}
}

// TestPositionChargeOnDecodedDocument guards the same charge on the mapper
// path: a legacy document charges the decoded bytes between two positions, so
// its positions are bounded by the work budget too.
func TestPositionChargeOnDecodedDocument(t *testing.T) {
	src := []byte(`<meta charset="windows-1251"><p>` + strings.Repeat("\xcf\xf0\xe8\xe2\xe5\xf2 ", 500))
	guard := budget.New(context.Background(), 0)
	doc, err := ParseWithOptions(src, ParseOptions{File: "cp1251.html", Budget: guard})
	if err != nil {
		t.Fatal(err)
	}
	if doc.mapper == nil {
		t.Fatal("a legacy document must use the mapper")
	}
	before := guard.Used()
	last := len(doc.text) - 1
	for off := 0; off <= last; off++ {
		doc.Position(off)
	}
	if got, want := guard.Used()-before, uint64(last); got != want {
		t.Fatalf("decoded positions charged %d units, want %d scanned bytes", got, want)
	}
}

// TestPathManyClasses guards that path building is linear in the bytes it
// writes: a forty-thousand-class element still produces the full path, built
// with one pass over its length.
func TestPathManyClasses(t *testing.T) {
	const count = 40_000
	classes := make([]string, count)
	var want strings.Builder
	want.WriteString("body > div")
	for i := range classes {
		classes[i] = "c" + strconv.Itoa(i)
		want.WriteString(".")
		want.WriteString(classes[i])
	}
	root := &Node{Type: DocumentNode}
	body := &Node{Type: ElementNode, Tag: "body", Parent: root}
	div := &Node{
		Type:   ElementNode,
		Tag:    "div",
		Parent: body,
		Attrs:  []Attr{{Key: "class", Value: strings.Join(classes, " ")}},
	}
	root.Children = []*Node{body}
	body.Children = []*Node{div}

	got := div.Path()
	if got != want.String() {
		t.Fatalf("path has %d bytes, want %d", len(got), want.Len())
	}
}

// TestClassTokensSplitOnASCIIWhitespace covers the class token rule: only
// ASCII whitespace separates tokens, so a no-break space stays inside its
// token.
func TestClassTokensSplitOnASCIIWhitespace(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{`<div class="a&#160;b"></div>`, []string{"a\u00a0b"}},
		{`<div class="a&#160; b"></div>`, []string{"a\u00a0", "b"}},
		{"<div class=\"a \t b\nc\"></div>", []string{"a", "b", "c"}},
		{`<div class="a &#9; b&#10;c&#12;d&#13;e"></div>`, []string{"a", "b", "c", "d", "e"}},
		{`<div class=""></div>`, nil},
		{`<div class="  &#9; "></div>`, nil},
		{`<div></div>`, nil},
	}
	for _, tc := range cases {
		doc, err := Parse([]byte(tc.src), "class.html")
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		div := findElement(doc.Root, "div")
		if div == nil {
			t.Fatalf("%q: no div", tc.src)
		}
		got := div.Class()
		if len(got) != len(tc.want) {
			t.Fatalf("%q: Class() = %q, want %q", tc.src, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%q: Class()[%d] = %q, want %q", tc.src, i, got[i], tc.want[i])
			}
		}
	}
}

// TestClassCacheIsShared guards that attributes are split once: repeated
// calls return the same slice instead of splitting the attribute again.
func TestClassCacheIsShared(t *testing.T) {
	doc, err := Parse([]byte(`<div class="a b c">x</div>`), "class.html")
	if err != nil {
		t.Fatal(err)
	}
	div := findElement(doc.Root, "div")
	first := div.Class()
	second := div.Class()
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("Class() = %q and %q, want three tokens", first, second)
	}
	if &first[0] != &second[0] {
		t.Fatal("the class attribute was split again on the second call")
	}
}
