package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
	"github.com/vyachin/go-pdf/internal/font"
)

// countingParser wraps the real font parser for one registry and counts actual
// parses, so one parse is proven independently of the shared *font.Font
// pointer.
func countingParser(parses *atomic.Int32) func([]byte, font.ParseOptions) (*font.Font, error) {
	return func(data []byte, opts font.ParseOptions) (*font.Font, error) {
		parses.Add(1)
		return font.LoadWithOptions(data, opts)
	}
}

// requestFace asks the registry for one family at one size, exercising the
// layout adapter path without running layout.
func requestFace(t *testing.T, registry *fontRegistry, family string, size int32) {
	t.Helper()
	if _, err := registry.Face([]string{family}, 400, false, size); err != nil {
		t.Fatalf("Face(%q, %d): %v", family, size, err)
	}
}

// TestDeclaredAndFallbackShareOneFont guards the sharing contract: one file
// named by two @font-face families, one canonical alias and the explicit
// fallback is read, parsed and charged once, while both families still resolve
// at every requested size. The independent counters prove one read, one actual
// parse and one byte admission: a shared pointer alone is not evidence of one
// parse.
func TestDeclaredAndFallbackShareOneFont(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "shared.ttf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sheet := css.Parse([]byte(`@font-face { font-family: Shared; src: url(shared.ttf) }
@font-face { font-family: Alias; src: url(./shared.ttf) }`))
	guard := budget.New(context.Background(), 0)
	reg := assets.New(guard)
	defer func() { _ = reg.Close() }()
	var parses atomic.Int32
	registry, err := newFontRegistryWithLoader(context.Background(), guard, []*css.Stylesheet{sheet},
		Options{BaseDir: dir, FallbackFont: path}, reg, countingParser(&parses))
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Reads(assets.Font); got != 1 {
		t.Fatalf("the shared font was read %d times, want 1", got)
	}
	if got := parses.Load(); got != 1 {
		t.Fatalf("the shared font was parsed %d times, want 1", got)
	}
	if got := reg.Total(assets.Font); got != int64(len(data)) {
		t.Fatalf("the shared font counted %d bytes, want %d", got, len(data))
	}
	// Repeated size requests resolve both families from the one record and add
	// neither a read nor a parse.
	for _, size := range []int32{12000, 12000, 24000, 12000} {
		requestFace(t, registry, "Shared", size)
		requestFace(t, registry, "Alias", size)
	}
	if got := reg.Reads(assets.Font); got != 1 {
		t.Fatalf("the size requests read the font %d times, want 1", got)
	}
	if got := parses.Load(); got != 1 {
		t.Fatalf("the size requests parsed the font %d times, want 1", got)
	}
	if got := reg.Total(assets.Font); got != int64(len(data)) {
		t.Fatalf("the size requests charged %d bytes, want %d", got, len(data))
	}
}

// TestFontSpellingsAndFallbackUseOneRecord covers the three spellings of one
// file in one conversion — two @font-face sources and the explicit fallback —
// plus a second canonical font: each canonical file is read, parsed and charged
// once.
func TestFontSpellingsAndFallbackUseOneRecord(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "a.ttf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.ttf"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	sheet := css.Parse([]byte(`@font-face { font-family: A; src: url(a.ttf) }
@font-face { font-family: B; src: url(./a.ttf) }
@font-face { font-family: C; src: url(b.ttf) }`))
	guard := budget.New(context.Background(), 0)
	reg := assets.New(guard)
	defer func() { _ = reg.Close() }()
	var parses atomic.Int32
	registry, err := newFontRegistryWithLoader(context.Background(), guard, []*css.Stylesheet{sheet},
		Options{BaseDir: dir, FallbackFont: path}, reg, countingParser(&parses))
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Reads(assets.Font); got != 2 {
		t.Fatalf("two canonical files under three families were read %d times, want 2", got)
	}
	if got := parses.Load(); got != 2 {
		t.Fatalf("two canonical files were parsed %d times, want 2", got)
	}
	if got := reg.Total(assets.Font); got != int64(2*len(data)) {
		t.Fatalf("the fonts counted %d bytes, want %d", got, 2*len(data))
	}
	for _, family := range []string{"A", "B", "C"} {
		requestFace(t, registry, family, 12000)
		requestFace(t, registry, family, 12000)
		requestFace(t, registry, family, 24000)
	}
	if got := reg.Reads(assets.Font); got != 2 {
		t.Fatalf("the size requests read the fonts %d times, want 2", got)
	}
	if got := parses.Load(); got != 2 {
		t.Fatalf("the size requests parsed the fonts %d times, want 2", got)
	}
}

// TestFailedFontParseRefundsAndRetries guards the failure half of the sharing
// contract: a parse that fails leaves no successful read and no retained
// charge, and a later conversion invokes the parser again instead of inheriting
// the failure. A successful retry is then reused without another parse.
func TestFailedFontParseRefundsAndRetries(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.ttf"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	sheet := css.Parse([]byte(`@font-face { font-family: Broken; src: url(broken.ttf) }`))
	var parses atomic.Int32
	loader := func(data []byte, opts font.ParseOptions) (*font.Font, error) {
		if parses.Add(1) == 1 {
			return nil, errors.New("corrupt font")
		}
		return font.LoadWithOptions(data, opts)
	}
	guard := budget.New(context.Background(), 0)
	reg := assets.New(guard)
	if _, err := newFontRegistryWithLoader(context.Background(), guard, []*css.Stylesheet{sheet},
		Options{BaseDir: dir}, reg, loader); err == nil {
		t.Fatal("the failing parse was accepted")
	}
	if got := reg.Reads(assets.Font); got != 0 {
		t.Fatalf("the failed parse kept %d successful reads", got)
	}
	if got := reg.Total(assets.Font); got != 0 {
		t.Fatalf("the failed parse retained %d bytes", got)
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}

	// The retry is a new conversion with its own registry; it invokes the
	// failed stage again and succeeds.
	guard = budget.New(context.Background(), 0)
	reg = assets.New(guard)
	defer func() { _ = reg.Close() }()
	registry, err := newFontRegistryWithLoader(context.Background(), guard, []*css.Stylesheet{sheet},
		Options{BaseDir: dir}, reg, loader)
	if err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	if got := parses.Load(); got != 2 {
		t.Fatalf("the retry parsed %d times, want 2 attempts in total", got)
	}
	if got := reg.Reads(assets.Font); got != 1 {
		t.Fatalf("the retry recorded %d successful reads, want 1", got)
	}
	if got := reg.Total(assets.Font); got != int64(len(data)) {
		t.Fatalf("the retry charged %d bytes, want %d", got, len(data))
	}
	requestFace(t, registry, "Broken", 12000)
	if got := parses.Load(); got != 2 {
		t.Fatalf("a face request parsed the font again: %d parses", got)
	}
}

// TestImageSymlinkEscapeIsRejected guards confinement through a symlink: a link
// inside the base directory that points outside is refused.
func TestImageSymlinkEscapeIsRejected(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.png")
	writeTestPNG(t, target)
	dir := t.TempDir()
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	res, err := Convert(context.Background(), []byte(`<img src="link.png">`), Options{
		BaseDir:      dir,
		File:         "symlink.html",
		FallbackFont: testFontPath(t),
	})
	if err == nil {
		t.Fatal("an escaping symlink was read")
	}
	msg := err.Error() + "\n" + strings.Join(res.Diagnostics.Lines(), "\n")
	if !strings.Contains(msg, "outside the base directory") {
		t.Fatalf("the diagnostic does not name the escape: %s", msg)
	}
	if len(res.PDF) != 0 {
		t.Fatal("a PDF was produced for an escaping image")
	}
}

// TestConcurrentConversionsIsolateAssets guards conversion-local state: two
// conversions that name the same relative file under different roots keep their
// own bytes, failures and totals.
func TestConcurrentConversionsIsolateAssets(t *testing.T) {
	good := t.TempDir()
	writeTestPNG(t, filepath.Join(good, "logo.png"))
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "logo.png"), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	font := testFontPath(t)
	var wg sync.WaitGroup
	var goodErr, badErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, goodErr = Convert(context.Background(), []byte(`<img src="logo.png">`), Options{BaseDir: good, File: "g.html", FallbackFont: font})
	}()
	go func() {
		defer wg.Done()
		_, badErr = Convert(context.Background(), []byte(`<img src="logo.png">`), Options{BaseDir: bad, File: "b.html", FallbackFont: font})
	}()
	wg.Wait()
	if goodErr != nil {
		t.Fatalf("the valid conversion failed: %v", goodErr)
	}
	if badErr == nil {
		t.Fatal("the corrupt image was accepted because another conversion had a valid one")
	}
}

// cancelAfter is a context that cancels after a number of Err calls, so a test
// can stop a conversion after preparation without timing it.
type cancelAfter struct {
	context.Context
	mu    sync.Mutex
	calls int
	after int
}

func (c *cancelAfter) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls > c.after {
		return context.Canceled
	}
	return nil
}

// openFDs counts the file descriptors of the test process. It skips on a
// platform without /dev/fd.
func openFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skipf("no /dev/fd: %v", err)
	}
	return len(entries)
}

// TestCancelledConversionsReleaseTheirRoots guards cleanup: every root a
// cancelled conversion opened is closed again, so repeated cancellations do not
// leak handles.
func TestCancelledConversionsReleaseTheirRoots(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	font := testFontPath(t)
	src := []byte(`<img src="a.png"><p>text</p>`)
	before := openFDs(t)
	for i := 0; i < 24; i++ {
		ctx := &cancelAfter{Context: context.Background(), after: 6}
		_, _ = Convert(ctx, src, Options{BaseDir: dir, File: "cancel.html", FallbackFont: font})
	}
	after := openFDs(t)
	if after > before+4 {
		t.Fatalf("cancelled conversions leaked handles: %d open before, %d after", before, after)
	}
}
