package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

func writeCSS(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if sub := filepath.Dir(path); sub != dir {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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

func TestCollectStylesheetsKeepsDocumentOrder(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "a.css", "p { color: #ff0000 }")
	doc := parseDoc(t, `<link rel="stylesheet" href="a.css"><style>p { color: #00ff00 }</style><p>текст</p>`)
	sheets, diags := CollectStylesheets(doc, Options{BaseDir: dir})
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
	sheets, diags := CollectStylesheets(printDoc, Options{BaseDir: dir})
	if diags.HasErrors() || len(sheets) != 1 {
		t.Fatalf("print stylesheet: sheets=%d diags=%v", len(sheets), diags.Lines())
	}

	screenDoc := parseDoc(t, `<link rel="stylesheet" media="screen" href="screen.css"><p>x</p>`)
	sheets, diags = CollectStylesheets(screenDoc, Options{BaseDir: dir})
	if diags.HasErrors() || len(sheets) != 0 {
		t.Fatalf("screen stylesheet must be skipped: sheets=%d diags=%v", len(sheets), diags.Lines())
	}

	iconDoc := parseDoc(t, `<link rel="icon" href="favicon.ico"><p>x</p>`)
	sheets, diags = CollectStylesheets(iconDoc, Options{BaseDir: dir})
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
			_, diags := CollectStylesheets(doc, Options{BaseDir: dir})
			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics, want 1: %v", len(diags), diags.Lines())
			}
			if string(diags[0].Code) != tc.code || diags[0].Severity != 0 {
				t.Fatalf("diagnostic = %+v, want code %s", diags[0], tc.code)
			}
		})
	}
}

func TestExternalStylesheetErrorsPointAtTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCSS(t, dir, "print.css", "p {\n  display: flex;\n}\n")
	doc := parseDoc(t, `<link rel="stylesheet" href="print.css"><p>x</p>`)
	diags := Validate(doc, Options{BaseDir: dir})
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
	diags := Validate(doc, Options{BaseDir: dir})
	if diags.HasErrors() {
		t.Fatalf("diagnostics: %v", diags.Lines())
	}
	sheets, _ := CollectStylesheets(doc, Options{BaseDir: dir})
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
	sheets, diags := CollectStylesheets(doc, Options{BaseDir: base})
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
	sheets, diags := CollectStylesheets(doc, Options{BaseDir: base})
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
	sheets, diags := CollectStylesheets(doc, Options{BaseDir: base})
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

func TestStylesheetByteLimit(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "big.css", strings.Repeat("p { color: #ff0000 }\n", 40))
	doc := parseDoc(t, `<link rel="stylesheet" href="big.css"><p>x</p>`)
	_, diags := CollectStylesheets(doc, Options{BaseDir: dir, MaxStylesheetBytes: 32})
	if !hasDiagCode(diags, "resource/limit") {
		t.Fatalf("stylesheet size limit was not enforced: %v", diags.Lines())
	}
}

func TestStylesheetCollectionHonoursBudget(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "a.css", "p { color: #ff0000 }")
	doc := parseDoc(t, strings.Repeat(`<p>x</p>`, 50)+`<link rel="stylesheet" href="a.css">`)
	guard := budget.New(context.Background(), 5)
	_, diags := CollectStylesheets(doc, Options{BaseDir: dir, Budget: guard})
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
	CollectStylesheets(doc, Options{Budget: guard})
	if !errors.Is(guard.Err(), context.Canceled) {
		t.Fatalf("collection did not notice the cancellation: %v", guard.Err())
	}
}
