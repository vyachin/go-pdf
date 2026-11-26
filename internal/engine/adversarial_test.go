package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/pdfdebug"
)

// adversarialCase is one confirmed hostile or pathological document. Want is
// the documented diagnostic code; an empty Want means the conversion must
// succeed and Check (when set) asserts the bounded outcome. The budget, never
// elapsed time, is the pass criterion.
type adversarialCase struct {
	ID      string
	Fixture string
	Want    string
	Options func(t *testing.T, dir string) Options
	Check   func(t *testing.T, res Result)
}

func adversarialDir() string { return filepath.Join(repoRoot(), "testdata", "adversarial") }

func adversarialCases() []adversarialCase {
	return []adversarialCase{
		{
			ID:      "P1-nested-media",
			Fixture: "p1-nested-media.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options { return Options{File: "nested-media.html"} },
		},
		{
			ID:      "P1-adjacent-selector",
			Fixture: "p1-adjacent-selector.html",
			Options: func(t *testing.T, dir string) Options {
				return Options{BaseDir: dir, File: "adjacent.html", FallbackFont: testFontPath(t)}
			},
			Check: func(t *testing.T, res Result) {
				for _, d := range res.Diagnostics {
					if d.Code == "css/unsupported-selector" {
						t.Fatalf("the adjacent selector is rejected: %s", d.String())
					}
				}
				if !bytes.HasPrefix(res.PDF, []byte("%PDF-")) {
					t.Fatal("no PDF produced")
				}
				// The second paragraph is red, so the adjacent rule applied, not
				// merely parsed.
				var streams bytes.Buffer
				for _, stream := range pdfdebug.ContentStreams(res.PDF) {
					streams.Write(stream)
					streams.WriteByte('\n')
				}
				if !bytes.Contains(streams.Bytes(), []byte("1 0 0 rg")) {
					t.Fatal("the adjacent selector did not colour the second paragraph")
				}
			},
		},
		{
			ID:      "P1-many-spelling-image",
			Fixture: "p1-many-spellings.html",
			Options: func(t *testing.T, dir string) Options {
				// The fixture is one long line, so the charged positions and
				// paths need the one pass over it that the work budget now
				// counts.
				return Options{BaseDir: dir, File: "spellings.html", Limits: Limits{MaxOperations: 8000}}
			},
			Check: func(t *testing.T, res Result) {
				if n := bytes.Count(res.PDF, []byte("/Subtype /Image")); n != 1 {
					t.Fatalf("the PDF embeds %d image objects, want 1", n)
				}
			},
		},
		{
			ID:      "P1-crafted-gpos",
			Fixture: "p1-crafted-gpos.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{
					FallbackFont: filepath.Join(dir, "gpos-bomb.ttf"),
					File:         "gpos.html",
					Limits:       Limits{MaxOperations: 250},
				}
			},
		},
		{
			// RV4-01: 1024 covered glyphs share one 65,535-pair PairSet, so the
			// map would need 67 million entries from a 264 KB file. The
			// declared-pair cap refuses it under the default limits.
			ID:      "P1-gpos-pairs",
			Fixture: "p1-gpos-pairs.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{FallbackFont: filepath.Join(dir, "gpos-pairs.ttf"), File: "pairs.html"}
			},
		},
		{
			ID:      "P2-wide-image",
			Fixture: "p2-wide-image.html",
			Options: func(t *testing.T, dir string) Options {
				return Options{BaseDir: dir, File: "wide.html"}
			},
			Check: func(t *testing.T, res Result) {
				for _, stream := range pdfdebug.ContentStreams(res.PDF) {
					if bytes.Contains(stream, []byte("q -")) {
						t.Fatalf("the image scale is negative: %q", stream)
					}
				}
			},
		},
		{
			// RV4-07: 5000 sibling paragraphs matched by `p + p`. The hop to
			// the preceding element sibling is constant time, and the cascade
			// charges per compound, so a small budget stops the conversion
			// instead of scanning every preceding sibling per element.
			ID:      "P2-adjacent-siblings",
			Fixture: "p2-adjacent-siblings.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "adjacent.html", FallbackFont: testFontPath(t), Limits: Limits{MaxOperations: 3000}}
			},
		},
		{
			// RV4-06: 20,000 serialised glyphs. The writer charges every
			// glyph, CID and serialised byte, so a small budget stops it
			// instead of writing a multi-megabyte document.
			ID:      "P2-many-glyphs",
			Fixture: "p2-many-glyphs.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "glyphs.html", FallbackFont: testFontPath(t), Limits: Limits{MaxOperations: 5000}}
			},
		},
		{
			// RV4-05: a 394 KB OTTO font whose 65,535 charstrings call a
			// ten-deep subroutine chain that draws a hundred curves each. The
			// conversion charges every instruction, so the budget stops it.
			ID:      "P2-cff-bomb",
			Fixture: "p2-cff-bomb.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{
					FallbackFont: filepath.Join(dir, "cff-bomb.otf"),
					File:         "cff.html",
					Limits:       Limits{MaxOperations: 200_000},
				}
			},
		},
		{
			// RV4-04: one word of 4000 characters. Shaping charges one unit per
			// rune, so the small budget stops it before the glyph and state
			// slices are built.
			ID:      "P2-huge-word",
			Fixture: "p2-huge-word.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "word.html", FallbackFont: testFontPath(t), Limits: Limits{MaxOperations: 1000}}
			},
		},
		{
			// RV4-02: 4096 stray braces would be 4096 problems without the
			// per-sheet cap; the parser stops at MaxProblems with one
			// resource/limit.
			ID:      "P2-stray-braces",
			Fixture: "p2-stray-braces.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options { return Options{File: "stray.html"} },
		},
		{
			ID:      "P2-dom-depth",
			Fixture: "p2-dom-depth.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "depth.html", Limits: Limits{MaxDOMDepth: 8}}
			},
		},
		{
			// RV4-03: 60 nested break-inside: avoid blocks of 30 words. Every
			// level re-measures its subtree, so without a width-keyed line cache
			// the cost is quadratic in the depth; a small budget must stop it
			// without spending that time.
			ID:      "P2-nested-avoid",
			Fixture: "p2-nested-avoid.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "avoid.html", Limits: Limits{MaxOperations: 2000}}
			},
		},
		{
			ID:      "P2-rules-elements",
			Fixture: "p2-rules-elements.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "rules.html", Limits: Limits{MaxOperations: 2000}}
			},
		},
		{
			ID:      "P2-repeated-data-uri",
			Fixture: "p2-repeated-data-uri.html",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "datauri.html", Limits: Limits{MaxOperations: 5000}}
			},
			Check: func(t *testing.T, res Result) {
				if n := bytes.Count(res.PDF, []byte("/Subtype /Image")); n != 1 {
					t.Fatalf("the PDF embeds %d image objects, want 1", n)
				}
			},
		},
		{
			ID:      "P2-page-breaks",
			Fixture: "p2-page-breaks.html",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "breaks.html", FallbackFont: testFontPath(t)}
			},
			Check: func(t *testing.T, res Result) {
				pages, err := pdfdebug.Pages(res.PDF)
				if err != nil {
					t.Fatal(err)
				}
				if len(pages) != 2 {
					t.Fatalf("the document produced %d pages, want 2", len(pages))
				}
			},
		},
		{
			ID:      "P2-long-list",
			Fixture: "p2-long-list.html",
			// Indexing a list is linear and charged: a small budget stops it
			// instead of the quadratic prefix scan the review measured.
			Want: "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "long-list.html", Limits: Limits{MaxOperations: 500}}
			},
		},
		{
			ID:      "P2-image-placements",
			Fixture: "p2-image-placements.html",
			// The same image placed four hundred times is hashed and embedded
			// once: the digest is per loaded record, not per placement.
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "placements.html", BaseDir: dir, FallbackFont: testFontPath(t)}
			},
			Check: func(t *testing.T, res Result) {
				if n := strings.Count(string(res.PDF), "/Subtype /Image"); n != 1 {
					t.Fatalf("the repeated image produced %d image objects, want 1", n)
				}
			},
		},
		{
			ID:      "P2-table-grid",
			Fixture: "p2-table-grid.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{File: "grid.html", Limits: Limits{MaxTableCells: 4}}
			},
		},
		{
			ID:      "P1-entity-fragments",
			Fixture: "p1-entity-fragments.html",
			Options: func(t *testing.T, dir string) Options {
				return Options{BaseDir: dir, File: "entity-fragments.html", FallbackFont: testFontPath(t)}
			},
			Check: func(t *testing.T, res Result) {
				if !bytes.HasPrefix(res.PDF, []byte("%PDF-")) {
					t.Fatal("no PDF produced")
				}
				for _, d := range res.Diagnostics {
					if d.Code == "resource/limit" {
						t.Fatalf("the entity fragments were limited: %s", d.String())
					}
				}
			},
		},
		{
			ID:      "P1-fostered-texts",
			Fixture: "p1-fostered-texts.html",
			Want:    "resource/limit",
			Options: func(t *testing.T, dir string) Options {
				return Options{
					BaseDir: dir,
					File:    "fostered-texts.html",
					Limits:  Limits{MaxOperations: 13_000},
				}
			},
		},
		{
			ID:      "P1-trailing-backslash-style",
			Fixture: "p1-trailing-backslash-style.html",
			Want:    "css/syntax",
			Options: func(t *testing.T, dir string) Options {
				return Options{BaseDir: dir, File: "trailing-style.html"}
			},
		},
		{
			ID:      "P1-trailing-backslash-sheet",
			Fixture: "p1-trailing-backslash-sheet.html",
			Want:    "css/syntax",
			Options: func(t *testing.T, dir string) Options {
				return Options{BaseDir: dir, File: "trailing-sheet.html"}
			},
		},
	}
}

