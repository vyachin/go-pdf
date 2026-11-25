package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/pdfgen"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

// TestRemovedSubcommandsRejected covers the migration path from the previous
// command surface: a subcommand word is named as removed instead of being read
// as a missing input file.
func TestRemovedSubcommandsRejected(t *testing.T) {
	for _, arg := range []string{"convert", "validate", "help"} {
		code, stdout, stderr := runCLI(t, arg)
		if code != exitUsage {
			t.Fatalf("%q exit code = %d, want %d", arg, code, exitUsage)
		}
		if !strings.Contains(stderr, "removed") {
			t.Fatalf("%q stderr = %q, want a removal notice", arg, stderr)
		}
		if stdout != "" {
			t.Fatalf("%q wrote to stdout: %q", arg, stdout)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"unknown flag", []string{"-frobnicate", "doc.html"}},
		{"two inputs", []string{"a.html", "b.html"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCLI(t, tc.args...)
			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d", code, exitUsage)
			}
			if stderr == "" {
				t.Fatal("expected a message on stderr")
			}
		})
	}
}

func TestHelpExitsZero(t *testing.T) {
	code, stdout, _ := runCLI(t, "-h")
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	for _, want := range []string{"pdfgen [-o out.pdf] [-charset name] <input.html>", "no\ncompatibility promise", "windows-1251"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help output does not contain %q:\n%s", want, stdout)
		}
	}
}

func requireFont(t *testing.T) {
	t.Helper()
	if p := os.Getenv("PDFTEST_FONT"); p != "" {
		return
	}
	for _, p := range []string{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
	} {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Skip("no TrueType font available; set PDFTEST_FONT")
}

func writeDocument(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultOutputPath(t *testing.T) {
	requireFont(t)
	dir := t.TempDir()
	in := writeDocument(t, dir, "doc.html", `<style>p{font-size:12pt}</style><p>Привет</p>`)

	code, stdout, stderr := runCLI(t, in)
	if code != exitOK {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "doc.pdf"))
	if err != nil {
		t.Fatalf("output was not written next to the input: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("output is not a PDF: %q", data[:min(8, len(data))])
	}
	if !strings.Contains(stdout, "wrote") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestExplicitOutputPath(t *testing.T) {
	requireFont(t)
	dir := t.TempDir()
	in := writeDocument(t, dir, "doc.html", `<style>p{font-size:12pt}</style><p>Привет</p>`)
	out := filepath.Join(t.TempDir(), "custom.pdf")

	code, _, stderr := runCLI(t, "-o", out, in)
	if code != exitOK {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("the explicit output was not written: %v", err)
	}
}

func TestMissingInputFileFails(t *testing.T) {
	code, _, stderr := runCLI(t, filepath.Join(t.TempDir(), "absent.html"))
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr, "no such file") {
		t.Fatalf("stderr = %q, want it to mention a missing file", stderr)
	}
}

func TestInvalidTemplateWritesNothing(t *testing.T) {
	dir := t.TempDir()
	in := writeDocument(t, dir, "bad.html", `<style>p{color:notacolour}</style><p>x</p>`)

	code, stdout, stderr := runCLI(t, in)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want it empty", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.pdf")); err == nil {
		t.Fatal("no PDF must be written for an invalid template")
	}
	if !strings.Contains(stderr, "bad.html") {
		t.Fatalf("diagnostics do not name the input file:\n%s", stderr)
	}
}

func TestFailedConversionKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	in := writeDocument(t, dir, "bad.html", `<style>p{color:notacolour}</style><p>x</p>`)
	out := filepath.Join(dir, "out.pdf")
	previous := []byte("%PDF-1.4 previous document")
	if err := os.WriteFile(out, previous, 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, _ := runCLI(t, "-o", out, in)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the previous file disappeared: %v", err)
	}
	if !bytes.Equal(got, previous) {
		t.Fatalf("the previous file changed:\n got: %q\nwant: %q", got, previous)
	}
}

