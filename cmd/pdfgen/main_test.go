package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no arguments", nil, exitUsage},
		{"unknown command", []string{"frobnicate"}, exitUsage},
		{"convert without input", []string{"convert"}, exitUsage},
		{"convert with two inputs", []string{"convert", "a.html", "b.html"}, exitUsage},
		{"validate without input", []string{"validate"}, exitUsage},
		{"bad date", []string{"convert", "-date", "yesterday", "a.html"}, exitUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCLI(t, tc.args...)
			if code != tc.want {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tc.want, stderr)
			}
			if stderr == "" {
				t.Fatal("expected a message on stderr")
			}
		})
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		code, stdout, _ := runCLI(t, args...)
		if code != exitOK {
			t.Fatalf("run(%v) = %d, want %d", args, code, exitOK)
		}
		if !strings.Contains(stdout, "pdfgen convert") {
			t.Fatalf("help output does not mention convert: %q", stdout)
		}
	}
	// Flag help for a subcommand must also exit zero.
	if code, _, _ := runCLI(t, "convert", "-h"); code != exitOK {
		t.Fatalf("convert -h exit code = %d, want %d", code, exitOK)
	}
}

func TestMissingInputFileFails(t *testing.T) {
	for _, cmd := range []string{"convert", "validate"} {
		code, _, stderr := runCLI(t, cmd, filepath.Join(t.TempDir(), "absent.html"))
		if code != exitFailure {
			t.Fatalf("%s with a missing file = %d, want %d", cmd, code, exitFailure)
		}
		if !strings.Contains(stderr, "no such file") {
			t.Fatalf("%s stderr = %q, want it to mention a missing file", cmd, stderr)
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

func TestConvertWritesPDF(t *testing.T) {
	requireFont(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(in, []byte("<style>p{font-size:12pt}</style><p>Привет</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "convert", in)
	if code != exitOK {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	out := filepath.Join(dir, "doc.pdf")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output file was not written: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("output is not a PDF: %q", data[:8])
	}
	if !strings.Contains(stdout, "wrote") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestConvertBrowserCertificate(t *testing.T) {
	requireFont(t)
	src := filepath.Join("..", "..", "testdata", "browser", "certificate.html")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("browser fixture is missing: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "certificate.pdf")
	code, stdout, stderr := runCLI(t, "convert", "-o", out, src)
	if code != exitOK {
		t.Fatalf("convert of a browser document = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the PDF was not written: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("output is not a PDF: %q", data[:min(8, len(data))])
	}
	if !strings.Contains(stderr, "browser/flex") || !strings.Contains(stderr, "browser/background") {
		t.Fatalf("stderr does not report the lowering:\n%s", stderr)
	}

	strictOut := filepath.Join(dir, "strict.pdf")
	code, _, _ = runCLI(t, "convert", "-strict", "-o", strictOut, src)
	if code != exitFailure {
		t.Fatalf("strict convert of a browser document = %d, want %d", code, exitFailure)
	}
	if _, err := os.Stat(strictOut); err == nil {
		t.Fatal("strict mode must not write a PDF for a browser document")
	}
}

func TestConvertFailsOnInvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.html")
	if err := os.WriteFile(in, []byte("<style>p{display:flex}</style><p>x</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "convert", in)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.pdf")); err == nil {
		t.Fatal("no PDF must be written for an invalid template")
	}
	if !strings.Contains(stderr, "bad.html") {
		t.Fatalf("diagnostics do not name the input file: %s", stderr)
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateCommand(t *testing.T) {
	valid := writeTemp(t, "valid.html", `<html><head><title>Счёт</title></head><body><h1>Счёт</h1><p>Текст</p></body></html>`)
	code, stdout, stderr := runCLI(t, "validate", valid)
	if code != exitOK {
		t.Fatalf("validate(valid) = %d, want 0 (stdout: %s, stderr: %s)", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "is valid") {
		t.Fatalf("stdout = %q, want a success message", stdout)
	}

	invalid := writeTemp(t, "invalid.html", `<style>p { display: flex }</style><p>x</p>`)
	code, stdout, stderr = runCLI(t, "validate", invalid)
	if code != exitFailure {
		t.Fatalf("validate(invalid) = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stdout, "css/unsupported-value") {
		t.Fatalf("stdout = %q, want the diagnostic on stdout", stdout)
	}
	if !strings.Contains(stderr, "invalid") {
		t.Fatalf("stderr = %q, want an invalid summary", stderr)
	}
}

func TestPrintProfile(t *testing.T) {
	code, stdout, stderr := runCLI(t, "validate", "-print-profile")
	if code != exitOK {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	for _, want := range []string{"Supported elements:", "display", "font-size", "counter(page)"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("profile output does not contain %q", want)
		}
	}
}

func TestConvertRenderPNG(t *testing.T) {
	requireFont(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(in, []byte("<p>png</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "convert", "-render-png", in)
	if code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "doc.png"))
	if err != nil {
		t.Fatalf("png was not written: %v", err)
	}
	if len(data) < 8 || string(data[1:4]) != "PNG" {
		t.Fatalf("output is not a PNG: %q", data[:min(8, len(data))])
	}
}

func TestStrictMode(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "script.html")
	if err := os.WriteFile(in, []byte(`<script>x()</script><p>текст</p>`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCLI(t, "validate", in)
	if code != exitOK {
		t.Fatalf("non-strict validate = %d, want 0 (stdout: %s)", code, stdout)
	}
	if !strings.Contains(stdout, "html/ignored-script") {
		t.Fatalf("stdout = %q, want the script warning", stdout)
	}

	code, _, _ = runCLI(t, "validate", "-strict", in)
	if code != exitFailure {
		t.Fatalf("strict validate = %d, want %d", code, exitFailure)
	}

	requireFont(t)
	code, _, _ = runCLI(t, "convert", "-strict", in)
	if code != exitFailure {
		t.Fatalf("strict convert = %d, want %d", code, exitFailure)
	}
	if _, err := os.Stat(filepath.Join(dir, "script.pdf")); err == nil {
		t.Fatal("strict convert must not write a PDF")
	}
}

func TestConvertAcceptsSharedPageUnit(t *testing.T) {
	requireFont(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(in, []byte(`<style>p{font-size:12pt}</style><p>Страница</p>`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "convert", "-page", "210x297mm", in)
	if code != exitOK {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "doc.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	// The documented syntax must produce an A4-sized MediaBox.
	if !strings.Contains(string(data), "/MediaBox [0 0 595.27") {
		t.Fatal("the shared-unit page size did not reach the PDF")
	}
}

// TestConvertPrintsEachDiagnosticOnce guards RV2-26: convert printed every
// error diagnostic and then the engine error, which joins the same lines, so
// each problem appeared twice on stderr.
func TestConvertPrintsEachDiagnosticOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.html")
	if err := os.WriteFile(path, []byte(`<p style="flex:1">x</p><p style="color:notacolour">y</p>`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "convert", "-o", filepath.Join(t.TempDir(), "out.pdf"), path)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if got := strings.Count(stderr, "flex"); got != 1 {
		t.Fatalf("the flex diagnostic appears %d times:\n%s", got, stderr)
	}
	if got := strings.Count(stderr, "notacolour"); got != 1 {
		t.Fatalf("the colour diagnostic appears %d times:\n%s", got, stderr)
	}
	if lines := strings.Count(strings.TrimSpace(stderr), "\n") + 1; lines != 3 {
		t.Fatalf("stderr has %d lines, want 3 (two diagnostics and one summary):\n%s", lines, stderr)
	}
	if !strings.Contains(stderr, "template is invalid") {
		t.Fatalf("the summary line is missing:\n%s", stderr)
	}
}
