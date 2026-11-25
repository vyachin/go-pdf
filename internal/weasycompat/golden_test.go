package weasycompat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckGoldenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	artifacts := t.TempDir()
	c := Case{Name: "sample"}
	pages := []string{"page one", "page two"}

	if err := checkGolden(dir, artifacts, c, pages, true); err != nil {
		t.Fatalf("update: %v", err)
	}
	committed, err := os.ReadFile(goldenPath(dir, "sample"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "page one\f\npage two"; string(committed) != want {
		t.Fatalf("golden = %q, want %q", committed, want)
	}
	if err := checkGolden(dir, artifacts, c, pages, false); err != nil {
		t.Fatalf("compare with the fresh golden: %v", err)
	}
}

func TestCheckGoldenReportsTheFirstDifference(t *testing.T) {
	dir := t.TempDir()
	artifacts := t.TempDir()
	c := Case{Name: "sample"}
	if err := checkGolden(dir, artifacts, c, []string{"page one", "page two"}, true); err != nil {
		t.Fatalf("update: %v", err)
	}

	err := checkGolden(dir, artifacts, c, []string{"page one", "page TWO"}, false)
	if err == nil {
		t.Fatal("a changed page text was accepted")
	}
	for _, want := range []string{"page 2 line 1", "page TWO", "page two", "-update"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(artifacts, "sample.txt")); statErr != nil {
		t.Fatalf("the produced text was not written for review: %v", statErr)
	}
}

func TestCheckGoldenReportsAPageCountDifference(t *testing.T) {
	dir := t.TempDir()
	artifacts := t.TempDir()
	c := Case{Name: "sample"}
	if err := checkGolden(dir, artifacts, c, []string{"one"}, true); err != nil {
		t.Fatalf("update: %v", err)
	}
	err := checkGolden(dir, artifacts, c, []string{"one", "two"}, false)
	if err == nil || !strings.Contains(err.Error(), "page count") {
		t.Fatalf("error = %v, want a page count difference", err)
	}
}

func TestCheckGoldenNeedsAGolden(t *testing.T) {
	dir := t.TempDir()
	artifacts := t.TempDir()
	err := checkGolden(dir, artifacts, Case{Name: "absent"}, []string{"x"}, false)
	if err == nil || !strings.Contains(err.Error(), "-update") {
		t.Fatalf("error = %v, want the regeneration command", err)
	}
}

func TestCheckGoldenUpdatesOnlyTheProcessedCase(t *testing.T) {
	dir := t.TempDir()
	artifacts := t.TempDir()
	other := Case{Name: "other"}
	if err := checkGolden(dir, artifacts, other, []string{"keep me"}, true); err != nil {
		t.Fatalf("update: %v", err)
	}
	before, err := os.ReadFile(goldenPath(dir, "other"))
	if err != nil {
		t.Fatal(err)
	}

	if err := checkGolden(dir, artifacts, Case{Name: "sample"}, []string{"new"}, true); err != nil {
		t.Fatalf("update: %v", err)
	}
	after, err := os.ReadFile(goldenPath(dir, "other"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("an unrelated golden changed: %q -> %q", before, after)
	}
}

func TestCheckStaleGoldens(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{Cases: []Case{{Name: "sample"}}}
	if err := checkStaleGoldens(dir, m); err != nil {
		t.Fatalf("a corpus without a golden directory: %v", err)
	}
	writeFile(t, dir, "golden/sample.txt", "x")
	if err := checkStaleGoldens(dir, m); err != nil {
		t.Fatalf("a golden with a case: %v", err)
	}
	writeFile(t, dir, "golden/ghost.txt", "x")
	err := checkStaleGoldens(dir, m)
	if err == nil {
		t.Fatal("a golden without a case was accepted")
	}
	if !strings.Contains(err.Error(), "ghost.txt") {
		t.Fatalf("error %q does not name the stale file", err)
	}
}
