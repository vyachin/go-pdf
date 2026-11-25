package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/css"
)

// TestDeclaredAndFallbackShareOneFont guards the sharing contract: one file
// named by @font-face and by the explicit fallback is read, parsed and counted
// once, while both roles still resolve.
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
	sheet := css.Parse([]byte(`@font-face { font-family: Shared; src: url(shared.ttf) }`))
	guard := budget.New(context.Background(), 0)
	reg := assets.New(guard)
	defer func() { _ = reg.Close() }()
	if _, err := newFontRegistry(context.Background(), guard, []*css.Stylesheet{sheet},
		Options{BaseDir: dir, FallbackFont: path}, reg); err != nil {
		t.Fatal(err)
	}
	if got := reg.Reads(assets.Font); got != 1 {
		t.Fatalf("the shared font was read %d times, want 1", got)
	}
	if got := reg.Total(assets.Font); got != int64(len(data)) {
		t.Fatalf("the shared font counted %d bytes, want %d", got, len(data))
	}
}

// TestFontSpellingsAndFallbackUseOneRecord covers the three spellings of one
// file in one conversion: two @font-face sources and the explicit fallback.
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
	sheet := css.Parse([]byte(`@font-face { font-family: A; src: url(a.ttf) }
@font-face { font-family: B; src: url(./a.ttf) }`))
	guard := budget.New(context.Background(), 0)
	reg := assets.New(guard)
	defer func() { _ = reg.Close() }()
	if _, err := newFontRegistry(context.Background(), guard, []*css.Stylesheet{sheet},
		Options{BaseDir: dir, FallbackFont: path}, reg); err != nil {
		t.Fatal(err)
	}
	if got := reg.Reads(assets.Font); got != 1 {
		t.Fatalf("one file under three roles was read %d times, want 1", got)
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