// TestAdversarialManifest guards the corpus: every case names an existing
// fixture, and every HTML fixture is registered, so a new limit or construct
// cannot ship without a fixture.
func TestAdversarialManifest(t *testing.T) {
	dir := adversarialDir()
	listed := map[string]bool{}
	for _, tc := range adversarialCases() {
		listed[tc.Fixture] = true
		path := filepath.Join(dir, tc.Fixture)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("fixture %s (%s) is missing: %v", tc.ID, tc.Fixture, err)
		}
		if info.Size() == 0 {
			t.Fatalf("fixture %s (%s) is empty", tc.ID, tc.Fixture)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		if !listed[entry.Name()] {
			t.Errorf("fixture %s is not registered in the adversarial corpus", entry.Name())
		}
	}
}

// TestAdversarialCorpus runs every case. The conversion is bounded by a small
// operation budget, and the assertion is the diagnostic code or the bounded
// output, never a duration.
func TestAdversarialCorpus(t *testing.T) {
	dir := adversarialDir()
	for _, tc := range adversarialCases() {
		tc := tc
		t.Run(tc.ID, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, tc.Fixture))
			if err != nil {
				t.Fatalf("reading the fixture: %v", err)
			}
			res, err := Convert(context.Background(), src, tc.Options(t, dir))
			if tc.Want != "" {
				if !hasCode(res.Diagnostics, tc.Want) {
					t.Fatalf("want %s, got err=%v diags:\n%s", tc.Want, err, strings.Join(res.Diagnostics.Lines(), "\n"))
				}
				return
			}
			if err != nil {
				t.Fatalf("conversion failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
			}
			if tc.Check != nil {
				tc.Check(t, res)
			}
		})
	}
}

