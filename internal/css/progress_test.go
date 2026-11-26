package css

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/diag"
)

// hasSyntaxProblem reports whether problems contain a plain syntax problem: the
// profile reports a Problem with an empty Code as css/syntax.
func hasSyntaxProblem(problems []Problem) bool {
	for _, pr := range problems {
		if pr.Code == "" {
			return true
		}
	}
	return false
}

func hasCode(problems []Problem, code string) bool {
	for _, pr := range problems {
		if pr.Code == code {
			return true
		}
	}
	return false
}

// TestTokenizerProgress guards RV5-02: no input can leave the tokenizer on the
// same offset. A trailing lone backslash is the known case; it must terminate
// with a syntax problem instead of looping.
func TestTokenizerProgress(t *testing.T) {
	cases := []string{"\\", "color:red\\", strings.Repeat("\\", 100)}
	for _, src := range cases {
		decls, problems := ParseDeclarations(src)
		if len(decls) != 0 {
			t.Fatalf("%q: declarations = %v, want none", src, decls)
		}
		if !hasSyntaxProblem(problems) {
			t.Fatalf("%q: problems = %+v, want a syntax problem", src, problems)
		}
	}
	// A valid escaped identifier is not a syntax error.
	decls, problems := ParseDeclarations(`content: "\2014"; font-family: caf\e9`)
	if hasSyntaxProblem(problems) {
		t.Fatalf("a valid escape reported %+v", problems)
	}
	if len(decls) != 2 {
		t.Fatalf("declarations = %v, want 2", decls)
	}
}

// TestTokenizerTrailingBackslashStops checks the lexer directly: it returns the
// tokens before the escape and a syntax error at the backslash.
func TestTokenizerTrailingBackslashStops(t *testing.T) {
	toks, err := TokenizeWithGuard([]byte("color:red\\"), nil)
	if err == nil {
		t.Fatal("want a syntax error")
	}
	var syn *syntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("error = %v, want *syntaxError", err)
	}
	if want := len("color:red"); syn.Offset != want {
		t.Fatalf("offset = %d, want %d", syn.Offset, want)
	}
	if len(toks) != 3 {
		t.Fatalf("tokens = %v, want the three before the escape", toks)
	}
}

// TestDeclarationsChargeTheGuard guards the declaration half: tokenization spends
// the shared budget and a tiny budget stops with resource/limit.
func TestDeclarationsChargeTheGuard(t *testing.T) {
	src := strings.Repeat("color:red;", 50)
	guard := budget.New(context.Background(), 0)
	decls, problems := ParseDeclarationsWithGuard(src, guard)
	if len(problems) != 0 {
		t.Fatalf("problems = %+v", problems)
	}
	if len(decls) != 50 {
		t.Fatalf("declarations = %d, want 50", len(decls))
	}
	if guard.Used() == 0 {
		t.Fatal("declaration parsing recorded no work")
	}
	tiny := budget.New(context.Background(), 3)
	decls, problems = ParseDeclarationsWithGuard(src, tiny)
	if len(decls) != 0 {
		t.Fatalf("a 3-unit budget produced %d declarations", len(decls))
	}
	if !hasCode(problems, string(diag.ResourceLimit)) {
		t.Fatalf("problems = %+v, want resource/limit", problems)
	}
}

// TestDeclarationsCancellation guards that a cancelled context stops declaration
// parsing; the reason stays on the guard for the caller to report.
func TestDeclarationsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	guard := budget.New(ctx, 0)
	decls, problems := ParseDeclarationsWithGuard("color:red", guard)
	if len(decls) != 0 {
		t.Fatalf("declarations = %v, want none", decls)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %+v, want no document diagnostic", problems)
	}
	if !errors.Is(guard.Err(), context.Canceled) {
		t.Fatalf("guard error = %v, want context.Canceled", guard.Err())
	}
}
