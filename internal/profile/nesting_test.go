package profile

import (
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/diag"
)

// TestNestedAtRuleLimitReachesValidation guards that the parser's nesting cap
// keeps its own code through validation: a deep stylesheet is resource/limit,
// not a generic css/syntax error, and validation itself does not crash.
func TestNestedAtRuleLimitReachesValidation(t *testing.T) {
	depth := css.MaxAtRuleNesting + 1
	src := "<style>" + strings.Repeat("@media { ", depth) + "p { color: #000 }" +
		strings.Repeat(" }", depth) + "</style><p>x</p>"
	diags := Validate(parseDoc(t, src), Options{})
	if !hasDiagCode(diags, "resource/limit") {
		t.Fatalf("deep nesting was not reported as resource/limit: %v", diags.Lines())
	}
	for _, d := range diags {
		if d.Code == "css/syntax" {
			t.Fatalf("the limit was reported as css/syntax: %v", diags.Lines())
		}
	}
}

// TestValidateRulesWalksDeepTrees guards that validation walks with an explicit
// stack: a tree the parser would never build is still validated instead of
// recursing once per level.
func TestValidateRulesWalksDeepTrees(t *testing.T) {
	rule := css.Rule{Kind: css.UnknownAtRule, Name: "weird"}
	for i := 0; i < 50_000; i++ {
		rule = css.Rule{Kind: css.MediaRule, Name: "print", Rules: []css.Rule{rule}}
	}
	var out diag.List
	validateRules(parseDoc(t, `<p>x</p>`), []css.Rule{rule}, &out)
	if !hasDiagCode(out, "css/unsupported-at-rule") {
		t.Fatalf("the innermost rule was not reached: %v", out.Lines())
	}
}
