package stylesheet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/profile"
)

func writeCSS(t *testing.T, dir, name, content string) string {
	t.Helper()
	return writeCSSBytes(t, dir, name, []byte(content))
}

func writeCSSBytes(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if sub := filepath.Dir(path); sub != dir {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func parseDoc(t *testing.T, src string) *html.Document {
	t.Helper()
	doc, err := html.Parse([]byte(src), "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// parseDocWithCharset parses an ASCII document that declares a legacy encoding,
// the way the engine parses an input file.
func parseDocWithCharset(t *testing.T, src, name string) *html.Document {
	t.Helper()
	doc, err := html.ParseWithOptions([]byte(src), html.ParseOptions{File: "doc.html", Charset: name})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func encodeText(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	out, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encoding %q: %v", s, err)
	}
	return out
}

func TestCollectKeepsDocumentOrder(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "a.css", "p { color: #ff0000 }")
	doc := parseDoc(t, `<link rel="stylesheet" href="a.css"><style>p { color: #00ff00 }</style><p>текст</p>`)
	sheets, diags := Collect(doc, Options{BaseDir: dir})
	if diags.HasErrors() {
		t.Fatalf("diagnostics: %v", diags.Lines())
	}
	if len(sheets) != 2 {
		t.Fatalf("got %d sheets, want 2", len(sheets))
	}
	if sheets[0].File == "" || sheets[1].File != "" {
		t.Fatalf("order looks wrong: %q then %q", sheets[0].File, sheets[1].File)
	}
	c := css.NewCascade(sheets...)
	p := doc.Body().FirstElementChild()
	st := c.Compute(p, nil)
	want := css.Color{R: 0, G: 255, B: 0, Set: true}
	if st.Color != want {
		t.Fatalf("color = %+v, want the later inline rule", st.Color)
	}
}

func TestLinkMediaAndRel(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "print.css", "p { color: #ff0000 }")
	writeCSS(t, dir, "screen.css", "p { color: #ff0000 }")

	printDoc := parseDoc(t, `<link rel="stylesheet" media="print" href="print.css"><p>x</p>`)
	sheets, diags := Collect(printDoc, Options{BaseDir: dir})
	if diags.HasErrors() || len(sheets) != 1 {
		t.Fatalf("print stylesheet: sheets=%d diags=%v", len(sheets), diags.Lines())
	}

	screenDoc := parseDoc(t, `<link rel="stylesheet" media="screen" href="screen.css"><p>x</p>`)
	sheets, diags = Collect(screenDoc, Options{BaseDir: dir})
	if diags.HasErrors() || len(sheets) != 0 {
		t.Fatalf("screen stylesheet must be skipped: sheets=%d diags=%v", len(sheets), diags.Lines())
	}

	iconDoc := parseDoc(t, `<link rel="icon" href="favicon.ico"><p>x</p>`)
	sheets, diags = Collect(iconDoc, Options{BaseDir: dir})
	if diags.HasErrors() || len(sheets) != 0 {
		t.Fatalf("icon link must be ignored: sheets=%d diags=%v", len(sheets), diags.Lines())
	}
}

func TestLinkErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		href string
		code string
	}{
		{"missing", "missing.css", "css/link-load"},
		{"remote", "https://example.com/site.css", "css/link-remote"},
		{"data", "data:text/css,p{}", "css/link-unsupported"},
		{"traversal", "../outside.css", "css/link-path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseDoc(t, `<link rel="stylesheet" href="`+tc.href+`"><p>x</p>`)
			_, diags := Collect(doc, Options{BaseDir: dir})
			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics, want 1: %v", len(diags), diags.Lines())
			}
			if string(diags[0].Code) != tc.code || diags[0].Severity != 0 {
				t.Fatalf("diagnostic = %+v, want code %s", diags[0], tc.code)
			}
		})
	}
}

