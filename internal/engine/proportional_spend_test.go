package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// spendAlloc measures the bytes a conversion allocates. Timings are never an
// assertion: the counters and the allocation bound decide pass or fail.
func spendAlloc(fn func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// spendStage is one byte-proportional stage of the pipeline: a hostile input
// builder, a budget below its next charge and a budget that admits its work.
type spendStage struct {
	name        string
	sizes       []int
	stopBudget  int
	admitBudget int
	build       func(t *testing.T, size int) []byte
	options     func(t *testing.T) Options
}

// TestProportionalSpendMatrix guards RV5-11's invariant over the stages whose
// memory grows with the input: at a fixed budget the allocated bytes do not
// follow the input size, and at a fixed input they grow with the admitted
// units. The documented bound is one unit per 4096 bytes plus the fixed
// pipeline overhead; a failure names the stage.
func TestProportionalSpendMatrix(t *testing.T) {
	const (
		bytesPerUnit  = 4096
		fixedOverhead = 8 << 20
	)
	stages := []spendStage{
		{
			// The line index is built from the decoded document before the
			// scanner runs, so a stylesheet made of newlines is decided there.
			name:        "line-index",
			sizes:       []int{20_000, 200_000},
			stopBudget:  1_000,
			admitBudget: 4_000,
			build: func(t *testing.T, size int) []byte {
				return []byte("<style>" + strings.Repeat("\n", size) + "</style><p>x</p>")
			},
			options: func(t *testing.T) Options {
				return Options{File: "line-index.html", FallbackFont: testFontPath(t)}
			},
		},
		{
			// A legacy document materialises a decoded copy and its width
			// table, charged before the buffers exist.
			name:        "decoded-text",
			sizes:       []int{1 << 20, 4 << 20},
			stopBudget:  100,
			admitBudget: 4_000,
			build: func(t *testing.T, size int) []byte {
				return []byte("<p>" + strings.Repeat("x", size) + "</p>")
			},
			options: func(t *testing.T) Options {
				return Options{File: "decoded.html", Charset: "windows-1251", FallbackFont: testFontPath(t)}
			},
		},
	}
	for _, stage := range stages {
		stage := stage
		t.Run(stage.name, func(t *testing.T) {
			check := func(label string, src []byte, opts Options, budget int) uint64 {
				t.Helper()
				opts.Limits.MaxOperations = budget
				var res Result
				alloc := spendAlloc(func() {
					res, _ = Convert(context.Background(), src, opts)
				})
				if !hasCode(res.Diagnostics, "resource/limit") {
					t.Fatalf("%s: the %d-unit budget did not stop the document: %v", label, budget, res.Diagnostics.Lines())
				}
				if limit := uint64(budget)*bytesPerUnit + fixedOverhead; alloc > limit {
					t.Fatalf("%s stage allocated %d bytes at a %d-unit budget (limit %d)", stage.name, alloc, budget, limit)
				}
				return alloc
			}
			// A fixed budget with a growing input: allocation stays within the
			// bound instead of following the input size.
			first := stage.build(t, stage.sizes[0])
			firstAlloc := check("fixed budget", first, stage.options(t), stage.stopBudget)
			for _, size := range stage.sizes[1:] {
				check("fixed budget", stage.build(t, size), stage.options(t), stage.stopBudget)
			}
			// A fixed input with a growing budget: the stage completes more
			// work and the allocation grows with the admitted units.
			admittedAlloc := check("increasing budget", first, stage.options(t), stage.admitBudget)
			if admittedAlloc <= firstAlloc {
				t.Fatalf("%s stage allocated %d bytes at the larger budget and %d at the smaller one", stage.name, admittedAlloc, firstAlloc)
			}
			// Validate shares preparation and layout, so it must report the
			// same limit and must not run the writer.
			opts := stage.options(t)
			opts.Limits.MaxOperations = stage.stopBudget
			diags, err := Validate(context.Background(), first, opts)
			if err != nil {
				t.Fatalf("Validate returned an error: %v", err)
			}
			if !hasCode(diags, "resource/limit") {
				t.Fatalf("%s: Validate did not report the limit Convert reported: %v", stage.name, diags.Lines())
			}
		})
	}
}

// TestWriterOnlyCostIsSeparate guards the pipeline split: a document that only
// the writer's budget refuses passes Validate, and Convert reports the limit.
func TestWriterOnlyCostIsSeparate(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(adversarialDir(), "p2-many-glyphs.html"))
	if err != nil {
		t.Fatal(err)
	}
	// The exact crossing depends on the charges of the stages before the
	// writer, so try a few budgets and require at least one that separates the
	// two entry points. A budget that also stops Validate is recalibration, not
	// evidence about the writer.
	separated := false
	for _, max := range []int{20_000, 30_000, 40_000} {
		opts := Options{File: "glyphs.html", FallbackFont: testFontPath(t), Limits: Limits{MaxOperations: max}}
		diags, err := Validate(context.Background(), src, opts)
		if err != nil {
			t.Fatal(err)
		}
		if hasCode(diags, "resource/limit") {
			continue
		}
		res, err := Convert(context.Background(), src, opts)
		if err == nil || !hasCode(res.Diagnostics, "resource/limit") {
			t.Fatalf("MaxOperations=%d: the writer did not report the limit: err=%v diags=%v", max, err, res.Diagnostics.Lines())
		}
		separated = true
	}
	if !separated {
		t.Fatal("no budget let Validate pass and Convert stop in the writer")
	}
}
