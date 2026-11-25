package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vyachin/go-pdf/internal/diag"
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
	}
	documents := map[string]string{
		"bad page size":      `<p>x</p>`,
		"bad margin":         `<p>x</p>`,
		"invalid title":      `<p>x</p>`,
		"missing stylesheet": `<link rel="stylesheet" href="missing.css"><p>x</p>`,
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