// TestLinkDiagnosticMatrix freezes the code, message, file and position of every
// link failure. The rows are the pre-extraction output: the loader maps the
// registry's typed failures and never resolves a reference itself, but the
// reader must see no difference.
func TestLinkDiagnosticMatrix(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCSS(t, dir, "big.css", strings.Repeat("p { color: #ff0000 }\n", 40))
	cases := []struct {
		name   string
		href   string
		max    int
		code   string
		prefix func(dir string) string
	}{
		{
			"missing", "missing.css", 0, "css/link-load",
			func(dir string) string {
				return "cannot read stylesheet " + strconv.Quote(filepath.Join(dir, "missing.css"))
			},
		},
		{
			"remote", "https://example.com/site.css", 0, "css/link-remote",
			func(string) string { return "remote stylesheets are not supported: https://example.com/site.css" },
		},
		{
			"data", "data:text/css,p{}", 0, "css/link-unsupported",
			func(string) string { return "data: stylesheets are not supported" },
		},
		{
			"escape", "../outside.css", 0, "css/link-path",
			func(string) string { return `stylesheet path "../outside.css" escapes the base directory` },
		},
		{
			"directory", "sub", 0, "css/link-load",
			func(string) string { return `cannot read stylesheet "sub": ` },
		},
		{
			"oversized", "big.css", 32, "resource/limit",
			func(string) string { return `budget: work limit exceeded: stylesheet "big.css" exceeds the limit` },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseDoc(t, `<link rel="stylesheet" href="`+tc.href+`"><p>x</p>`)
			sheets, diags := Collect(doc, Options{BaseDir: dir, MaxStylesheetBytes: tc.max})
			if len(sheets) != 0 {
				t.Fatalf("a failed link produced %d sheets", len(sheets))
			}
			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics, want 1: %v", len(diags), diags.Lines())
			}
			d := diags[0]
			if string(d.Code) != tc.code {
				t.Fatalf("code = %q, want %q (%s)", d.Code, tc.code, d.Message)
			}
			if !strings.HasPrefix(d.Message, tc.prefix(dir)) {
				t.Fatalf("message = %q, want prefix %q", d.Message, tc.prefix(dir))
			}
			if d.Pos.File != "doc.html" || d.Pos.Line != 1 || d.Pos.Col != 1 {
				t.Fatalf("position = %s:%d:%d, want doc.html:1:1", d.Pos.File, d.Pos.Line, d.Pos.Col)
			}
			if d.Element != "head > link" {
				t.Fatalf("element = %q, want head > link", d.Element)
			}
		})
	}
}

// TestReferenceIsResolvedOnce guards the single-resolver rule: four spellings
// of one href — a query, a fragment, percent encoding and surrounding spaces —
// name one file, are read once by the registry, and share one parse while both
// inclusions keep their cascade place. The parsed sheet names the same path the
// pre-extraction resolver produced.
func TestReferenceIsResolvedOnce(t *testing.T) {
	dir := t.TempDir()
	path := writeCSS(t, dir, "a b.css", "p { color: #ff0000 }")
	doc := parseDoc(t, `<link rel="stylesheet" href="a%20b.css?v=1#frag">`+
		`<link rel="stylesheet" href=" a b.css ">`+
		`<link rel="stylesheet" href="a%20b.css">`+
		`<link rel="stylesheet" href="./a b.css?x=2"><p>x</p>`)
	reg := assets.New(nil)
	defer func() { _ = reg.Close() }()
	sheets, diags := Collect(doc, Options{BaseDir: dir, Assets: reg})
	if diags.HasErrors() {
		t.Fatalf("collection failed: %v", diags.Lines())
	}
	if len(sheets) != 4 {
		t.Fatalf("collected %d inclusions, want 4", len(sheets))
	}
	for i, sheet := range sheets {
		if sheet != sheets[0] {
			t.Fatalf("inclusion %d was parsed again", i)
		}
		if sheet.File != path {
			t.Fatalf("inclusion %d names %q, want %q", i, sheet.File, path)
		}
	}
	if got := reg.Reads(assets.Stylesheet); got != 1 {
		t.Fatalf("read %d stylesheets, want 1", got)
	}
}

func TestExternalStylesheetErrorsPointAtTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCSS(t, dir, "print.css", "p {\n  display: flex;\n}\n")
	doc := parseDoc(t, `<link rel="stylesheet" href="print.css"><p>x</p>`)
	sheets, sheetDiags := Collect(doc, Options{BaseDir: dir})
	diags := profile.ValidateSheets(doc, profile.Options{}, sheets, sheetDiags)
	if !diags.HasErrors() {
		t.Fatal("expected an error from the linked stylesheet")
	}
	var found bool
	for _, d := range diags {
		if d.Code == "css/unsupported-value" && d.Pos.File == path && d.Pos.Line == 2 && d.Pos.Col > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no diagnostic pointing at %s:2: %v", path, diags.Lines())
	}
}

