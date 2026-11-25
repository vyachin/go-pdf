// Package diag defines diagnostics with source positions.
//
// Every layer of the engine reports problems through this package so that
// validation, layout and PDF writing produce the same shape of message and a
// deterministic order of output.
package diag

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Severity classifies a diagnostic.
type Severity int

const (
	// Error means the document cannot be rendered.
	Error Severity = iota
	// Warning means the document can be rendered but the result may be
	// surprising (for example content that overflows its page).
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// Position is a 1-based location in a source file. A zero field means the
// value is unknown and is omitted from the formatted output.
type Position struct {
	File string
	Line int
	Col  int
}

// Diagnostic is a single problem report.
type Diagnostic struct {
	Severity Severity
	// Code is a stable machine-readable identifier, for example "css/unsupported-value".
	Code    string
	Message string
	Pos     Position
	// Element is a human-readable path such as "body > table > tr".
	Element string
}

// String renders a diagnostic in the form used by the CLI:
//
//	file:line:col: error[code]: message (at body > p)
func (d Diagnostic) String() string {
	var b strings.Builder
	if d.Pos.File != "" {
		b.WriteString(d.Pos.File)
		if d.Pos.Line > 0 {
			fmt.Fprintf(&b, ":%d", d.Pos.Line)
			if d.Pos.Col > 0 {
				fmt.Fprintf(&b, ":%d", d.Pos.Col)
			}
		}
		b.WriteString(": ")
	}
	b.WriteString(d.Severity.String())
	if d.Code != "" {
		b.WriteString("[")
		b.WriteString(d.Code)
		b.WriteString("]")
	}
	b.WriteString(": ")
	b.WriteString(d.Message)
	if d.Element != "" {
		b.WriteString(" (at ")
		b.WriteString(d.Element)
		b.WriteString(")")
	}
	return b.String()
}

// List is an ordered collection of diagnostics.
type List []Diagnostic

// Errorf appends an error diagnostic.
func (l *List) Errorf(code string, pos Position, element, format string, args ...any) {
	*l = append(*l, Diagnostic{
		Severity: Error,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
		Pos:      pos,
		Element:  element,
	})
}

// Warnf appends a warning diagnostic.
func (l *List) Warnf(code string, pos Position, element, format string, args ...any) {
	*l = append(*l, Diagnostic{
		Severity: Warning,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
		Pos:      pos,
		Element:  element,
	})
}

// HasErrors reports whether the list contains at least one error.
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Sort orders diagnostics deterministically: by file, line, column, code and
// finally message. Two runs over the same input therefore produce the same
// report.
func (l List) Sort() {
	// Stable: two diagnostics with the same key keep their discovery order,
	// which is what the golden layers pin.
	slices.SortStableFunc(l, func(a, b Diagnostic) int {
		if c := cmp.Compare(a.Pos.File, b.Pos.File); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Pos.Line, b.Pos.Line); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Pos.Col, b.Pos.Col); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Code, b.Code); c != 0 {
			return c
		}
		return cmp.Compare(a.Message, b.Message)
	})
}

// EscalateWarnings turns every warning into an error. Strict mode uses it so
// that a template with warnings fails the build.
func (l List) EscalateWarnings() {
	for i := range l {
		if l[i].Severity == Warning {
			l[i].Severity = Error
		}
	}
}

// Lines renders every diagnostic on its own line, in the current order.
func (l List) Lines() []string {
	out := make([]string, len(l))
	for i, d := range l {
		out[i] = d.String()
	}
	return out
}

// Error returns an error containing every error-severity diagnostic, or nil
// when the list has no errors. Warnings are not errors.
func (l List) Error() error {
	var msgs []string
	for _, d := range l {
		if d.Severity == Error {
			msgs = append(msgs, d.String())
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return &Errors{Diagnostics: l, lines: msgs}
}

// Errors is the error type returned by List.Error.
type Errors struct {
	Diagnostics List
	lines       []string
}

func (e *Errors) Error() string { return strings.Join(e.lines, "\n") }
