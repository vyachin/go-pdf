package engine

import (
	"context"
	"strings"
	"testing"
)

// BenchmarkNestedAvoidLines measures the width-keyed line cache: without it the
// same text is split once per nesting level.
func BenchmarkNestedAvoidLines(b *testing.B) {
	src := []byte(nestedAvoidDocument(120, 120))
	font := testFontPath(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Convert(context.Background(), src, Options{File: "nested.html", FallbackFont: font}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPNGEmbed measures embedding a repeated PNG, which reuses the
// loader's decode instead of decoding it again.
func BenchmarkPNGEmbed(b *testing.B) {
	src := []byte(`<p><img src="a.png" width="20pt"></p>`)
	dir := adversarialDir()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Convert(context.Background(), src, Options{File: "png.html", BaseDir: dir, FallbackFont: testFontPath(b)}); err != nil {
			b.Fatal(err)
		}
	}
}

// nestedAvoidDocument builds depth nested break-inside: avoid blocks, each with
// a paragraph of words words.
func nestedAvoidDocument(depth, words int) string {
	para := "<p>" + strings.Repeat("word ", words) + "</p>"
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString(`<div style="break-inside: avoid">`)
		b.WriteString(para)
	}
	for i := 0; i < depth; i++ {
		b.WriteString("</div>")
	}
	return b.String()
}

// BenchmarkHugeWordConvert measures the 8 MiB single-word profile. Painting
// reuses the shaped glyph slice when no glyph needs an offset, so the reported
// allocation does not include a second glyph slice per rune.
func BenchmarkHugeWordConvert(b *testing.B) {
	src := []byte("<p>" + strings.Repeat("x", 8<<20) + "</p>")
	opts := Options{
		File:         "word.html",
		FallbackFont: testFontPath(b),
		Limits:       Limits{MaxInputBytes: 16 << 20},
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := Convert(context.Background(), src, opts)
		if err != nil {
			b.Fatalf("the default budget refused the profile: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
		}
	}
}