func TestLinkedStylesApplyToTheDocument(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "styles/invoice.css", ".total { font-weight: 700 }")
	doc := parseDoc(t, `<link rel="stylesheet" href="styles/invoice.css"><p class="total">Итого</p>`)
	sheets, sheetDiags := Collect(doc, Options{BaseDir: dir})
	if diags := profile.ValidateSheets(doc, profile.Options{}, sheets, sheetDiags); diags.HasErrors() {
		t.Fatalf("diagnostics: %v", diags.Lines())
	}
	c := css.NewCascade(sheets...)
	p := doc.Body().FirstElementChild()
	if got := c.Compute(p, nil).FontWeight; got != 700 {
		t.Fatalf("font-weight = %d, want 700 from the linked stylesheet", got)
	}
}

// TestStylesheetOutsideTheBaseDirectoryIsRejected is the non-symlink case: an
// existing stylesheet above the base directory is reported with the same code
// as an escaping symlink, and its rules never reach the cascade.
func TestStylesheetOutsideTheBaseDirectoryIsRejected(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "base")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCSS(t, parent, "outside.css", "p { color: #ff0000 }")
	doc := parseDoc(t, `<link rel="stylesheet" href="../outside.css"><p>x</p>`)
	sheets, diags := Collect(doc, Options{BaseDir: base})
	if len(sheets) != 0 {
		t.Fatal("a stylesheet outside the base directory must not be loaded")
	}
	if len(diags) != 1 || diags[0].Code != "css/link-path" {
		t.Fatalf("diagnostics = %v, want one css/link-path", diags.Lines())
	}
}

func TestSymlinkEscapeIsRejected(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.css")
	if err := os.WriteFile(outsideFile, []byte("p { color: #ff0000 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link.css")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	doc := parseDoc(t, `<link rel="stylesheet" href="link.css"><p>x</p>`)
	sheets, diags := Collect(doc, Options{BaseDir: base})
	if len(sheets) != 0 {
		t.Fatal("a stylesheet outside the base directory must not be loaded")
	}
	if len(diags) != 1 || diags[0].Code != "css/link-path" {
		t.Fatalf("diagnostics = %v, want one css/link-path", diags.Lines())
	}
}

func TestSymlinkInsideBaseWorks(t *testing.T) {
	base := t.TempDir()
	real := writeCSS(t, base, "real.css", "p { color: #ff0000 }")
	link := filepath.Join(base, "link.css")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	doc := parseDoc(t, `<link rel="stylesheet" href="link.css"><p>x</p>`)
	sheets, diags := Collect(doc, Options{BaseDir: base})
	if diags.HasErrors() {
		t.Fatalf("diagnostics: %v", diags.Lines())
	}
	if len(sheets) != 1 || len(sheets[0].Rules) != 1 {
		t.Fatalf("sheets = %d, rules = %d", len(sheets), len(sheets[0].Rules))
	}
}

func hasDiagCode(diags diag.List, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func firstWithCode(diags diag.List, code diag.Code) *diag.Diagnostic {
	for i := range diags {
		if diag.Code(diags[i].Code) == code {
			return &diags[i]
		}
	}
	return nil
}

func TestStylesheetByteLimit(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "big.css", strings.Repeat("p { color: #ff0000 }\n", 40))
	doc := parseDoc(t, `<link rel="stylesheet" href="big.css"><p>x</p>`)
	_, diags := Collect(doc, Options{BaseDir: dir, MaxStylesheetBytes: 32})
	if !hasDiagCode(diags, "resource/limit") {
		t.Fatalf("stylesheet size limit was not enforced: %v", diags.Lines())
	}
}

func TestStylesheetCollectionHonoursBudget(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "a.css", "p { color: #ff0000 }")
	doc := parseDoc(t, strings.Repeat(`<p>x</p>`, 50)+`<link rel="stylesheet" href="a.css">`)
	guard := budget.New(context.Background(), 5)
	_, diags := Collect(doc, Options{BaseDir: dir, Budget: guard})
	if !errors.Is(guard.Err(), budget.ErrExceeded) {
		t.Fatalf("collection did not stop at the budget: %v", guard.Err())
	}
	if hasDiagCode(diags, "css/link-load") {
		t.Fatalf("the stop was reported as a link failure: %v", diags.Lines())
	}
}

func TestStylesheetCollectionHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := parseDoc(t, `<p>x</p>`)
	guard := budget.New(ctx, 0)
	Collect(doc, Options{Budget: guard})
	if !errors.Is(guard.Err(), context.Canceled) {
		t.Fatalf("collection did not notice the cancellation: %v", guard.Err())
	}
}

// TestRepeatedStylesheetSharesOneParseKeepsOrder guards the stylesheet half of
// the asset contract: one file linked twice with another sheet between is read
// and parsed once, yet both inclusions keep their place in the cascade.
func TestRepeatedStylesheetSharesOneParseKeepsOrder(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "a.css", "p { color: #ff0000 }")
	writeCSS(t, dir, "b.css", "p { color: #00ff00 }")
	doc := parseDoc(t, `<link rel="stylesheet" href="a.css"><link rel="stylesheet" href="b.css"><link rel="stylesheet" href="a.css"><p>x</p>`)
	reg := assets.New(nil)
	defer func() { _ = reg.Close() }()
	sheets, diags := Collect(doc, Options{BaseDir: dir, Assets: reg})
	if diags.HasErrors() {
		t.Fatalf("collection failed: %v", diags.Lines())
	}
	if len(sheets) != 3 {
		t.Fatalf("collected %d inclusions, want 3", len(sheets))
	}
	if sheets[0] != sheets[2] {
		t.Fatal("the repeated stylesheet was parsed again")
	}
	if filepath.Base(sheets[1].File) != "b.css" {
		t.Fatalf("the middle inclusion is %q, want b.css", sheets[1].File)
	}
	// One file read once: a.css and b.css, in that order, nothing else.
	if got := reg.Reads(assets.Stylesheet); got != 2 {
		t.Fatalf("read %d stylesheets, want 2", got)
	}
	// The cascade keeps the document order: red, green, red.
	rules := css.RulesForPrint(sheets)
	if len(rules) != 3 {
		t.Fatalf("the cascade has %d rules, want 3", len(rules))
	}
	if rules[0].Sheet != rules[2].Sheet || rules[0].Offset != rules[2].Offset {
		t.Fatal("the repeated inclusion does not point at the same parsed rule")
	}
}

// TestPermissionFailureInsideTheRootIsALoadFailure guards RV5-39: a file named
// escapes.css whose mode denies reading is inside the root, so the reported
// cause is a load failure and the file name cannot turn it into a path escape.
func TestPermissionFailureInsideTheRootIsALoadFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	path := writeCSS(t, dir, "escapes.css", "p { color: #ff0000 }")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	doc := parseDoc(t, `<link rel="stylesheet" href="escapes.css"><p>x</p>`)
	sheets, diags := Collect(doc, Options{BaseDir: dir})
	if len(sheets) != 0 {
		t.Fatal("an unreadable stylesheet was loaded")
	}
	if len(diags) != 1 || diags[0].Code != "css/link-load" {
		t.Fatalf("diagnostics = %v, want one css/link-load", diags.Lines())
	}
}

// TestCollectReportsABadRoot guards the standalone path: a collection called
// without a conversion registry opens its own root and reports the directory it
// cannot open instead of reading without a boundary.
func TestCollectReportsABadRoot(t *testing.T) {
	doc := parseDoc(t, `<link rel="stylesheet" href="a.css"><p>x</p>`)
	_, diags := Collect(doc, Options{BaseDir: filepath.Join(t.TempDir(), "missing")})
	if len(diags) != 1 || diags[0].Code != "options/asset-root" {
		t.Fatalf("diagnostics = %v, want one options/asset-root", diags.Lines())
	}
}

