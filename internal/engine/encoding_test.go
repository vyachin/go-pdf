package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"

	"github.com/vyachin/go-pdf/internal/diag"
)

// cp1251Bytes returns s in windows-1251.
func cp1251Bytes(t *testing.T, s string) []byte {
	t.Helper()
	return encodedBytes(t, charmap.Windows1251, s)
}

// koi8rBytes returns s in KOI8-R.
func koi8rBytes(t *testing.T, s string) []byte {
	t.Helper()
	return encodedBytes(t, charmap.KOI8R, s)
}

func encodedBytes(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	out, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encoding %q: %v", s, err)
	}
	return out
}

// fixtureOptions are the options a fixture converts with: the bundled font, the
// fixture directory for assets and the bundled font directory.
func fixtureOptions(t *testing.T, name string) Options {
	t.Helper()
	return Options{
		FallbackFont: testFontPath(t),
		File:         name,
		BaseDir:      filepath.Join(repoRoot(), "testdata", "fixtures"),
		FontDir:      filepath.Join(repoRoot(), "testdata", "fonts"),
	}
}

// convertEncodingFixture converts one file from testdata/fixtures.
func convertEncodingFixture(t *testing.T, name string) (Result, []byte) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRoot(), "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), src, fixtureOptions(t, name))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res, src
}

