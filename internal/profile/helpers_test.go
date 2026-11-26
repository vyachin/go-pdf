package profile

import (
	"testing"

	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/stylesheet"
)

// parseDoc parses src under the document name the tests expect.
func parseDoc(t *testing.T, src string) *html.Document {
	t.Helper()
	doc, err := html.Parse([]byte(src), "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// validateDoc collects the document's stylesheets with the loading stage and
// validates them with the profile, the way the engine does.
func validateDoc(t *testing.T, doc *html.Document, opts Options) diag.List {
	t.Helper()
	sheets, sheetDiags := stylesheet.Collect(doc, stylesheet.Options{Budget: opts.Budget})
	out := ValidateSheets(doc, opts, sheets, sheetDiags)
	out.Sort()
	return out
}

// hasDiagCode reports whether the list carries the code.
func hasDiagCode(diags diag.List, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
