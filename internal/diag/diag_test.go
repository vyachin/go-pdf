package diag

import (
	"strings"
	"testing"
)

func TestStringFormatting(t *testing.T) {
	cases := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			name: "full position",
			d: Diagnostic{
				Severity: Error,
				Code:     "css/unsupported-value",
				Message:  "display: flex is not supported",
				Pos:      Position{File: "invoice.html", Line: 12, Col: 5},
				Element:  "body > div > p",
			},
			want: "invoice.html:12:5: error[css/unsupported-value]: display: flex is not supported (at body > div > p)",
		},
		{
			name: "no position",
			d: Diagnostic{
				Severity: Warning,
				Code:     "layout/overflow",
				Message:  "content exceeds page",
			},
			want: "warning[layout/overflow]: content exceeds page",
		},
		{
			name: "file without line",
			d: Diagnostic{
				Severity: Error,
				Message:  "cannot open font",
				Pos:      Position{File: "fonts/pt.ttf"},
			},
			want: "fonts/pt.ttf: error: cannot open font",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSortIsDeterministic(t *testing.T) {
	build := func() List {
		return List{
			{Severity: Error, Code: "b", Message: "second", Pos: Position{File: "a.html", Line: 2, Col: 1}},
			{Severity: Error, Code: "a", Message: "first", Pos: Position{File: "a.html", Line: 1, Col: 9}},
			{Severity: Warning, Code: "z", Message: "warn", Pos: Position{File: "a.html", Line: 1, Col: 9}},
			{Severity: Error, Code: "a", Message: "aaa", Pos: Position{File: "a.html", Line: 1, Col: 9}},
			{Severity: Error, Code: "a", Message: "zero", Pos: Position{File: "a.html"}},
		}
	}
	a, b := build(), build()
	a.Sort()
	b.Sort()
	if strings.Join(a.Lines(), "|") != strings.Join(b.Lines(), "|") {
		t.Fatalf("sort is not deterministic:\n%v\n%v", a.Lines(), b.Lines())
	}
	// The zero line sorts first within the same file, then line 1 entries by code/message.
	if a[0].Pos.Line != 0 {
		t.Fatalf("first diagnostic should be the one without a line, got %+v", a[0])
	}
	if a[1].Message != "aaa" || a[2].Message != "first" || a[3].Message != "warn" {
		t.Fatalf("unexpected order: %v", a.Lines())
	}
}

func TestHasErrorsIgnoresWarnings(t *testing.T) {
	l := List{{Severity: Warning, Message: "just a warning"}}
	if l.HasErrors() {
		t.Fatal("warnings must not count as errors")
	}
	if err := l.Error(); err != nil {
		t.Fatalf("Error() on warnings only = %v, want nil", err)
	}
	l = append(l, Diagnostic{Severity: Error, Message: "boom"})
	if !l.HasErrors() {
		t.Fatal("expected errors")
	}
	if err := l.Error(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Error() = %v, want it to contain boom", err)
	}
	if strings.Contains(errText(l), "just a warning") {
		t.Fatal("warnings must not appear in the error text")
	}
}

func errText(l List) string {
	if err := l.Error(); err != nil {
		return err.Error()
	}
	return ""
}