func TestExplicitCharset(t *testing.T) {
	font := testFontPath(t)
	t.Run("decodes a document that declares nothing", func(t *testing.T) {
		src := cp1251Bytes(t, "<p>Счёт на оплату</p>")
		res, err := Convert(context.Background(), src, Options{FallbackFont: font, Charset: "windows-1251", File: "doc.html"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Diagnostics.HasErrors() {
			t.Fatalf("diagnostics: %v", res.Diagnostics.Lines())
		}
		text, err := extractText(res.PDF)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "Счёт на оплату") {
			t.Fatalf("text = %q, want the decoded Cyrillic", text)
		}
	})

	t.Run("wins over a declaration", func(t *testing.T) {
		src := cp1251Bytes(t, `<meta charset="koi8-r"><p>Счёт</p>`)
		res, err := Convert(context.Background(), src, Options{FallbackFont: font, Charset: "cp1251", File: "doc.html"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Diagnostics.HasErrors() {
			t.Fatalf("diagnostics: %v", res.Diagnostics.Lines())
		}
		text, err := extractText(res.PDF)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "Счёт") {
			t.Fatalf("text = %q, want the explicit encoding to win", text)
		}
	})

	t.Run("fails before an asset is read", func(t *testing.T) {
		// BaseDir does not exist: if the pipeline opened the base directory
		// first, the failure would name css/link-load instead.
		src := []byte(`<link rel="stylesheet" href="x.css"><p>x</p>`)
		missing := filepath.Join(t.TempDir(), "absent")
		runs := []struct {
			name string
			run  func() (diag.List, error)
		}{
			{"validate", func() (diag.List, error) {
				return Validate(context.Background(), src, Options{Charset: "shift_jis", BaseDir: missing, File: "doc.html"})
			}},
			{"convert", func() (diag.List, error) {
				// Convert returns the diagnostics as its error, so only the
				// list matters here.
				res, _ := Convert(context.Background(), src, Options{Charset: "shift_jis", BaseDir: missing, File: "doc.html"})
				return res.Diagnostics, nil
			}},
		}
		for _, r := range runs {
			diags, err := r.run()
			if err != nil {
				t.Fatalf("%s: %v", r.name, err)
			}
			if len(diags) != 1 {
				t.Fatalf("%s: diagnostics = %v, want exactly one", r.name, diags.Lines())
			}
			d := diags[0]
			if d.Code != "options/charset" {
				t.Fatalf("%s: code = %q, want options/charset (%v)", r.name, d.Code, diags.Lines())
			}
			if !strings.Contains(d.Message, "shift_jis") || !strings.Contains(d.Message, "windows-1251") {
				t.Fatalf("%s: message does not name the value and the set: %q", r.name, d.Message)
			}
			if d.Pos.Line != 0 || d.Pos.Col != 0 {
				t.Fatalf("%s: an option error must have no source position, got %+v", r.name, d.Pos)
			}
		}
	})
}

func TestEncodingDiagnostics(t *testing.T) {
	font := testFontPath(t)
	cases := []struct {
		name    string
		src     []byte
		line    int
		col     int
		message []string
	}{
		{
			name:    "unsupported declaration",
			src:     []byte(`<meta charset="shift_jis"><p>x</p>`),
			line:    1,
			col:     1,
			message: []string{"shift_jis", "windows-1251", "utf-8"},
		},
		{
			name:    "utf-16 document",
			src:     []byte{0xFF, 0xFE, '<', 0, 'p', 0, '>', 0},
			message: []string{"UTF-16"},
		},
		{
			name:    "undeclared legacy document",
			src:     []byte("<p>" + string(cp1251Bytes(t, "Счёт")) + "</p>"),
			line:    1,
			col:     4,
			message: []string{"Options.Charset"},
		},
		{
			name:    "undefined byte",
			src:     append(cp1251Bytes(t, "<meta charset=\"windows-1251\">\n<p>"), 0x98),
			line:    2,
			col:     4,
			message: []string{"0x98", "windows-1251"},
		},
		{
			name:    "invalid utf-8 under a declaration",
			src:     []byte("<meta charset=\"utf-8\">\n<p>\xC3\x28</p>"),
			line:    2,
			col:     4,
			message: []string{"not valid UTF-8"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{File: "doc.html", FallbackFont: font}
			diags, err := Validate(context.Background(), tc.src, opts)
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !diags.HasErrors() {
				t.Fatal("validation accepted a document it cannot decode")
			}
			d := firstWithCode(diags, "html/encoding")
			if d == nil {
				t.Fatalf("no html/encoding diagnostic: %v", diags.Lines())
			}
			for _, want := range tc.message {
				if !strings.Contains(d.Message, want) {
					t.Fatalf("message %q does not contain %q", d.Message, want)
				}
			}
			if tc.line != 0 && (d.Pos.Line != tc.line || d.Pos.Col != tc.col) {
				t.Fatalf("position = %d:%d, want %d:%d (%s)", d.Pos.Line, d.Pos.Col, tc.line, tc.col, d.Message)
			}

			// Convert returns the diagnostics as its error; what matters here
			// is that no PDF was produced and the code is the same one.
			res, _ := Convert(context.Background(), tc.src, opts)
			if len(res.PDF) != 0 {
				t.Fatal("a document that cannot be decoded must not produce a PDF")
			}
			if !res.Diagnostics.HasErrors() {
				t.Fatal("Convert reported no error")
			}
			if firstWithCode(res.Diagnostics, "html/encoding") == nil {
				t.Fatalf("Convert reported no html/encoding diagnostic: %v", res.Diagnostics.Lines())
			}
		})
	}
}

// TestUTF8ByteOrderMark proves the mark is invisible: the document with it and
// the same document without it convert to the same bytes.
func TestUTF8ByteOrderMark(t *testing.T) {
	withBOM, _ := convertEncodingFixture(t, "utf8-bom-invoice.html")
	plain, _ := convertEncodingFixture(t, "utf8-invoice.html")
	if withBOM.Diagnostics.HasErrors() || plain.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics: %v / %v", withBOM.Diagnostics.Lines(), plain.Diagnostics.Lines())
	}
	if !bytes.Equal(withBOM.PDF, plain.PDF) {
		t.Fatal("the byte order mark changed the output")
	}
	text, err := extractText(withBOM.PDF)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Обычный UTF-8") {
		t.Fatalf("text = %q", text)
	}
	if strings.ContainsRune(text, '\uFEFF') {
		t.Fatal("the byte order mark survived into the text layer")
	}
}