func TestAssetsResolveAgainstInputDir(t *testing.T) {
	dir := t.TempDir()
	logo, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "logo.png"))
	if err != nil {
		t.Fatalf("fixture is missing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), logo, 0o644); err != nil {
		t.Fatal(err)
	}
	in := writeDocument(t, dir, "doc.html", `<html><head><style>
  @page { size: A5; margin: 10mm }
  img { width: 20mm; height: 20mm }
</style></head><body><p><img src="logo.png"></p></body></html>`)

	// Run from an unrelated directory: only the input path may matter.
	t.Chdir(t.TempDir())
	code, _, stderr := runCLI(t, in)
	if code != exitOK {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc.pdf")); err != nil {
		t.Fatalf("the PDF was not written next to the input: %v", err)
	}
}

func TestBrowserCertificateIsRejectedByCLI(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "browser", "certificate.html")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("browser fixture is missing: %v", err)
	}
	out := filepath.Join(t.TempDir(), "certificate.pdf")
	if err := os.WriteFile(out, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLI(t, "-o", out, src)
	if code != exitFailure {
		t.Fatalf("convert of the browser certificate = %d, want %d\nstdout: %s\nstderr: %s", code, exitFailure, stdout, stderr)
	}
	if !strings.Contains(stderr, "error[") {
		t.Fatalf("stderr carries no profile diagnostics:\n%s", stderr)
	}
	if strings.Contains(stderr, "error[browser/") || strings.Contains(stderr, "warning[browser/") {
		t.Fatalf("stderr carries a removed browser code:\n%s", stderr)
	}
	if data, err := os.ReadFile(out); err != nil || string(data) != "previous" {
		t.Fatalf("the existing output was changed: %q, %v", data, err)
	}
}

// TestDiagnosticsPrintedOnce guards RV2-26: the command printed every error
// diagnostic and then an error that joins the same lines, so each problem
// appeared twice on stderr.
func TestDiagnosticsPrintedOnce(t *testing.T) {
	dir := t.TempDir()
	in := writeDocument(t, dir, "bad.html", `<p style="display:grid">x</p><p style="color:notacolour">y</p>`)

	code, _, stderr := runCLI(t, "-o", filepath.Join(t.TempDir(), "out.pdf"), in)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	for _, name := range []string{"grid", "notacolour"} {
		if got := strings.Count(stderr, name); got != 1 {
			t.Fatalf("the %s diagnostic appears %d times:\n%s", name, got, stderr)
		}
	}
	if lines := strings.Count(strings.TrimSpace(stderr), "\n") + 1; lines != 3 {
		t.Fatalf("stderr has %d lines, want 3 (two diagnostics and one summary):\n%s", lines, stderr)
	}
	if !strings.Contains(stderr, "template is invalid") {
		t.Fatalf("the summary line is missing:\n%s", stderr)
	}
}

