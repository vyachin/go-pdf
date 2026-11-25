package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// TestNonBreakingSpaceSurvivesExtraction guards RV4-31 end to end: the text
// layer carries U+00A0 between the letters and the line is not broken there.
func TestNonBreakingSpaceSurvivesExtraction(t *testing.T) {
	src := []byte(`<p style="width:60pt">aaaa&nbsp;bbbb cccc</p>`)
	res, err := Convert(context.Background(), src, Options{File: "nbsp.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "aaaa\u00a0bbbb") {
		t.Fatalf("the extracted text lost the non-breaking space: %q", text)
	}
	if strings.Contains(text, "aaaa bbbb") {
		t.Fatalf("the line broke at the non-breaking space: %q", text)
	}
}

// TestFosteredStyleIsNotParsedAsCSS guards the behaviour half of RV4-10: the
// stylesheet of a <style> inside a table is its own CSS, and the fostered text
// beside it is body text, not a rule.
func TestFosteredStyleIsNotParsedAsCSS(t *testing.T) {
	src := []byte(`<table><style>p{color:red}</style>p{color:blue}<tr><td>y</td></tr></table><p>after</p>`)
	res, err := Convert(context.Background(), src, Options{File: "foster.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("the document reported %v", res.Diagnostics.Lines())
	}
	var streams strings.Builder
	for _, stream := range pdfdebug.ContentStreams(res.PDF) {
		streams.Write(stream)
		streams.WriteByte('\n')
	}
	if !strings.Contains(streams.String(), "1 0 0 rg") {
		t.Fatal("the red rule was not applied")
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "p{color:blue}") {
		t.Fatalf("the fostered text is not body text: %q", text)
	}
}

// TestDeclaredWindows1252OverUTF8 guards RV4-17: a document that declares
// windows-1252 fails even when its bytes happen to be valid UTF-8, while the
// no-declaration fallback still accepts a UTF-8 document. No PDF is produced.
func TestDeclaredWindows1252OverUTF8(t *testing.T) {
	src := []byte(`<meta charset="windows-1252"><p>Привет</p>`)
	res, err := Convert(context.Background(), src, Options{File: "cp1252.html", FallbackFont: testFontPath(t)})
	if err == nil || len(res.PDF) != 0 {
		t.Fatalf("a declared windows-1252 was accepted: err=%v pdf=%d", err, len(res.PDF))
	}
	var found bool
	for _, d := range res.Diagnostics {
		if d.Code == "html/encoding" && strings.Contains(d.Message, "windows-1252") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want html/encoding naming windows-1252, got:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	// The same bytes without the declaration are valid UTF-8 and convert.
	plain := []byte(`<p>Привет</p>`)
	if _, err := Convert(context.Background(), plain, Options{File: "utf8.html", FallbackFont: testFontPath(t)}); err != nil {
		t.Fatalf("a UTF-8 document without a declaration failed: %v", err)
	}
}

// TestMediaAttributeOnStyle guards RV4-18: media on a <style> element works in
// collection but was reported as an unsupported attribute, and Strict turned
// that warning into an error. The attribute is part of the profile now, and the
// print-only sheet still applies.
func TestMediaAttributeOnStyle(t *testing.T) {
	src := []byte(`<style media="print">p{color:#00ff00}</style><p>x</p>`)
	res, err := Convert(context.Background(), src, Options{
		File:         "media.html",
		FallbackFont: testFontPath(t),
		Strict:       true,
	})
	if err != nil {
		t.Fatalf("a print-only style element failed under Strict: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if hasCode(res.Diagnostics, "html/unsupported-attribute") {
		t.Fatalf("media was reported as an unsupported attribute:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	var streams strings.Builder
	for _, stream := range pdfdebug.ContentStreams(res.PDF) {
		streams.Write(stream)
	}
	if !strings.Contains(streams.String(), "0 1 0 rg") {
		t.Fatal("the print-only rule was not applied")
	}
}

// TestPseudoElementBeforeDescendantIsRejected guards the rendering half of
// RV4-19: the invalid selector must be reported and must not style the span.
func TestPseudoElementBeforeDescendantIsRejected(t *testing.T) {
	src := []byte(`<style>a::after span { color:#ff0000 }</style><a href="#x">a<span>s</span></a>`)
	res, err := Convert(context.Background(), src, Options{File: "pseudo.html", FallbackFont: testFontPath(t)})
	if err == nil {
		t.Fatal("the invalid selector was accepted")
	}
	if !hasCode(res.Diagnostics, "css/unsupported-selector") {
		t.Fatalf("want css/unsupported-selector, got:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}

// TestMediaListAppliesForPrint guards RV4-49: validation rejected
// "@media print, screen" while both collection paths accepted a list, so a rule
// that applies was thrown away. One media-list parser now answers both.
func TestMediaListAppliesForPrint(t *testing.T) {
	src := []byte(`<style>@media print, screen { p { color: #ff0000 } }</style><p>x</p>`)
	res, err := Convert(context.Background(), src, Options{File: "list.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatalf("the media list was rejected: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if hasCode(res.Diagnostics, "css/unsupported-media") {
		t.Fatalf("the list was reported as unsupported:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	var streams strings.Builder
	for _, stream := range pdfdebug.ContentStreams(res.PDF) {
		streams.Write(stream)
	}
	if !strings.Contains(streams.String(), "1 0 0 rg") {
		t.Fatal("the rule did not apply to the print layout")
	}
}

// TestTinyFontSizeFailsValidation guards RV4-50: a percentage font size that
// rounds to zero against the resolved parent size was accepted by validation,
// then silently dropped by the cascade, so the paragraph rendered at its
// parent's size. The declaration is reported instead.
func TestTinyFontSizeFailsValidation(t *testing.T) {
	src := []byte(`<div style="font-size:1pt"><p style="font-size:0.01%">x</p></div>`)
	list, err := Validate(context.Background(), src, Options{File: "tiny.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range list {
		if d.Code == "css/unsupported-value" && strings.Contains(d.Message, "font-size") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no css/unsupported-value for the tiny font size:\n%s", strings.Join(list.Lines(), "\n"))
	}
}

// TestLongTokenSplitsTJArrays guards RV4-51: a 20,000-character token emitted
// one string per glyph in a single TJ array of 24,000 elements. Contiguous
// glyphs now share one string and the array is split at 8,191 elements without
// changing the glyph positions.
func TestLongTokenSplitsTJArrays(t *testing.T) {
	const n = 20000
	src := []byte(`<p style="width:5000pt">` + strings.Repeat("a", n) + `</p>`)
	res, err := Convert(context.Background(), src, Options{File: "tj.html", FallbackFont: testFontPath(t)})
	if err != nil {
		t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	var content strings.Builder
	for _, stream := range pdfdebug.ContentStreams(res.PDF) {
		content.Write(stream)
		content.WriteByte('\n')
	}
	body := content.String()
	arrays := 0
	for {
		i := strings.Index(body, "[<")
		if i < 0 {
			break
		}
		j := strings.Index(body[i:], "] TJ")
		if j < 0 {
			break
		}
		arrays++
		body = body[i+j+4:]
	}
	if arrays < 2 {
		t.Fatalf("the token was written in %d TJ array(s), want several", arrays)
	}
	text, err := pdfdebug.Text(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(text, "a"); got != n {
		t.Fatalf("the extracted text carries %d glyphs, want %d", got, n)
	}
}