// TestLegacyInvoice converts the windows-1251 invoice, freezes its text layer
// as a golden and checks the output with the independent validator.
func TestLegacyInvoice(t *testing.T) {
	res, _ := convertEncodingFixture(t, "cp1251-invoice.html")
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics: %v", res.Diagnostics.Lines())
	}
	validateWithPDFCPU(t, res.PDF)

	text, err := extractText(res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	for _, mark := range []string{"Счёт № 7", "Исполнитель", "Ромашка", "Наименование", "534 000,00", "НДС включён"} {
		if !strings.Contains(text, mark) {
			t.Fatalf("text does not contain %q:\n%s", mark, text)
		}
	}
	checkGolden(t, "cp1251-invoice.txt", text)

	again, _ := convertEncodingFixture(t, "cp1251-invoice.html")
	if !bytes.Equal(res.PDF, again.PDF) {
		t.Fatal("two conversions of the same decoded document differ")
	}
}

// TestLegacyStylesheet covers the external stylesheet rules: it is decoded with
// the document's encoding, a matching @charset is accepted, a conflicting one
// is rejected, and a diagnostic inside it names the source position.
func TestLegacyStylesheet(t *testing.T) {
	font := testFontPath(t)
	doc := cp1251Bytes(t, `<meta charset="windows-1251"><link rel="stylesheet" href="styles.css"><p class="drop">Удалить</p><p>Оставить</p>`)

	// convert writes a stylesheet next to the document and converts the pair.
	convert := func(t *testing.T, sheet []byte) Result {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "doc.html"), doc, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "styles.css"), sheet, 0o644); err != nil {
			t.Fatal(err)
		}
		// A rejected stylesheet is reported through the diagnostics, which
		// Convert also returns as its error.
		res, _ := Convert(context.Background(), doc, Options{FallbackFont: font, File: "doc.html", BaseDir: dir})
		return res
	}

	t.Run("applies a decoded sheet with a matching charset", func(t *testing.T) {
		sheet := cp1251Bytes(t, "@charset \"windows-1251\";\n/* Скрыть строку. */\n.drop { display: none }\n")
		res := convert(t, sheet)
		if res.Diagnostics.HasErrors() {
			t.Fatalf("diagnostics: %v", res.Diagnostics.Lines())
		}
		text, err := extractText(res.PDF)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "Удалить") {
			t.Fatalf("the decoded sheet did not apply: %q", text)
		}
		if !strings.Contains(text, "Оставить") {
			t.Fatalf("text = %q", text)
		}
	})

	t.Run("rejects a conflicting charset", func(t *testing.T) {
		sheet := cp1251Bytes(t, "@charset \"utf-8\";\n.drop { display: none }\n")
		res := convert(t, sheet)
		d := requireDiagnostic(t, res.Diagnostics, "css/link-encoding", "utf-8", "windows-1251")
		if filepath.Base(d.Pos.File) != "styles.css" || d.Pos.Line != 1 || d.Pos.Col != 1 {
			t.Fatalf("position = %s:%d:%d, want styles.css:1:1", d.Pos.File, d.Pos.Line, d.Pos.Col)
		}
	})

	t.Run("rejects an undefined byte", func(t *testing.T) {
		sheet := append(cp1251Bytes(t, ".drop { display: none }\n/* "), 0x98)
		res := convert(t, sheet)
		d := requireDiagnostic(t, res.Diagnostics, "css/link-encoding", "0x98")
		if d.Pos.Line != 2 || d.Pos.Col != 4 {
			t.Fatalf("position = %d:%d, want 2:4", d.Pos.Line, d.Pos.Col)
		}
	})

	t.Run("reports a position inside a decoded inline sheet", func(t *testing.T) {
		// The same document twice: a four-byte Cyrillic word and a four-byte
		// ASCII word on line 2. The inline sheet is parsed at document offsets,
		// so the two must report the same line and column.
		line := "/* WORD */ p { display: flex }"
		makeDoc := func(word string) []byte {
			return cp1251Bytes(t, "<meta charset=\"windows-1251\">\n<style>\n"+strings.Replace(line, "WORD", word, 1)+"\n</style><p>x</p>")
		}
		cyrillic, err := Validate(context.Background(), makeDoc("Счёт"), Options{File: "doc.html", FallbackFont: font})
		if err != nil {
			t.Fatal(err)
		}
		ascii, err := Validate(context.Background(), makeDoc("Abcd"), Options{File: "doc.html", FallbackFont: font})
		if err != nil {
			t.Fatal(err)
		}
		fromCyrillic := requireDiagnostic(t, cyrillic, "css/unsupported-value", "flex")
		fromASCII := requireDiagnostic(t, ascii, "css/unsupported-value", "flex")
		if fromCyrillic.Pos.Line != 3 || fromCyrillic.Pos.Col != fromASCII.Pos.Col {
			t.Fatalf("position = %d:%d, want line 3 and the ASCII column %d", fromCyrillic.Pos.Line, fromCyrillic.Pos.Col, fromASCII.Pos.Col)
		}
	})

	t.Run("reports the source column of a decoded sheet", func(t *testing.T) {
		// Two sheets with the same byte layout: a four-byte Cyrillic word and a
		// four-byte ASCII word. The columns must agree, because a column counts
		// source bytes; a decoded column would put the Cyrillic one four
		// columns further right.
		cyrillic := convert(t, cp1251Bytes(t, "/* Счёт */ .x { display: flex }\n"))
		ascii := convert(t, []byte("/* Abcd */ .x { display: flex }\n"))
		fromCyrillic := requireDiagnostic(t, cyrillic.Diagnostics, "css/unsupported-value", "flex")
		fromASCII := requireDiagnostic(t, ascii.Diagnostics, "css/unsupported-value", "flex")
		if fromCyrillic.Pos.Line != 1 || fromCyrillic.Pos.Line != fromASCII.Pos.Line {
			t.Fatalf("lines = %d and %d, want 1 and 1", fromCyrillic.Pos.Line, fromASCII.Pos.Line)
		}
		if fromCyrillic.Pos.Col != fromASCII.Pos.Col {
			t.Fatalf("columns = %d (windows-1251) and %d (ASCII); decoding must not move a position", fromCyrillic.Pos.Col, fromASCII.Pos.Col)
		}
	})
}

