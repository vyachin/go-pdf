package engine

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/vyachin/go-pdf/internal/assets"
	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/html"
	"github.com/vyachin/go-pdf/internal/stylesheet"
)

// countingFS wraps a file system and counts, per file, the bytes the
// conversion actually read from it. A zero count proves that a refused read
// never started.
type countingFS struct {
	fs.FS
	mu    sync.Mutex
	read  map[string]int
	opens map[string]int
}

func newCountingFS(files fstest.MapFS) *countingFS {
	return &countingFS{FS: files, read: map[string]int{}, opens: map[string]int{}}
}

func (c *countingFS) Open(name string) (fs.File, error) {
	f, err := c.FS.Open(name)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.opens[name]++
	c.mu.Unlock()
	return &countingFile{File: f, name: name, fs: c}, nil
}

func (c *countingFS) readBytes(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.read[name]
}

func (c *countingFS) readFiles() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var names []string
	for name, n := range c.read {
		if n > 0 {
			names = append(names, name)
		}
	}
	return names
}

type countingFile struct {
	fs.File
	name string
	fs   *countingFS
}

func (f *countingFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if n > 0 {
		f.fs.mu.Lock()
		f.fs.read[f.name] += n
		f.fs.mu.Unlock()
	}
	return n, err
}

// addTestFont makes the bundled font readable through the source.
func addTestFont(t *testing.T, files fstest.MapFS) {
	t.Helper()
	data, err := os.ReadFile(testFontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	files["DejaVuSans.ttf"] = &fstest.MapFile{Data: data}
}

// stylesheetData returns a stylesheet of exactly size bytes holding one comment
// with n unique text. A comment is valid CSS and keeps parsing cheap.
func stylesheetData(n int, size int) []byte {
	body := []byte(fmt.Sprintf("/* sheet %d ", n))
	if len(body)+2 > size {
		panic("size too small")
	}
	body = append(body, bytes.Repeat([]byte{'x'}, size-len(body)-2)...)
	return append(body, '*', '/')
}

// stylesheetLinks returns a document body that links every name.
func stylesheetLinks(names []string) []byte {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head>")
	for _, name := range names {
		fmt.Fprintf(&b, `<link rel="stylesheet" href="%s">`, name)
	}
	b.WriteString("</head><body><p>text</p></body></html>")
	return []byte(b.String())
}

// TestTotalStylesheetLimitIsResourceLimit guards RV5-13: 60 unique 1 MiB linked
// stylesheets under the default totals fail with resource/limit before the
// seventeenth file is read.
func TestTotalStylesheetLimitIsResourceLimit(t *testing.T) {
	const files = 60
	mapFS := fstest.MapFS{}
	names := make([]string, files)
	data := stylesheetData(0, 1<<20)
	for i := 0; i < files; i++ {
		names[i] = fmt.Sprintf("s%d.css", i)
		mapFS[names[i]] = &fstest.MapFile{Data: data}
	}
	cfs := newCountingFS(mapFS)
	res, err := Convert(context.Background(), stylesheetLinks(names), Options{
		Source:       cfs,
		File:         "sheets.html",
		FallbackFont: testFontPath(t),
	})
	if err == nil || !hasCode(res.Diagnostics, "resource/limit") || len(res.PDF) != 0 {
		t.Fatalf("default totals did not stop the stylesheets: err=%v diags=%v", err, res.Diagnostics.Lines())
	}
	if reads := cfs.readFiles(); len(reads) != 16 {
		t.Fatalf("%d stylesheets were read, want 16 before the total is crossed", len(reads))
	}
	if got := len(cfs.readFiles()); got > 16 {
		t.Fatalf("the seventeenth file was read: %d files", got)
	}
}

// TestSingleStylesheetAtThePerFileLimitConverts guards the upper bound of one
// file: an 8 MiB sheet under the default totals still converts, and its PDF is
// identical to one converted with the total named explicitly.
func TestSingleStylesheetAtThePerFileLimitConverts(t *testing.T) {
	data := stylesheetData(0, 8<<20)
	mapFS := fstest.MapFS{"big.css": &fstest.MapFile{Data: data}}
	addTestFont(t, mapFS)
	src := stylesheetLinks([]string{"big.css"})
	def, err := Convert(context.Background(), src, Options{Source: newCountingFS(mapFS), File: "big.html", FallbackFont: "DejaVuSans.ttf"})
	if err != nil {
		t.Fatalf("an 8 MiB sheet under the default totals failed: %v\n%s", err, strings.Join(def.Diagnostics.Lines(), "\n"))
	}
	explicit, err := Convert(context.Background(), src, Options{
		Source:       newCountingFS(mapFS),
		File:         "big.html",
		FallbackFont: "DejaVuSans.ttf",
		Limits:       Limits{MaxTotalStylesheetBytes: 16 << 20},
	})
	if err != nil {
		t.Fatalf("naming the default total changed the result: %v", err)
	}
	if !bytes.Equal(def.PDF, explicit.PDF) {
		t.Fatal("the PDF depends on whether the default total was named explicitly")
	}
}

// TestCallerRaisesTotalStylesheetLimit guards the escape hatch: a caller that
// raises the total gets every linked stylesheet read and a successful
// conversion.
func TestCallerRaisesTotalStylesheetLimit(t *testing.T) {
	const files = 20
	mapFS := fstest.MapFS{}
	names := make([]string, files)
	data := stylesheetData(0, 1<<20)
	for i := 0; i < files; i++ {
		names[i] = fmt.Sprintf("s%d.css", i)
		mapFS[names[i]] = &fstest.MapFile{Data: data}
	}
	addTestFont(t, mapFS)
	cfs := newCountingFS(mapFS)
	res, err := Convert(context.Background(), stylesheetLinks(names), Options{
		Source:       cfs,
		File:         "sheets.html",
		FallbackFont: "DejaVuSans.ttf",
		Limits:       Limits{MaxTotalStylesheetBytes: 24 << 20},
	})
	if err != nil {
		t.Fatalf("the raised total still failed: %v\n%s", err, strings.Join(res.Diagnostics.Lines(), "\n"))
	}
	if len(res.PDF) == 0 {
		t.Fatal("no PDF produced")
	}
	read := 0
	for _, name := range cfs.readFiles() {
		if strings.HasSuffix(name, ".css") {
			read++
		}
	}
	if read != files {
		t.Fatalf("%d of %d stylesheets were read", read, files)
	}
}

// TestOneStylesheetLinkedTwiceCountsOnce guards the identity rule: the bytes of
// a repeated link count once against the total, and both inclusions keep their
// place in the cascade.
func TestOneStylesheetLinkedTwiceCountsOnce(t *testing.T) {
	data := stylesheetData(0, 4096)
	cfs := newCountingFS(fstest.MapFS{"a.css": &fstest.MapFile{Data: data}})
	src := stylesheetLinks([]string{"a.css", "a.css"})
	doc, err := parseTestDocument(t, src)
	if err != nil {
		t.Fatal(err)
	}
	guard := budget.New(context.Background(), 0)
	reg := assets.NewWithSource(guard, cfs)
	defer func() { _ = reg.Close() }()
	sheets, diags := stylesheet.Collect(doc, stylesheet.Options{
		Budget:                  guard,
		Assets:                  reg,
		MaxTotalStylesheetBytes: 16 << 20,
	})
	if diags.HasErrors() {
		t.Fatalf("collecting the sheets failed: %v", diags.Lines())
	}
	if len(sheets) != 2 {
		t.Fatalf("the cascade has %d sheets, want 2", len(sheets))
	}
	if got := reg.Total(assets.Stylesheet); got != int64(len(data)) {
		t.Fatalf("the total counts %d bytes, want %d", got, len(data))
	}
	if got := cfs.readBytes("a.css"); got != len(data) {
		t.Fatalf("the file was read %d bytes, want %d", got, len(data))
	}
}

// parseTestDocument parses a document without a work limit.
func parseTestDocument(t *testing.T, src []byte) (*html.Document, error) {
	t.Helper()
	return html.Parse(src, "sheets.html")
}
