package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

// TestCreationDateRange guards the date option: an unrepresentable year is an
// option error, and a valid date still produces a strict-valid PDF.
func TestCreationDateRange(t *testing.T) {
	res, err := Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "date.html",
		FallbackFont: testFontPath(t),
		Date:         time.Date(10000, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	if err == nil || !hasCode(res.Diagnostics, "options/date") || len(res.PDF) != 0 {
		t.Fatalf("an out-of-range date was accepted: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	valid, err := Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "date.html",
		FallbackFont: testFontPath(t),
		Date:         time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("a valid date failed: %v\n%s", err, strings.Join(valid.Diagnostics.Lines(), "\n"))
	}
	validateWithPDFCPU(t, valid.PDF)
}

// TestMetadataIsValidatedByBothEntryPoints guards RV4-09: the metadata checks
// moved into preparation, so an invalid title fails both entry points with the
// same options/title diagnostic and no PDF is written.
func TestMetadataIsValidatedByBothEntryPoints(t *testing.T) {
	src := []byte(`<p>x</p>`)
	opts := Options{File: "title.html", FallbackFont: testFontPath(t), Title: "bad \xff"}
	res, err := Convert(context.Background(), src, opts)
	if err == nil || len(res.PDF) != 0 {
		t.Fatalf("Convert accepted an invalid title: err=%v pdf=%d", err, len(res.PDF))
	}
	if !hasCode(res.Diagnostics, "options/title") {
		t.Fatalf("Convert reported %v, want options/title", res.Diagnostics.Lines())
	}
	list, err := Validate(context.Background(), src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(list, "options/title") {
		t.Fatalf("Validate reported %v, want options/title", list.Lines())
	}
	if got, want := strings.Join(list.Lines(), "\n"), strings.Join(res.Diagnostics.Lines(), "\n"); got != want {
		t.Fatalf("the entry points disagree:\nValidate: %s\nConvert:  %s", got, want)
	}
}

// TestInvalidOptionStillValidatesInlineRules guards RV4-28: a wrong option
// suppressed every problem in a <style> element, although declarations in a
// style attribute were still reported. Both codes must arrive in one pass, and
// no linked asset may be opened for a request that cannot proceed.
func TestInvalidOptionStillValidatesInlineRules(t *testing.T) {
	src := []byte(`<style>p{display:flex}</style><link rel="stylesheet" href="missing.css"><p>x</p>`)
	res, err := Convert(context.Background(), src, Options{
		File:         "invalid.html",
		FallbackFont: testFontPath(t),
		PageSize:     "banana",
	})
	if err == nil {
		t.Fatal("an invalid page size was accepted")
	}
	if !hasCode(res.Diagnostics, "options/page-size") || !hasCode(res.Diagnostics, "css/unsupported-value") {
		t.Fatalf("want options/page-size and css/unsupported-value, got:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if hasCode(res.Diagnostics, "css/link-load") {
		t.Fatalf("a linked asset was opened for a request that cannot proceed:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}

// TestMissingFontNamesItsDeclaration guards RV4-37: font/unavailable carried no
// position, although the src declaration that named the missing file has one.
func TestMissingFontNamesItsDeclaration(t *testing.T) {
	src := []byte("<style>\n@font-face { font-family: Missing; src: url(missing.ttf) }\np{font-family:Missing}\n</style><p>x</p>")
	res, err := Convert(context.Background(), src, Options{
		File:         "fonts.html",
		BaseDir:      t.TempDir(),
		FallbackFont: testFontPath(t),
	})
	if err == nil {
		t.Fatal("a missing font file was accepted")
	}
	var found *diag.Diagnostic
	for i := range res.Diagnostics {
		if res.Diagnostics[i].Code == "font/unavailable" {
			found = &res.Diagnostics[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no font/unavailable diagnostic:\n%s", strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if found.Pos.File != "fonts.html" || found.Pos.Line != 2 || found.Pos.Col < 1 {
		t.Fatalf("the diagnostic points at %s:%d:%d, want fonts.html:2:<col>", found.Pos.File, found.Pos.Line, found.Pos.Col)
	}
}

// TestDiagnosticColumnsCountCharacters guards RV4-39: a column counted bytes in
// a UTF-8 document and characters in a decoded one. Both now count characters,
// so a diagnostic after Cyrillic text names the column the author sees.
func TestDiagnosticColumnsCountCharacters(t *testing.T) {
	// <p>Привет</p><img src="missing.png">: the image starts at character 14.
	utf8Src := []byte(`<p>Привет</p><img src="missing.png">`)
	res, _ := Convert(context.Background(), utf8Src, Options{
		File:         "columns.html",
		BaseDir:      t.TempDir(),
		FallbackFont: testFontPath(t),
	})
	var utf8Col int
	for _, d := range res.Diagnostics {
		if d.Code == "img/load" {
			utf8Col = d.Pos.Col
		}
	}
	if utf8Col != 14 {
		t.Fatalf("the UTF-8 column is %d, want 14:\n%s", utf8Col, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	// The same document in windows-1251: one byte per Cyrillic character, so
	// the same column.
	legacy := []byte(`<p>`)
	for _, r := range "Привет" {
		legacy = append(legacy, byte(r-0x400+0xC0))
	}
	legacy = append(legacy, []byte(`</p><img src="missing.png">`)...)
	res, _ = Convert(context.Background(), legacy, Options{
		File:         "columns.html",
		BaseDir:      t.TempDir(),
		FallbackFont: testFontPath(t),
		Charset:      "windows-1251",
	})
	var legacyCol int
	for _, d := range res.Diagnostics {
		if d.Code == "img/load" {
			legacyCol = d.Pos.Col
		}
	}
	if legacyCol != 14 {
		t.Fatalf("the legacy column is %d, want 14:\n%s", legacyCol, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
}

// TestCreationDateIsValidatedInUTC guards RV4-23: only the local year was
// checked, so a valid local date whose UTC instant fell in year 10000 wrote
// "D:100000101113000Z". The UTC year is validated and no PDF is produced.
func TestCreationDateIsValidatedInUTC(t *testing.T) {
	res, err := Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "utc.html",
		FallbackFont: testFontPath(t),
		Date:         time.Date(9999, 12, 31, 23, 30, 0, 0, time.FixedZone("m12", -12*3600)),
	})
	if err == nil || len(res.PDF) != 0 || !hasCode(res.Diagnostics, "options/date") {
		t.Fatalf("a date whose UTC year is 10000 was accepted: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	// A date whose UTC form stays inside the range is written as its UTC
	// instant.
	ok, err := Convert(context.Background(), []byte(`<p>x</p>`), Options{
		File:         "utc.html",
		FallbackFont: testFontPath(t),
		Date:         time.Date(2026, 11, 25, 23, 30, 0, 0, time.FixedZone("p3", 3*3600)),
	})
	if err != nil {
		t.Fatalf("a valid date failed: %v", err)
	}
	if !strings.Contains(string(ok.PDF), "D:20261125203000Z") {
		t.Fatal("the creation date is not the UTC instant")
	}
}

// TestOnePipelineServesBothEntryPoints guards IMP-02: Convert and Validate run
// the same preparation and layout, so a document that fails before
// serialisation produces the same diagnostic list from both. A new preparation
// check therefore reaches both entry points without a second fixture.
func TestOnePipelineServesBothEntryPoints(t *testing.T) {
	cases := map[string]Options{
		"bad page size":      {File: "p.html", PageSize: "banana"},
		"bad margin":         {File: "p.html", Margin: "huge"},
		"invalid title":      {File: "p.html", Title: "bad \xff", FallbackFont: testFontPath(t)},
		"missing stylesheet": {File: "p.html", BaseDir: t.TempDir(), FallbackFont: testFontPath(t)},
		"missing glyph":      {File: "p.html", FallbackFont: testFontPath(t)},
	}
	documents := map[string]string{
		"bad page size":      `<p>x</p>`,
		"bad margin":         `<p>x</p>`,
		"invalid title":      `<p>x</p>`,
		"missing stylesheet": `<link rel="stylesheet" href="missing.css"><p>x</p>`,
		"missing glyph":      "<p>\U0010FFFF</p>",
	}
	for name, opts := range cases {
		src := []byte(documents[name])
		res, _ := Convert(context.Background(), src, opts)
		list, err := Validate(context.Background(), src, opts)
		if err != nil {
			t.Fatalf("%s: Validate: %v", name, err)
		}
		if got, want := strings.Join(list.Lines(), "\n"), strings.Join(res.Diagnostics.Lines(), "\n"); got != want {
			t.Fatalf("%s: the entry points disagree:\nValidate:\n%s\nConvert:\n%s", name, got, want)
		}
	}
}

// TestAggregateDiagnosticNamesTheDocument guards the documented location
// classes: a failure with no single source node names the document and leaves
// the coordinates zero, while a source-attributable failure keeps its position.
func TestAggregateDiagnosticNamesTheDocument(t *testing.T) {
	t.Run("input limit", func(t *testing.T) {
		res, err := Convert(context.Background(), []byte(strings.Repeat("x", 64)), Options{
			File:   "big.html",
			Limits: Limits{MaxInputBytes: 8},
		})
		if err == nil {
			t.Fatal("the input limit was not enforced")
		}
		d := diagWithCode(t, res.Diagnostics, "resource/limit")
		if d.Pos.File != "big.html" || d.Pos.Line != 0 || d.Pos.Col != 0 {
			t.Fatalf("aggregate diagnostic = %s:%d:%d, want big.html:0:0", d.Pos.File, d.Pos.Line, d.Pos.Col)
		}
	})

	t.Run("raster limit", func(t *testing.T) {
		res, err := Convert(context.Background(), []byte(`<p>A</p>`), Options{
			File:         "render.html",
			FallbackFont: testFontPath(t),
			RenderPNG:    true,
			Limits:       Limits{MaxRasterPixels: 1},
		})
		if err == nil {
			t.Fatal("the raster limit was not enforced")
		}
		d := diagWithCode(t, res.Diagnostics, "resource/limit")
		if d.Pos.File != "render.html" || d.Pos.Line != 0 || d.Pos.Col != 0 {
			t.Fatalf("aggregate diagnostic = %s:%d:%d, want render.html:0:0", d.Pos.File, d.Pos.Line, d.Pos.Col)
		}
	})

	t.Run("conversion budget names the document", func(t *testing.T) {
		res, err := Convert(context.Background(), []byte(strings.Repeat("<p>Строка для бюджета операций.</p>", 20)), Options{
			File:         "budget.html",
			FallbackFont: testFontPath(t),
			Limits:       Limits{MaxOperations: 20},
		})
		if err == nil {
			t.Fatal("the operation budget was not enforced")
		}
		d := diagWithCode(t, res.Diagnostics, "resource/limit")
		if d.Pos.File != "budget.html" {
			t.Fatalf("budget diagnostic names %q, want budget.html", d.Pos.File)
		}
	})

	t.Run("source attributable keeps its position", func(t *testing.T) {
		res, err := Convert(context.Background(), []byte("<p>text</p>\n<img src=\"missing.png\">"), Options{
			File:         "page.html",
			BaseDir:      t.TempDir(),
			FallbackFont: testFontPath(t),
		})
		if err == nil {
			t.Fatal("the missing image was accepted")
		}
		d := diagWithCode(t, res.Diagnostics, "img/load")
		if d.Pos.File != "page.html" || d.Pos.Line != 2 || d.Pos.Col < 1 {
			t.Fatalf("source diagnostic = %s:%d:%d, want page.html:2:<col>", d.Pos.File, d.Pos.Line, d.Pos.Col)
		}
	})
}

// diagWithCode returns the first diagnostic with the given code.
func diagWithCode(t *testing.T, l diag.List, code string) diag.Diagnostic {
	t.Helper()
	for _, d := range l {
		if d.Code == code {
			return d
		}
	}
	t.Fatalf("no %s diagnostic:\n%s", code, strings.Join(l.Lines(), "\n"))
	return diag.Diagnostic{}
}

// metadataDocument builds the head metadata of a document without a parser, so
// every metadata code the gate declares can be checked. The HTML parser keeps
// attribute values valid UTF-8, so only a synthetic tree can drive the other
// three codes; the gate must not depend on where the value came from.
func metadataDocument(title, author, subject, keywords string) *html.Document {
	head := &html.Node{Type: html.ElementNode, Tag: "head"}
	if title != "" {
		head.Children = append(head.Children, &html.Node{
			Type:     html.ElementNode,
			Tag:      "title",
			Children: []*html.Node{{Type: html.TextNode, Data: title}},
		})
	}
	for _, meta := range []struct{ name, value string }{
		{"author", author},
		{"description", subject},
		{"keywords", keywords},
	} {
		if meta.value == "" {
			continue
		}
		head.Children = append(head.Children, &html.Node{
			Type:  html.ElementNode,
			Tag:   "meta",
			Attrs: []html.Attr{{Key: "name", Value: meta.name}, {Key: "content", Value: meta.value}},
		})
	}
	root := &html.Node{
		Type: html.DocumentNode,
		Children: []*html.Node{{
			Type:     html.ElementNode,
			Tag:      "html",
			Children: []*html.Node{head},
		}},
	}
	return &html.Document{Root: root, File: "meta.html"}
}

// TestMetadataOptionGateReportsAllFourCodes guards RV5-40's scope: the title,
// author, subject and keywords values are validated together, so each code is
// decided by the same gate.
func TestMetadataOptionGateReportsAllFourCodes(t *testing.T) {
	cases := []struct {
		name                          string
		title, author, subject, words string
		want                          string
	}{
		{"title", "bad \xff", "", "", "", "options/title"},
		{"author", "", "bad \xff", "", "", "options/author"},
		{"subject", "", "", "bad \xff", "", "options/subject"},
		{"keywords", "", "", "", "bad \xff", "options/keywords"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.List
			doc := metadataDocument(tc.title, tc.author, tc.subject, tc.words)
			validateMetadata(doc, Options{}, &diags)
			if len(diags) != 1 || string(diags[0].Code) != tc.want {
				t.Fatalf("diagnostics = %v, want one %s", diags.Lines(), tc.want)
			}
		})
	}
}

// TestMetadataOptionGatePrecedesAssets guards RV5-40: the document metadata is
// validated with the other options, before any linked asset is opened. The
// scenarios cover a missing stylesheet, a valid stylesheet whose opens are
// counted, and a valid metadata set that keeps the existing missing-stylesheet
// report.
func TestMetadataOptionGatePrecedesAssets(t *testing.T) {
	t.Run("invalid title and a missing stylesheet", func(t *testing.T) {
		dir := t.TempDir()
		src := []byte(`<style>p{display:flex}</style><link rel="stylesheet" href="missing.css"><p>x</p>`)
		res, err := Convert(context.Background(), src, Options{
			File:         "meta.html",
			BaseDir:      dir,
			FallbackFont: testFontPath(t),
			Title:        "bad \xff",
		})
		if err == nil {
			t.Fatal("an invalid title was accepted")
		}
		if !hasCode(res.Diagnostics, "options/title") {
			t.Fatalf("the option error is missing: %v", res.Diagnostics.Lines())
		}
		if !hasCode(res.Diagnostics, "css/unsupported-value") {
			t.Fatalf("inline styles were not validated: %v", res.Diagnostics.Lines())
		}
		if hasCode(res.Diagnostics, "css/link-load") {
			t.Fatalf("the missing stylesheet was opened for a request that cannot proceed: %v", res.Diagnostics.Lines())
		}
		if hasCode(res.Diagnostics, "font/unavailable") {
			t.Fatalf("a font was opened for a request that cannot proceed: %v", res.Diagnostics.Lines())
		}
		if len(res.PDF) != 0 {
			t.Fatal("a PDF was produced for an invalid option")
		}
	})

	t.Run("invalid title and a valid stylesheet", func(t *testing.T) {
		// The source counts actual opens, so the test proves the linked
		// stylesheet and the font are not opened, not merely that no diagnostic
		// was reported.
		files := fstest.MapFS{"present.css": &fstest.MapFile{Data: []byte("p { color: #ff0000 }")}}
		addTestFont(t, files)
		source := newCountingFS(files)
		src := []byte(`<style>p{display:flex}</style><link rel="stylesheet" href="present.css"><p>x</p>`)
		res, err := Convert(context.Background(), src, Options{
			Source:       source,
			File:         "meta.html",
			FallbackFont: "DejaVuSans.ttf",
			Title:        "bad \xff",
		})
		if err == nil {
			t.Fatal("an invalid title was accepted")
		}
		if !hasCode(res.Diagnostics, "options/title") {
			t.Fatalf("the option error is missing: %v", res.Diagnostics.Lines())
		}
		if got := source.opens["present.css"]; got != 0 {
			t.Fatalf("the linked stylesheet was opened %d times", got)
		}
		if got := source.opens["DejaVuSans.ttf"]; got != 0 {
			t.Fatalf("the font was opened %d times", got)
		}
		if reads := source.readFiles(); len(reads) != 0 {
			t.Fatalf("an asset was read for a request that cannot proceed: %v", reads)
		}
		if len(res.PDF) != 0 {
			t.Fatal("a PDF was produced for an invalid option")
		}
	})

	t.Run("valid metadata and a missing stylesheet", func(t *testing.T) {
		dir := t.TempDir()
		valid := []byte(`<html><head><title>Invoice</title>
<meta name="author" content="Acme"><meta name="description" content="Q1">
<meta name="keywords" content="invoice, q1"></head>
<body><link rel="stylesheet" href="missing.css"><p>x</p></body></html>`)
		res, err := Convert(context.Background(), valid, Options{
			File:         "meta.html",
			BaseDir:      dir,
			FallbackFont: testFontPath(t),
		})
		if err == nil || !hasCode(res.Diagnostics, "css/link-load") {
			t.Fatalf("a missing stylesheet was not reported: err=%v diags=%v", err, res.Diagnostics.Lines())
		}
		for _, code := range []string{"options/title", "options/author", "options/subject", "options/keywords"} {
			if hasCode(res.Diagnostics, code) {
				t.Fatalf("valid metadata produced %s: %v", code, res.Diagnostics.Lines())
			}
		}
	})
}

// TestPermissionFailureOnEscapesNamedStylesheetIsLoadFailure guards RV5-39 at
// the conversion level: a file named escapes.css whose mode denies reading is a
// load failure, not a path escape.
func TestPermissionFailureOnEscapesNamedStylesheetIsLoadFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "escapes.css")
	if err := os.WriteFile(path, []byte("p { color: #ff0000 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	res, err := Convert(context.Background(), []byte(`<link rel="stylesheet" href="escapes.css"><p>x</p>`), Options{
		File:         "escapes.html",
		BaseDir:      dir,
		FallbackFont: testFontPath(t),
	})
	if err == nil {
		t.Fatal("an unreadable stylesheet was accepted")
	}
	if !hasCode(res.Diagnostics, "css/link-load") {
		t.Fatalf("a permission failure inside the root was not a load failure: %v", res.Diagnostics.Lines())
	}
	if hasCode(res.Diagnostics, "css/link-path") {
		t.Fatalf("a permission failure was reported as a path escape: %v", res.Diagnostics.Lines())
	}
	if len(res.PDF) != 0 {
		t.Fatal("a PDF was produced without the stylesheet")
	}
}
