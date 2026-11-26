package html

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// BenchmarkParseEntityFragments reports the scaling of a document whose text is
// nothing but character references: the case that used to fall through to a full
// scan of every span for every node.
func BenchmarkParseEntityFragments(b *testing.B) {
	for _, n := range []int{16_000, 32_000} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			var src strings.Builder
			for i := 0; i < n; i++ {
				src.WriteString("<p>&amp;</p>")
			}
			doc := []byte(src.String())
			b.SetBytes(int64(len(doc)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Parse(doc, "bench.html"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPositionSingleLine measures position lookup on a 112 KB UTF-8 line
// without a byte order mark. The cursor scans each byte once, so 8,000 queries
// over the line cost about as much as 1,000 queries over the same line instead
// of a rescan from the line start for every query.
func BenchmarkPositionSingleLine(b *testing.B) {
	const lineBytes = 112 * 1024
	var src strings.Builder
	src.WriteString("<p>")
	for src.Len() < lineBytes {
		src.WriteString("Счёт ")
	}
	src.WriteString("</p>")
	doc, err := Parse([]byte(src.String()), "one-line.html")
	if err != nil {
		b.Fatal(err)
	}
	if doc.mapper != nil {
		b.Fatal("UTF-8 without a byte order mark must not use the mapper")
	}
	var boundaries []int
	for off := 0; off < len(doc.text); {
		boundaries = append(boundaries, off)
		_, size := utf8.DecodeRune(doc.text[off:])
		off += size
	}
	boundaries = append(boundaries, len(doc.text))

	for _, queries := range []int{1_000, 8_000} {
		b.Run(fmt.Sprintf("queries=%d", queries), func(b *testing.B) {
			offsets := make([]int, queries)
			for i := range offsets {
				offsets[i] = boundaries[(i+1)*len(boundaries)/(queries+1)]
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Start each pass from the line start, so the measured work is
				// one linear scan of the line plus the per-query overhead.
				doc.Position(0)
				for _, off := range offsets {
					doc.Position(off)
				}
			}
		})
	}
}

// BenchmarkPathManyClasses measures diagnostic path building for one element
// with many class tokens. A builder plus a per-byte charge keeps the cost
// linear: the 40,000-class case should cost about forty times the 1,000-class
// one, not grow with the square of the token count.
func BenchmarkPathManyClasses(b *testing.B) {
	for _, count := range []int{1_000, 40_000} {
		b.Run(fmt.Sprintf("classes=%d", count), func(b *testing.B) {
			classes := make([]string, count)
			for i := range classes {
				classes[i] = "c" + strconv.Itoa(i)
			}
			root := &Node{Type: DocumentNode}
			body := &Node{Type: ElementNode, Tag: "body", Parent: root}
			div := &Node{
				Type:  ElementNode,
				Tag:   "div",
				Attrs: []Attr{{Key: "class", Value: strings.Join(classes, " ")}},
			}
			root.Children = []*Node{body}
			body.Children = []*Node{div}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if p := div.Path(); p == "" {
					b.Fatal("empty path")
				}
			}
		})
	}
}
