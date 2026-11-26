package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
)

// copyFontFile copies the bundled test font to dir/name and returns the path.
func copyFontFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFallbackAggregateLimitIsResourceLimit guards the review reproduction: an
// explicit fallback font refused by MaxTotalFontBytes must end the conversion
// with resource/limit, not with the missing-family text/font diagnostic.
func TestFallbackAggregateLimitIsResourceLimit(t *testing.T) {
	dir := t.TempDir()
	path := copyFontFile(t, dir, "fallback.ttf")
	src := []byte(`<p>Text in the fallback font</p>`)
	opts := Options{File: "limit.html", FallbackFonts: []string{path}, Limits: Limits{MaxTotalFontBytes: 1}}
	res, err := Convert(context.Background(), src, opts)
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") {
		t.Fatalf("Convert: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	for _, d := range res.Diagnostics {
		if d.Code == "text/font" || d.Code == "font/unavailable" {
			t.Fatalf("a dependent missing-family diagnostic escaped: %s", d.String())
		}
	}
	diags, verr := Validate(context.Background(), src, opts)
	if verr != nil {
		t.Fatal(verr)
	}
	if !hasCode(diags, "resource/limit") {
		t.Fatalf("Validate missed the limit: %v", diags.Lines())
	}
	if hasCode(diags, "text/font") || hasCode(diags, "font/unavailable") {
		t.Fatalf("Validate reported a dependent missing-family diagnostic: %v", diags.Lines())
	}
}

// TestFallbackPerFileLimitIsResourceLimit guards the per-file bound: the same
// refusal, named by MaxFontBytes.
func TestFallbackPerFileLimitIsResourceLimit(t *testing.T) {
	dir := t.TempDir()
	path := copyFontFile(t, dir, "fallback.ttf")
	res, err := Convert(context.Background(), []byte(`<p>Text</p>`), Options{
		File:          "limit.html",
		FallbackFonts: []string{path},
		Limits:        Limits{MaxFontBytes: 10},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || hasCode(res.Diagnostics, "text/font") {
		t.Fatalf("err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestFallbackVariantLimitIsResourceLimit guards the sibling: the regular face
// fits, the bold file next to it crosses the total, so the family is refused
// instead of silently printing in the regular weight.
func TestFallbackVariantLimitIsResourceLimit(t *testing.T) {
	dir := t.TempDir()
	regular := copyFontFile(t, dir, "Family.ttf")
	copyFontFile(t, dir, "Family Bold.ttf")
	info, err := os.Stat(regular)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), []byte(`<p>Text</p>`), Options{
		File:          "variant.html",
		FallbackFonts: []string{regular},
		Limits:        Limits{MaxTotalFontBytes: int(info.Size()) + int(info.Size())/2},
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || hasCode(res.Diagnostics, "text/font") {
		t.Fatalf("err=%v diags=%v", err, res.Diagnostics.Lines())
	}
}

// TestAbsentFallbackCandidateFallsThrough guards the documented list order: a
// missing first entry is skipped and the next one serves the document.
func TestAbsentFallbackCandidateFallsThrough(t *testing.T) {
	dir := t.TempDir()
	path := copyFontFile(t, dir, "fallback.ttf")
	res, err := Convert(context.Background(), []byte(`<p>Text</p>`), Options{
		File:          "fallback.html",
		FallbackFonts: []string{filepath.Join(dir, "missing.ttf"), path},
	})
	if err != nil {
		t.Fatalf("a missing candidate stopped discovery: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("no PDF produced")
	}
}

// TestFallbackDiscoveryHonoursCancellation guards the context cause: a
// cancellation during discovery is returned as the context error and the
// remaining candidates are not read.
func TestFallbackDiscoveryHonoursCancellation(t *testing.T) {
	dir := t.TempDir()
	path := copyFontFile(t, dir, "fallback.ttf")
	ctx, cancel := context.WithCancel(context.Background())
	guard := budget.New(ctx, 0)
	reg := assets.New(guard)
	defer func() { _ = reg.Close() }()
	registry, err := newFontRegistry(context.Background(), guard, []*css.Stylesheet{}, Options{
		FallbackFonts: []string{path},
	}, reg)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := registry.Face([]string{"sans-serif"}, 400, false, 12000); !errors.Is(err, context.Canceled) {
		t.Fatalf("Face returned %v, want context.Canceled", err)
	}
}