// TestCharsetFlag covers the input-encoding flag: a bad value is a usage error
// decided before the input is read, and a good one decodes a document that
// declares nothing.
func TestCharsetFlag(t *testing.T) {
	// "Привет" in windows-1251, with no declaration.
	const legacy = `<p>` + "\xcf\xf0\xe8\xe2\xe5\xf2" + `</p>`

	t.Run("an unknown encoding is a usage error", func(t *testing.T) {
		dir := t.TempDir()
		in := writeDocument(t, dir, "doc.html", legacy)
		code, stdout, stderr := runCLI(t, "-charset", "shift_jis", in)
		if code != exitUsage {
			t.Fatalf("exit code = %d, want %d", code, exitUsage)
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want it empty", stdout)
		}
		if !strings.Contains(stderr, "shift_jis") || !strings.Contains(stderr, "Usage:") {
			t.Fatalf("stderr does not name the value and the usage line:\n%s", stderr)
		}
		if _, err := os.Stat(filepath.Join(dir, "doc.pdf")); !os.IsNotExist(err) {
			t.Fatal("a usage error must not write a PDF")
		}
	})

	t.Run("the flag decodes a document that declares nothing", func(t *testing.T) {
		requireFont(t)
		dir := t.TempDir()
		in := writeDocument(t, dir, "doc.html", legacy)

		// Without the flag the document cannot be decoded, which is the whole
		// reason the flag exists.
		code, _, stderr := runCLI(t, in)
		if code != exitFailure {
			t.Fatalf("exit code without -charset = %d, want %d", code, exitFailure)
		}
		if !strings.Contains(stderr, "html/encoding") {
			t.Fatalf("stderr does not name html/encoding:\n%s", stderr)
		}

		code, _, stderr = runCLI(t, "-charset", "windows-1251", in)
		if code != exitOK {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
		}
		data, err := os.ReadFile(filepath.Join(dir, "doc.pdf"))
		if err != nil {
			t.Fatalf("no PDF was written: %v", err)
		}
		if !strings.HasPrefix(string(data), "%PDF-") {
			t.Fatalf("output is not a PDF: %q", data[:min(8, len(data))])
		}
	})

	t.Run("detection without the flag", func(t *testing.T) {
		requireFont(t)
		dir := t.TempDir()
		in := writeDocument(t, dir, "doc.html", `<meta charset="windows-1251">`+legacy)
		code, _, stderr := runCLI(t, in)
		if code != exitOK {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
		}
	})
}

// TestOutputEqualToInputRefused guards the source: neither the default output
// nor an explicit one may truncate the input.
func TestOutputEqualToInputRefused(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "template.pdf")
	if err := os.WriteFile(source, []byte("source bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, source)
	if code != exitUsage || !strings.Contains(stderr, "refusing") {
		t.Fatalf("default output: code=%d stderr=%q, want a refusal", code, stderr)
	}
	if data, _ := os.ReadFile(source); string(data) != "source bytes" {
		t.Fatalf("the input was modified: %q", data)
	}

	doc := writeDocument(t, dir, "invoice.html", `<p>x</p>`)
	code, _, stderr = runCLI(t, "-o", doc, doc)
	if code != exitUsage || !strings.Contains(stderr, "refusing") {
		t.Fatalf("explicit output: code=%d stderr=%q, want a refusal", code, stderr)
	}
	if data, _ := os.ReadFile(doc); !strings.Contains(string(data), "<p>") {
		t.Fatalf("the input was modified: %q", data)
	}
}

// TestOversizedInputRejectedBeforeConversion guards the size check: a file
// beyond the exported default limit fails before it is read.
func TestOversizedInputRejectedBeforeConversion(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "big.html")
	limit := pdfgen.DefaultLimits().MaxInputBytes
	f, err := os.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(limit) + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, in)
	if code != exitFailure || !strings.Contains(stderr, "the limit is") {
		t.Fatalf("code=%d stderr=%q, want the limit failure", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "big.pdf")); err == nil {
		t.Fatal("an output file was written for an oversized input")
	}
}

// TestAtomicWriteKeepsThePreviousFile guards temp+rename: a write that cannot
// create its temporary file leaves the destination byte-identical.
func TestAtomicWriteKeepsThePreviousFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based failure does not apply to root")
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(dest, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	err := writeFileAtomic(dest, []byte("new"))
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("the write succeeded in a read-only directory")
	}
	if data, readErr := os.ReadFile(dest); readErr != nil || string(data) != "previous" {
		t.Fatalf("destination = %q, %v; want the previous bytes", data, readErr)
	}
}

// TestUsageDocumentsTheNewFailures guards the documented messages.
func TestUsageDocumentsTheNewFailures(t *testing.T) {
	var b strings.Builder
	usage(&b)
	for _, want := range []string{"must not name the input", "MaxInputBytes", "renamed into place"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("usage does not mention %q:\n%s", want, b.String())
		}
	}
}
