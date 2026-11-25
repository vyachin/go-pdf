package css

import (
	"strconv"
	"strings"
	"testing"
)

// nestedAtRules renders depth nested @media blocks around one style rule.
func nestedAtRules(depth int) []byte {
	return []byte(strings.Repeat("@media { ", depth) + "p { color: #000 }" + strings.Repeat(" }", depth))
}

// TestAtRuleNestingLimitAcceptsTheLimit guards the boundary: exactly
// MaxAtRuleNesting levels are valid and the rules inside them still apply.
func TestAtRuleNestingLimitAcceptsTheLimit(t *testing.T) {
	sh := Parse(nestedAtRules(MaxAtRuleNesting))
	if len(sh.Problems) != 0 {
		t.Fatalf("%d levels produced problems: %v", MaxAtRuleNesting, sh.Problems)
	}
	printRules := RulesForPrint([]*Stylesheet{sh})
	if len(printRules) != 1 || printRules[0].Kind != StyleRule {
		t.Fatalf("%d levels lost the style rule: %+v", MaxAtRuleNesting, printRules)
	}
}

// TestAtRuleNestingLimitRejectsOneLevelMore guards the cap itself: the level
// beyond the limit is rejected with resource/limit before its body is parsed,
// so the tree the later walks receive is bounded.
func TestAtRuleNestingLimitRejectsOneLevelMore(t *testing.T) {
	sh := Parse(nestedAtRules(MaxAtRuleNesting + 1))
	if len(sh.Problems) != 1 {
		t.Fatalf("%d levels produced %d problems, want 1: %v",
			MaxAtRuleNesting+1, len(sh.Problems), sh.Problems)
	}
	p := sh.Problems[0]
	if p.Code != "resource/limit" {
		t.Fatalf("problem code = %q, want resource/limit (%v)", p.Code, p.Message)
	}
	if !strings.Contains(p.Message, strconv.Itoa(MaxAtRuleNesting)) {
		t.Fatalf("the message does not name the limit: %q", p.Message)
	}
	// The rejected level's body was skipped, so the rule behind it is gone and
	// the returned tree never nests deeper than the limit.
	if printRules := RulesForPrint([]*Stylesheet{sh}); len(printRules) != 0 {
		t.Fatalf("the rejected level still contributed rules: %+v", printRules)
	}
}

// TestAtRuleNestingLimitCoversMarginBoxes guards that the cap is not specific
// to @media: a margin box inside @page counts as a nesting level too.
func TestAtRuleNestingLimitCoversMarginBoxes(t *testing.T) {
	nested := strings.Repeat("@top-center {", MaxAtRuleNesting) + strings.Repeat("}", MaxAtRuleNesting)
	sh := Parse([]byte("@page { " + nested + " }"))
	if len(sh.Problems) != 1 || sh.Problems[0].Code != "resource/limit" {
		t.Fatalf("nested margin boxes were not bounded: %v", sh.Problems)
	}
}

// TestUnclosedDeepSheetReportsOneProblem guards the stop: an unclosed sheet of
// 500,000 levels used to add one "unclosed block" problem per enclosing level
// on top of the limit, and the reported diagnostic must be the limit alone.
func TestUnclosedDeepSheetReportsOneProblem(t *testing.T) {
	// 20,000 levels is enough to exercise the cascade without paying for the
	// 4.5 MB input in every unit run; the subprocess test covers that size.
	sh := Parse([]byte(strings.Repeat("@media {\n", 20_000)))
	if len(sh.Problems) != 1 || sh.Problems[0].Code != "resource/limit" {
		t.Fatalf("problems = %d, want one resource/limit: %v", len(sh.Problems), sh.Problems)
	}
}