// TestLegacySheetKeepsSourcePositions guards the position contract of the
// decode stage: a windows-1251 or KOI8-R sheet and its ASCII twin have the same
// byte layout, so the diagnostic inside them must report the same line and
// column, and a leading @charset rule must be blanked without moving a later
// offset. Each encoding case converts one sheet with and one without @charset.
func TestLegacySheetKeepsSourcePositions(t *testing.T) {
	cases := []struct {
		name string
		enc  encoding.Encoding
	}{
		{"windows-1251", charmap.Windows1251},
		{"koi8-r", charmap.KOI8R},
	}
	for _, tc := range cases {
		for _, charset := range []string{"", "@charset \"" + tc.name + "\";\n"} {
			name := "without @charset"
			if charset != "" {
				name = "with @charset"
			}
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				dir := t.TempDir()
				build := func(word string) []byte {
					return encodeText(t, tc.enc, charset+"/* "+word+" */\n.x { display: flex }\n")
				}
				legacyPath := writeCSSBytes(t, dir, "legacy.css", build("Счёт"))
				asciiPath := writeCSSBytes(t, dir, "ascii.css", build("Abcd"))
				legacyDoc := parseDocWithCharset(t, `<link rel="stylesheet" href="legacy.css"><p class="x">x</p>`, tc.name)
				asciiDoc := parseDocWithCharset(t, `<link rel="stylesheet" href="ascii.css"><p class="x">x</p>`, tc.name)

				legacySheets, legacySheetDiags := Collect(legacyDoc, Options{BaseDir: dir})
				asciiSheets, asciiSheetDiags := Collect(asciiDoc, Options{BaseDir: dir})
				if len(legacySheets) != 1 || len(asciiSheets) != 1 {
					t.Fatalf("sheets = %d (legacy) and %d (ASCII)", len(legacySheets), len(asciiSheets))
				}
				if legacySheets[0].File != legacyPath || asciiSheets[0].File != asciiPath {
					t.Fatalf("files = %q and %q, want %q and %q",
						legacySheets[0].File, asciiSheets[0].File, legacyPath, asciiPath)
				}
				legacyDiags := profile.ValidateSheets(legacyDoc, profile.Options{}, legacySheets, legacySheetDiags)
				asciiDiags := profile.ValidateSheets(asciiDoc, profile.Options{}, asciiSheets, asciiSheetDiags)
				legacyDiag := firstWithCode(legacyDiags, diag.CSSUnsupportedValue)
				asciiDiag := firstWithCode(asciiDiags, diag.CSSUnsupportedValue)
				if legacyDiag == nil || asciiDiag == nil {
					t.Fatalf("missing the unsupported-value diagnostic: %v / %v", legacyDiags.Lines(), asciiDiags.Lines())
				}
				wantLine := 2
				if charset != "" {
					wantLine = 3
				}
				if legacyDiag.Pos.Line != wantLine || asciiDiag.Pos.Line != wantLine {
					t.Fatalf("lines = %d (legacy) and %d (ASCII), want %d", legacyDiag.Pos.Line, asciiDiag.Pos.Line, wantLine)
				}
				if legacyDiag.Pos.Col != asciiDiag.Pos.Col {
					t.Fatalf("columns = %d (legacy) and %d (ASCII); decoding must not move a position",
						legacyDiag.Pos.Col, asciiDiag.Pos.Col)
				}
				// The @charset rule is blanked, never removed: the parser sees
				// only whitespace where it stood, and the first rule still maps
				// back to the line of the source file.
				if len(legacySheets[0].Rules) != 1 || len(asciiSheets[0].Rules) != 1 {
					t.Fatalf("rules = %d (legacy) and %d (ASCII), want 1", len(legacySheets[0].Rules), len(asciiSheets[0].Rules))
				}
				for name, sheet := range map[string]*css.Stylesheet{"legacy": legacySheets[0], "ASCII": asciiSheets[0]} {
					pos := sheet.Position(sheet.Rules[0].Offset)
					if pos.Line != wantLine || pos.Col != 1 {
						t.Fatalf("%s rule position = %d:%d, want %d:1", name, pos.Line, pos.Col, wantLine)
					}
				}
			})
		}
	}
}

// TestCharsetRule pins the @charset grammar the decode stage consumes.
func TestCharsetRule(t *testing.T) {
	cases := []struct {
		src   string
		label string
		end   int
		ok    bool
	}{
		{`@charset "windows-1251";`, "windows-1251", 24, true},
		{`@CHARSET 'utf-8' ;`, "utf-8", 18, true},
		{`@charset "utf-8" ;`, "utf-8", 18, true},
		{`@charset "utf-8"; p { color: #000 }`, "utf-8", 17, true},
		{` @charset "utf-8";`, "", 0, false},
		{`@charset "utf-8"`, "", 0, false},
		{`@charset utf-8;`, "", 0, false},
	}
	for _, tc := range cases {
		label, end, ok := charsetRule([]byte(tc.src))
		if label != tc.label || end != tc.end || ok != tc.ok {
			t.Errorf("charsetRule(%q) = %q, %d, %t; want %q, %d, %t",
				tc.src, label, end, ok, tc.label, tc.end, tc.ok)
		}
	}
}
