package html

import (
	"fmt"
	"strings"
	"testing"
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