// requireDiagnostic returns the first diagnostic with the code and checks its
// message.
func requireDiagnostic(t *testing.T, diags diag.List, code string, message ...string) *diag.Diagnostic {
	t.Helper()
	d := firstWithCode(diags, code)
	if d == nil {
		t.Fatalf("no %s diagnostic: %v", code, diags.Lines())
	}
	for _, want := range message {
		if !strings.Contains(d.Message, want) {
			t.Fatalf("message %q does not contain %q", d.Message, want)
		}
	}
	return d
}

func firstWithCode(diags diag.List, code string) *diag.Diagnostic {
	for i := range diags {
		if diags[i].Code == code {
			return &diags[i]
		}
	}
	return nil
}

// TestEncodingDetection covers every detection rule the design lists.
func TestEncodingDetection(t *testing.T) {
	font := testFontPath(t)
	declared := func(decl string) []byte {
		return cp1251Bytes(t, decl+`<p>`+"Счёт"+`</p>`)
	}
	cases := []struct {
		name string
		src  []byte
		ok   bool
	}{
		{"utf-8 byte order mark", append([]byte{0xEF, 0xBB, 0xBF}, []byte("<p>Привет</p>")...), true},
		{"meta charset", declared(`<meta charset="windows-1251">`), true},
		{"http-equiv", declared(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1251">`), true},
		{"koi8-r declaration", koi8rBytes(t, `<meta charset="koi8-r"><p>Счёт</p>`), true},
		{"no declaration, utf-8", []byte("<p>Привет</p>"), true},
		{"declaration inside a comment", append([]byte(`<!-- <meta charset="windows-1251"> --><p>`), cp1251Bytes(t, "Счёт")...), false},
		{"declaration ends inside 1024 bytes", append(bytes.Repeat([]byte(" "), 990), declared(`<meta charset="windows-1251">`)...), true},
		{"declaration after 1024 bytes", append(bytes.Repeat([]byte(" "), 1100), declared(`<meta charset="windows-1251">`)...), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags, err := Validate(context.Background(), tc.src, Options{File: "doc.html", FallbackFont: font})
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got := !diags.HasErrors(); got != tc.ok {
				t.Fatalf("decoded = %v, want %v: %v", got, tc.ok, diags.Lines())
			}
		})
	}
}

// TestHostileEncodingInput guards the two cheap-to-store, expensive-to-report
// shapes: one long line with thousands of diagnostics, and a document at the
// input limit.
func TestHostileEncodingInput(t *testing.T) {
	font := testFontPath(t)

	t.Run("one line, many diagnostics", func(t *testing.T) {
		var b strings.Builder
		b.WriteString(`<meta charset="windows-1251">`)
		for i := 0; i < 20000; i++ {
			b.WriteString("<foo></foo>Счёт")
		}
		src := cp1251Bytes(t, b.String())
		start := time.Now()
		diags, err := Validate(context.Background(), src, Options{File: "hostile.html", FallbackFont: font})
		if err != nil {
			t.Fatalf("Validate: %v", err)
		}
		elapsed := time.Since(start)
		if len(diags) < 20000 {
			t.Fatalf("got %d diagnostics, want at least 20000", len(diags))
		}
		last := 0
		for _, d := range diags {
			if d.Code != "html/unsupported-element" {
				t.Fatalf("diagnostic = %q (%s)", d.Code, d.Message)
			}
			// Every position is a source position and the walk resumes from its
			// cursor: a walk from the line start per diagnostic would not
			// finish in this budget.
			if d.Pos.Line != 1 || d.Pos.Col < last {
				t.Fatalf("position = %d:%d after column %d", d.Pos.Line, d.Pos.Col, last)
			}
			last = d.Pos.Col
		}
		if elapsed > 30*time.Second {
			t.Fatalf("%d diagnostics took %v", len(diags), elapsed)
		}
	})

	t.Run("input at the limit", func(t *testing.T) {
		const limit = 8 << 20
		head := cp1251Bytes(t, `<meta charset="windows-1251"><p>Счёт</p><!-- `)
		tail := []byte(" -->")
		src := make([]byte, 0, limit)
		src = append(src, head...)
		src = append(src, bytes.Repeat([]byte("a"), limit-len(head)-len(tail))...)
		src = append(src, tail...)
		if len(src) != limit {
			t.Fatalf("built %d bytes, want %d", len(src), limit)
		}
		start := time.Now()
		res, err := Convert(context.Background(), src, Options{File: "big.html", FallbackFont: font})
		if err != nil {
			t.Fatalf("Convert: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 30*time.Second {
			t.Fatalf("an 8 MiB document took %v", elapsed)
		}
		if len(res.PDF) == 0 {
			t.Fatalf("no PDF; diagnostics: %v", res.Diagnostics.Lines())
		}
		text, err := extractText(res.PDF)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "Счёт") {
			t.Fatalf("text = %q", text)
		}
	})
}

// TestEncodingDeterminismAndConcurrency proves decoded documents convert
// identically in sequence and in parallel, which is the guarantee that the
// shared encoding tables introduce no mutable state.
func TestEncodingDeterminismAndConcurrency(t *testing.T) {
	type doc struct {
		name string
		src  []byte
		opts Options
	}
	var docs []doc
	for _, name := range []string{"cp1251-invoice.html", "koi8r-invoice.html", "utf8-bom-invoice.html"} {
		src, err := os.ReadFile(filepath.Join(repoRoot(), "testdata", "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc{name: name, src: src, opts: fixtureOptions(t, name)})
	}
	if len(docs) != 3 {
		t.Fatalf("%d documents", len(docs))
	}

	want := make(map[string][]byte, len(docs))
	for _, d := range docs {
		res, err := Convert(context.Background(), d.src, d.opts)
		if err != nil {
			t.Fatalf("%s: %v", d.name, err)
		}
		if res.Diagnostics.HasErrors() {
			t.Fatalf("%s: %v", d.name, res.Diagnostics.Lines())
		}
		want[d.name] = res.PDF
	}

	var wg sync.WaitGroup
	errs := make(chan string, len(docs)*3)
	for _, d := range docs {
		for worker := 0; worker < 3; worker++ {
			wg.Add(1)
			go func(d doc) {
				defer wg.Done()
				res, err := Convert(context.Background(), d.src, d.opts)
				if err != nil {
					errs <- d.name + ": " + err.Error()
					return
				}
				if !bytes.Equal(res.PDF, want[d.name]) {
					errs <- d.name + ": a parallel conversion differs from the sequential one"
				}
			}(d)
		}
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Fatal(msg)
	}
}
