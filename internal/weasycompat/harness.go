package weasycompat

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/vyachin/go-pdf/pdfgen"
)

// outcome records what one conversion of a case actually did. A panic is an
// observation, not a crash: it is reported with its stack.
type outcome struct {
	pdf   []byte
	err   error
	diags []pdfgen.Diagnostic
	panic string
}

// caseHTML reads the case document, injecting the extra stylesheet the manifest
// names. Upstream passes that stylesheet on the command line (`weasyprint -s`),
// and a browser template would carry the same link in the markup, so the
// harness adds it to an in-memory copy rather than growing the public options.
func caseHTML(dir string, c Case) ([]byte, error) {
	path := filepath.Join(dir, filepath.FromSlash(c.Dir), filepath.FromSlash(c.HTML))
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if c.Stylesheet == "" {
		return src, nil
	}
	link := []byte(`<link rel="stylesheet" href="` + c.Stylesheet + `">`)
	idx := bytes.Index(bytes.ToLower(src), []byte("</head>"))
	if idx < 0 {
		return nil, fmt.Errorf("%s has no </head> to insert the %s stylesheet into", path, c.Stylesheet)
	}
	out := make([]byte, 0, len(src)+len(link))
	out = append(out, src[:idx]...)
	out = append(out, link...)
	out = append(out, src[idx:]...)
	return out, nil
}

// options returns the conversion options a browser-authored template gets:
// browser import on, resources resolved from the sample directory, and no
// fallback font, so a sample that cannot load its declared font fails instead
// of silently rendering with a substitute.
func options(dir string, c Case) pdfgen.Options {
	sample := filepath.Join(dir, filepath.FromSlash(c.Dir))
	return pdfgen.Options{
		BaseDir:       sample,
		FontDir:       sample,
		File:          filepath.Join(sample, filepath.FromSlash(c.HTML)),
		ImportBrowser: true,
	}
}

// runCase converts one case once.
func runCase(dir string, c Case) (o outcome) {
	defer func() {
		if r := recover(); r != nil {
			o.panic = fmt.Sprintf("%v\n%s", r, debug.Stack())
		}
	}()
	src, err := caseHTML(dir, c)
	if err != nil {
		o.err = err
		return o
	}
	res, err := pdfgen.Convert(context.Background(), src, options(dir, c))
	o.pdf = res.PDF
	o.err = err
	o.diags = res.Diagnostics
	return o
}

// diagSignature renders diagnostics so two runs can be compared byte for byte.
// Position and code are part of it because "rejected in another place for
// another reason" is a different observation.
func diagSignature(diags []pdfgen.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		fmt.Fprintf(&b, "%s %s %s:%d:%d %s\n", d.Severity, d.Code, d.File, d.Line, d.Column, d.Message)
	}
	return b.String()
}

// firstErrorCode returns the code of the first error diagnostic, or "".
func firstErrorCode(diags []pdfgen.Diagnostic) string {
	for _, d := range diags {
		if d.Severity == pdfgen.SeverityError {
			return d.Code
		}
	}
	return ""
}

// checkBlocked asserts everything the corpus claims about a blocked case: it is
// rejected with an error, at least one error diagnostic names the reason, the
// first one equals the committed code, two runs agree, and nothing panicked.
func checkBlocked(c Case, first, second outcome) error {
	if first.panic != "" {
		return fmt.Errorf("case %s panicked instead of being rejected:\n%s", c.Name, first.panic)
	}
	if second.panic != "" {
		return fmt.Errorf("case %s panicked on the second run:\n%s", c.Name, second.panic)
	}
	if first.err == nil {
		return fmt.Errorf("case %s is declared blocked but converted; promote it to %q in samples.json and record its golden", c.Name, StateMeasure)
	}
	if !hasErrorDiag(first.diags) {
		return fmt.Errorf("case %s failed without an error diagnostic: %v", c.Name, first.err)
	}
	if c.BlockedBy == "" {
		return fmt.Errorf("case %s is blocked but samples.json names no blocked_by code (observed %q)", c.Name, firstErrorCode(first.diags))
	}
	if got := firstErrorCode(first.diags); got != c.BlockedBy {
		return fmt.Errorf("case %s: first error code is %q, samples.json says %q (the blocker changed)", c.Name, got, c.BlockedBy)
	}
	if a, b := diagSignature(first.diags), diagSignature(second.diags); a != b {
		return fmt.Errorf("case %s produced different diagnostics on two runs:\nfirst:\n%s\nsecond:\n%s", c.Name, a, b)
	}
	return nil
}

func hasErrorDiag(diags []pdfgen.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == pdfgen.SeverityError {
			return true
		}
	}
	return false
}
