package pdfgen_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/vyachin/go-pdf/pdfgen"
)

func TestDiagnosticOptionalFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    pdfgen.Diagnostic
		want string
	}{
		{"default severity", pdfgen.Diagnostic{Message: "failed"}, "error: failed"},
		{"file only", pdfgen.Diagnostic{File: "a.html", Column: 9, Message: "failed"}, "a.html: error: failed"},
		{"line without column", pdfgen.Diagnostic{File: "a.html", Line: 3, Message: "failed"}, "a.html:3: error: failed"},
		{"position without file", pdfgen.Diagnostic{Line: 3, Column: 9, Message: "failed"}, "error: failed"},
		{"warning", pdfgen.Diagnostic{Severity: pdfgen.SeverityWarning, Code: "layout/overflow", Element: "body > p", Message: "wide"}, "warning[layout/overflow]: wide (at body > p)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.String(); got != tc.want {
				t.Fatalf("String = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDiagnosticsErrFiltersWarningsAndOwnsItsList(t *testing.T) {
	list := pdfgen.Diagnostics{
		{Severity: pdfgen.SeverityWarning, Message: "warning"},
		{Message: "implicit error"},
		{Severity: pdfgen.SeverityError, Code: "css/unsupported-value", Message: "explicit error"},
	}
	want := append(pdfgen.Diagnostics(nil), list[1:]...)
	var got *pdfgen.Error
	if err := list.Err(); !errors.As(err, &got) {
		t.Fatalf("Err = %v, want *pdfgen.Error", err)
	}
	if !reflect.DeepEqual(got.Diagnostics, want) {
		t.Fatalf("Error.Diagnostics = %+v, want %+v", got.Diagnostics, want)
	}
	if got.Error() != "error: implicit error\nerror[css/unsupported-value]: explicit error" {
		t.Fatalf("Error = %q", got.Error())
	}
	list[1].Message = "changed by caller"
	if !reflect.DeepEqual(got.Diagnostics, want) {
		t.Fatal("changing the original list changed the returned error")
	}
}

func TestDiagnosticsLinesAndEmptyLists(t *testing.T) {
	for _, list := range []pdfgen.Diagnostics{nil, {}, {{Severity: pdfgen.SeverityWarning, Message: "warning"}}} {
		if list.HasErrors() || list.Err() != nil {
			t.Fatalf("empty or warning-only list reports an error: %+v", list)
		}
		if len(list.Lines()) != len(list) {
			t.Fatalf("Lines lost entries: %+v", list)
		}
	}
	list := pdfgen.Diagnostics{{Message: "first"}, {Severity: pdfgen.SeverityWarning, Message: "second"}}
	want := []string{"error: first", "warning: second"}
	if got := list.Lines(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Lines = %q, want %q", got, want)
	}
	if !list.HasErrors() {
		t.Fatal("the default severity must count as an error")
	}
}
