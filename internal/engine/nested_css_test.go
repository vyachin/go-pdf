package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// nestedStyleChildEnv marks the child process of
// TestDeeplyNestedStyleSubprocess.
const nestedStyleChildEnv = "PDFGEN_NESTED_STYLE_CHILD"

// TestDeeplyNestedStyleSubprocess guards the at-rule nesting limit end to end.
// A sheet of 500,000 nested levels used to die with a fatal stack overflow,
// which runtime.throw cannot recover, so the input runs in a child process: the
// parent asserts a normal exit with exactly the resource/limit diagnostic, not a
// crash and not a cascade of syntax errors.
func TestDeeplyNestedStyleSubprocess(t *testing.T) {
	if os.Getenv(nestedStyleChildEnv) == "1" {
		deepStyleChild()
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestDeeplyNestedStyleSubprocess$")
	cmd.Env = append(os.Environ(), nestedStyleChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the nested stylesheet killed the process (exit %v), want a diagnostic:\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "resource/limit") {
		t.Fatalf("the nested stylesheet did not report resource/limit:\n%s", text)
	}
	if !strings.Contains(text, "diagnostics: 1") {
		t.Fatalf("the nested stylesheet did not report exactly one diagnostic:\n%s", text)
	}
	if strings.Contains(text, "unclosed block") {
		t.Fatalf("the rejected sheet reported a cascade of syntax errors:\n%s", text)
	}
}

// deepStyleChild runs in the subprocess. The expected conversion error is part
// of the result, not a test failure: the child reports it and exits 0 so the
// parent can assert the diagnostic.
func deepStyleChild() {
	// 500,000 levels of a valid @media query is 6.5 MB, below the default
	// 8 MiB input limit, and deeper than any stack can hold.
	src := []byte("<style>" + strings.Repeat("@media print{", 500_000) + "</style><p>x</p>")
	res, err := Convert(context.Background(), src, Options{File: "nested.html"})
	fmt.Printf("diagnostics: %d\n", len(res.Diagnostics))
	for _, line := range res.Diagnostics.Lines() {
		fmt.Println(line)
	}
	if err != nil {
		fmt.Println("conversion error:", err)
	}
}