// TestAdversarialSubprocess is the child of the fatal-error helper. It only
// acts when the parent sets the environment variable, so a normal test run does
// not convert the fixture in-process: the failure mode was a fatal stack
// overflow that no recover could stop.
func TestAdversarialSubprocess(t *testing.T) {
	if os.Getenv("PDF_ADVERSARIAL_CHILD") == "" {
		return
	}
	src, err := os.ReadFile(filepath.Join(adversarialDir(), "p1-nested-media.html"))
	if err != nil {
		fmt.Printf("reading the fixture: %v\n", err)
		os.Exit(1)
	}
	res, _ := Convert(context.Background(), src, Options{File: "nested-media.html"})
	for _, d := range res.Diagnostics {
		fmt.Println(d.Code)
	}
	os.Exit(0)
}

// TestAdversarialFatalErrorRunsInSubprocess runs the process-killing fixture in
// a subprocess and requires a normal exit with the diagnostic. A non-zero exit
// is a failure: the work guard must stop the work, not the runtime.
func TestAdversarialFatalErrorRunsInSubprocess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestAdversarialSubprocess$")
	cmd.Env = append(os.Environ(), "PDF_ADVERSARIAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the subprocess did not exit normally: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "resource/limit") {
		t.Fatalf("the subprocess reported no resource/limit:\n%s", out)
	}
}
