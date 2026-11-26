package css

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
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
// to @media: a page margin box counts as a nesting level too. A margin box is
// valid only as a direct child of @page, so the valid prefix nests @page and
// its margin box under a chain of media rules and the boundary is exactly
// MaxAtRuleNesting levels.
func TestAtRuleNestingLimitCoversMarginBoxes(t *testing.T) {
	atDepth := func(mediaDepth int) []byte {
		return []byte(strings.Repeat("@media { ", mediaDepth) +
			`@page { @top-center { content: "x" } }` + strings.Repeat(" }", mediaDepth))
	}
	// mediaDepth + @page + the margin box == MaxAtRuleNesting.
	ok := Parse(atDepth(MaxAtRuleNesting - 2))
	if len(ok.Problems) != 0 {
		t.Fatalf("%d levels produced problems: %v", MaxAtRuleNesting, ok.Problems)
	}
	over := Parse(atDepth(MaxAtRuleNesting - 1))
	if len(over.Problems) != 1 || over.Problems[0].Code != "resource/limit" {
		t.Fatalf("one level over the cap produced %v, want one resource/limit", over.Problems)
	}
}

// TestMarginBoxRequiresPageParent guards RV5-24: a page margin box is a
// construct of @page only. Outside it the block is rejected as
// css/unsupported-at-rule and contributes no rule, so its content cannot be
// painted; directly inside @page it is still accepted.
func TestMarginBoxRequiresPageParent(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		code   string
		reject string
	}{
		{"top level", `@top-center { content: "HEADER" }`, "css/unsupported-at-rule", "top-center"},
		{"inside media", `@media print { @top-center { content: "HEADER" } }`, "css/unsupported-at-rule", "top-center"},
		{"inside margin box", `@page { @top-center { @bottom-left { content: "X" } } }`, "css/unsupported-at-rule", "bottom-left"},
		{"inside page", `@page { @top-center { content: "HEADER" } }`, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := Parse([]byte(tc.src))
			if tc.code == "" {
				if len(sh.Problems) != 0 {
					t.Fatalf("problems = %v, want none", sh.Problems)
				}
				printRules := RulesForPrint([]*Stylesheet{sh})
				if len(printRules) != 2 || printRules[0].Kind != PageRule ||
					printRules[1].Kind != PageMarginRule || printRules[1].Name != "top-center" {
					t.Fatalf("a margin box inside @page was lost: %+v", printRules)
				}
				return
			}
			if len(sh.Problems) != 1 || sh.Problems[0].Code != tc.code {
				t.Fatalf("problems = %v, want one %s", sh.Problems, tc.code)
			}
			for _, r := range RulesForPrint([]*Stylesheet{sh}) {
				if r.Kind == PageMarginRule && r.Name == tc.reject {
					t.Fatalf("a rejected margin box still contributed a rule: %+v", r)
				}
			}
		})
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

// TestProblemCapBoundsStrayBraces guards RV4-02: a megabyte of stray braces
// used to record one problem per byte, with a diagnostic and an error line for
// each. The sheet now stops at MaxProblems with one resource/limit, and the
// memory stays proportional to that cap rather than to the input.
func TestProblemCapBoundsStrayBraces(t *testing.T) {
	src := []byte(strings.Repeat("}", 1<<20))
	sh := Parse(src)
	if len(sh.Problems) != MaxProblems+1 {
		t.Fatalf("the sheet recorded %d problems, want %d", len(sh.Problems), MaxProblems+1)
	}
	last := sh.Problems[len(sh.Problems)-1]
	if last.Code != "resource/limit" {
		t.Fatalf("the last problem is %q, want resource/limit", last.Code)
	}
	allocs := testing.AllocsPerRun(1, func() {
		Parse(src)
	})
	if allocs > 200 {
		t.Fatalf("a megabyte of stray braces allocated %v objects", allocs)
	}
}

// TestProblemCapCountsEachProblemOnce guards RV5-25: a problem found inside a
// nested block was counted when it was recorded and again when the block was
// merged, so a sheet with sixty problems stopped at forty and reported
// resource/limit. Each problem must count once, in a flat and in a nested rule
// alike.
func TestProblemCapCountsEachProblemOnce(t *testing.T) {
	decls := strings.Repeat("broken; ", 60)
	cases := []struct {
		name string
		src  string
	}{
		{"one rule", "p { " + decls + "}"},
		{"one media block", "@media print { p { " + decls + "} }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := Parse([]byte(tc.src))
			if len(sh.Problems) != 60 {
				t.Fatalf("problems = %d, want 60: %v", len(sh.Problems), sh.Problems)
			}
			for _, p := range sh.Problems {
				if p.Code == "resource/limit" {
					t.Fatalf("60 problems hit the cap: %v", sh.Problems)
				}
			}
		})
	}
}

// TestProblemCapCountsEachProblemOnceAcrossTheCap guards the other side: a
// sheet with more distinct problems than the cap still stores the cap plus one
// resource/limit, so merging a nested block cannot grow the list either.
func TestProblemCapCountsEachProblemOnceAcrossTheCap(t *testing.T) {
	sh := Parse([]byte("@media print { p { " + strings.Repeat("broken; ", MaxProblems+20) + "} }"))
	if len(sh.Problems) != MaxProblems+1 {
		t.Fatalf("problems = %d, want %d", len(sh.Problems), MaxProblems+1)
	}
	last := sh.Problems[len(sh.Problems)-1]
	if last.Code != "resource/limit" || !strings.Contains(last.Message, strconv.Itoa(MaxProblems)) {
		t.Fatalf("last problem = %+v, want resource/limit naming the cap", last)
	}
}

// TestGuardStopsTheTokenizer guards the charging half of RV4-02: the tokenizer
// spends the conversion's budget and stops with a single resource/limit.
func TestGuardStopsTheTokenizer(t *testing.T) {
	g := budget.New(context.Background(), 1000)
	sh := ParseSheet(SheetOptions{Guard: g}, []byte(strings.Repeat("a{", 10000)))
	if len(sh.Problems) != 1 || sh.Problems[0].Code != "resource/limit" {
		t.Fatalf("problems = %v, want one resource/limit", sh.Problems)
	}
	if sh.Rules != nil {
		t.Fatalf("a stopped parse kept %d rules", len(sh.Rules))
	}
}
