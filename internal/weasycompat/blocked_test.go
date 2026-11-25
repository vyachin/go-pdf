package weasycompat

import (
	"errors"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/pdfgen"
)

func errDiag(code string) pdfgen.Diagnostic {
	return pdfgen.Diagnostic{Severity: pdfgen.SeverityError, Code: code, Message: "unsupported", File: "a.html", Line: 3, Column: 7}
}

func blockedCase() Case {
	return Case{Name: "sample", Dir: "sample", HTML: "doc.html", State: StateBlocked, BlockedBy: "css/unsupported-property"}
}

func TestCheckBlockedAcceptsAMatchingRejection(t *testing.T) {
	obs := outcome{err: errors.New("rejected"), diags: []pdfgen.Diagnostic{errDiag("css/unsupported-property")}}
	if err := checkBlocked(blockedCase(), obs, obs); err != nil {
		t.Fatalf("checkBlocked: %v", err)
	}
}

func TestCheckBlockedRejects(t *testing.T) {
	tests := []struct {
		name   string
		first  outcome
		second outcome
		mutate func(*Case)
		want   string
	}{
		{
			name:  "a blocked case converted",
			first: outcome{pdf: []byte("%PDF-1.7")},
			want:  "declared blocked but converted",
		},
		{
			name:  "no error diagnostic",
			first: outcome{err: errors.New("boom"), diags: []pdfgen.Diagnostic{{Severity: pdfgen.SeverityWarning, Code: "img/load"}}},
			want:  "without an error diagnostic",
		},
		{
			name:  "a different first code",
			first: outcome{err: errors.New("rejected"), diags: []pdfgen.Diagnostic{errDiag("html/unsupported-element")}},
			want:  "the blocker changed",
		},
		{
			name:  "no committed blocker",
			first: outcome{err: errors.New("rejected"), diags: []pdfgen.Diagnostic{errDiag("css/unsupported-property")}},
			want:  "no blocked_by",
			mutate: func(c *Case) {
				c.BlockedBy = ""
			},
		},
		{
			name:  "unstable diagnostics",
			first: outcome{err: errors.New("rejected"), diags: []pdfgen.Diagnostic{errDiag("css/unsupported-property")}},
			second: outcome{err: errors.New("rejected"), diags: []pdfgen.Diagnostic{
				{Severity: pdfgen.SeverityError, Code: "css/unsupported-property", Message: "unsupported", File: "b.html", Line: 9, Column: 1},
			}},
			want: "different diagnostics",
		},
		{
			name:  "a panic",
			first: outcome{panic: "runtime error: index out of range\ngoroutine 1 [running]:"},
			want:  "panicked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := blockedCase()
			if tt.mutate != nil {
				tt.mutate(&c)
			}
			second := tt.second
			if second.err == nil && second.panic == "" && second.pdf == nil {
				second = tt.first
			}
			err := checkBlocked(c, tt.first, second)
			if err == nil {
				t.Fatal("checkBlocked accepted a wrong observation")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestCaseHTMLInjectsTheStylesheet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "poster/poster.html", "<html><head><title>p</title></head><body>x</body></html>\n")

	got, err := caseHTML(dir, Case{Dir: "poster", HTML: "poster.html", Stylesheet: "flyer.css"})
	if err != nil {
		t.Fatalf("caseHTML: %v", err)
	}
	want := "<html><head><title>p</title><link rel=\"stylesheet\" href=\"flyer.css\"></head><body>x</body></html>\n"
	if string(got) != want {
		t.Fatalf("caseHTML = %q, want %q", got, want)
	}
}

func TestCaseHTMLWithoutAStylesheetIsCopiedVerbatim(t *testing.T) {
	dir := t.TempDir()
	src := "<html><head></head><body>x</body></html>\n"
	writeFile(t, dir, "a/a.html", src)

	got, err := caseHTML(dir, Case{Dir: "a", HTML: "a.html"})
	if err != nil {
		t.Fatalf("caseHTML: %v", err)
	}
	if string(got) != src {
		t.Fatalf("caseHTML changed a document without a stylesheet: %q", got)
	}
}

func TestCaseHTMLNeedsAHead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/a.html", "<p>no head</p>\n")
	if _, err := caseHTML(dir, Case{Dir: "a", HTML: "a.html", Stylesheet: "a.css"}); err == nil {
		t.Fatal("caseHTML accepted a document with no </head>")
	}
}
