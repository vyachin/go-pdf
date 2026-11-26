package assets

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/vyachin/go-pdf/internal/budget"
)

// countingSource wraps a file system and counts the actual reader invocations,
// the bytes read and the handles closed per file, so a test can prove that one
// read admitted one copy of the bytes.
type countingSource struct {
	fs.FS
	mu     sync.Mutex
	opens  map[string]int
	closes map[string]int
	read   map[string]int
}

func newCountingSource(files fstest.MapFS) *countingSource {
	return &countingSource{FS: files, opens: map[string]int{}, closes: map[string]int{}, read: map[string]int{}}
}

func (c *countingSource) Open(name string) (fs.File, error) {
	f, err := c.FS.Open(name)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.opens[name]++
	c.mu.Unlock()
	return &countingSourceFile{File: f, name: name, source: c}, nil
}

// counters reports the opens, closes and bytes read for one source file.
func (c *countingSource) counters(name string) (opens, closes, bytesRead int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens[name], c.closes[name], c.read[name]
}

type countingSourceFile struct {
	fs.File
	name   string
	source *countingSource
}

func (f *countingSourceFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if n > 0 {
		f.source.mu.Lock()
		f.source.read[f.name] += n
		f.source.mu.Unlock()
	}
	return n, err
}

func (f *countingSourceFile) Close() error {
	f.source.mu.Lock()
	f.source.closes[f.name]++
	f.source.mu.Unlock()
	return f.File.Close()
}

// fixedInfo is the file information of the synthetic sources below.
type fixedInfo struct {
	name string
	size int64
}

func (i fixedInfo) Name() string       { return i.name }
func (i fixedInfo) Size() int64        { return i.size }
func (i fixedInfo) Mode() fs.FileMode  { return 0o644 }
func (i fixedInfo) ModTime() time.Time { return time.Time{} }
func (i fixedInfo) IsDir() bool        { return false }
func (i fixedInfo) Sys() any           { return nil }

// lyingSource serves one file whose Stat reports a declared size while Read
// returns different bytes. It models the defect where the total was charged
// from the size seen before the read.
type lyingSource struct {
	name string
	data []byte
	size int64
	mu   sync.Mutex
	opens,
	closes int
}

func (s *lyingSource) Open(name string) (fs.File, error) {
	if name != s.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	s.mu.Lock()
	s.opens++
	s.mu.Unlock()
	return &lyingFile{Reader: bytes.NewReader(s.data), source: s}, nil
}

func (s *lyingSource) counters() (opens, closes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens, s.closes
}

type lyingFile struct {
	*bytes.Reader
	source *lyingSource
}

func (f *lyingFile) Stat() (fs.FileInfo, error) {
	return fixedInfo{name: f.source.name, size: f.source.size}, nil
}

func (f *lyingFile) Close() error {
	f.source.mu.Lock()
	f.source.closes++
	f.source.mu.Unlock()
	return nil
}

// errFlakyRead is the transient failure the failing source reports once.
var errFlakyRead = errors.New("transient source read failure")

// flakySource fails the first read of one file and serves its bytes afterwards,
// so a test can observe a failed attempt and a later retry separately.
type flakySource struct {
	name string
	data []byte
	mu   sync.Mutex
	opens,
	closes,
	reads int
}

func (s *flakySource) Open(name string) (fs.File, error) {
	if name != s.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	s.mu.Lock()
	s.opens++
	s.mu.Unlock()
	return &flakyFile{source: s}, nil
}

func (s *flakySource) counters() (opens, closes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens, s.closes
}

type flakyFile struct {
	source *flakySource
	off    int
}

func (f *flakyFile) Read(p []byte) (int, error) {
	f.source.mu.Lock()
	first := f.source.reads == 0
	f.source.reads++
	f.source.mu.Unlock()
	if first {
		return 0, errFlakyRead
	}
	if f.off >= len(f.source.data) {
		return 0, io.EOF
	}
	n := copy(p, f.source.data[f.off:])
	f.off += n
	return n, nil
}

func (f *flakyFile) Stat() (fs.FileInfo, error) {
	return fixedInfo{name: f.source.name, size: int64(len(f.source.data))}, nil
}

func (f *flakyFile) Close() error {
	f.source.mu.Lock()
	f.source.closes++
	f.source.mu.Unlock()
	return nil
}

// installBarrier holds the leader of an in-flight read until every peer has
// joined it, so the overlap is deterministic and channels, not sleeps, control
// it. It returns the counters of actual read invocations and of joined peers.
func installBarrier(reg *Registry, readers int) (invocations, joined *atomic.Int32) {
	invocations, joined = &atomic.Int32{}, &atomic.Int32{}
	release := make(chan struct{})
	reg.hooks = &readHooks{
		join: func() {
			if joined.Add(1) == int32(readers-1) {
				close(release)
			}
		},
		beforeRead: func() {
			invocations.Add(1)
			<-release
		},
	}
	return invocations, joined
}

// concurrentReaders runs n callers of one request and returns their records and
// errors.
func concurrentReaders(t *testing.T, reg *Registry, req Request, n int) ([]*Record, []error) {
	t.Helper()
	records := make([]*Record, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			records[i], errs[i] = reg.Read(req)
		}(i)
	}
	wg.Wait()
	return records, errs
}

// TestConcurrentReadersShareOneAdmission guards RV5-37: eight goroutines of one
// conversion read the same 1 MiB file against a 1.5 MiB total. All receive a
// usable record, the file is read once, and the total records 1 MiB.
func TestConcurrentReadersShareOneAdmission(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 1<<20)
	writeFile(t, dir, "one.bin", data)
	reg := New(nil)
	defer func() { _ = reg.Close() }()

	const readers = 8
	invocations, joined := installBarrier(reg, readers)
	records, errs := concurrentReaders(t, reg, Request{
		Kind:          Image,
		Reference:     "one.bin",
		Dir:           dir,
		MaxTotalBytes: 3 << 19, // 1.5 MiB
	}, readers)
	for i := range records {
		if errs[i] != nil {
			t.Fatalf("reader %d failed: %v", i, errs[i])
		}
		if records[i] == nil || !bytes.Equal(records[i].Bytes, data) {
			t.Fatalf("reader %d did not receive the file", i)
		}
		if records[i].Key != records[0].Key {
			t.Fatalf("reader %d received a different identity: %q", i, records[i].Key)
		}
	}
	if got := invocations.Load(); got != 1 {
		t.Fatalf("the file was read %d times, want 1", got)
	}
	if got := joined.Load(); got != readers-1 {
		t.Fatalf("%d peers joined the in-flight read, want %d", got, readers-1)
	}
	if got := reg.Reads(Image); got != 1 {
		t.Fatalf("read %d times, want 1", got)
	}
	if got := reg.Total(Image); got != int64(len(data)) {
		t.Fatalf("the total is %d bytes, want %d", got, len(data))
	}
}

// TestConcurrentSourceReadersOpenOnce counts the actual reader invocations for
// a source-backed conversion: the eight peers share one open, one read and one
// admission.
func TestConcurrentSourceReadersOpenOnce(t *testing.T) {
	data := bytes.Repeat([]byte("y"), 1<<20)
	source := newCountingSource(fstest.MapFS{"one.bin": &fstest.MapFile{Data: data}})
	reg := NewWithSource(nil, source)
	defer func() { _ = reg.Close() }()

	const readers = 8
	invocations, joined := installBarrier(reg, readers)
	records, errs := concurrentReaders(t, reg, Request{
		Kind:          Image,
		Reference:     "one.bin",
		MaxTotalBytes: 3 << 19, // 1.5 MiB
	}, readers)
	for i := range records {
		if errs[i] != nil {
			t.Fatalf("reader %d failed: %v", i, errs[i])
		}
		if records[i] == nil || !bytes.Equal(records[i].Bytes, data) {
			t.Fatalf("reader %d did not receive the file", i)
		}
	}
	if got := invocations.Load(); got != 1 {
		t.Fatalf("the leader read %d times, want 1", got)
	}
	if got := joined.Load(); got != readers-1 {
		t.Fatalf("%d peers joined the in-flight read, want %d", got, readers-1)
	}
	if opens, closes, bytesRead := source.counters("one.bin"); opens != 1 || closes != 1 || bytesRead != len(data) {
		t.Fatalf("source opened %d times, closed %d times, read %d bytes; want 1, 1, %d",
			opens, closes, bytesRead, len(data))
	}
	if got := reg.Reads(Image); got != 1 {
		t.Fatalf("read %d times, want 1", got)
	}
	if got := reg.Total(Image); got != int64(len(data)) {
		t.Fatalf("the total is %d bytes, want %d", got, len(data))
	}
}

// TestConcurrentConversionsKeepTheirOwnCounters checks that the read and
// admission counters stay conversion-local when two conversions run at the same
// time.
func TestConcurrentConversionsKeepTheirOwnCounters(t *testing.T) {
	type conversion struct {
		reg  *Registry
		src  *countingSource
		data []byte
	}
	first := bytes.Repeat([]byte("a"), 1024)
	second := bytes.Repeat([]byte("b"), 2048)
	conversions := []*conversion{
		{src: newCountingSource(fstest.MapFS{"logo.bin": &fstest.MapFile{Data: first}}), data: first},
		{src: newCountingSource(fstest.MapFS{"logo.bin": &fstest.MapFile{Data: second}}), data: second},
	}
	for _, c := range conversions {
		c.reg = NewWithSource(nil, c.src)
		defer func(c *conversion) { _ = c.reg.Close() }(c)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for _, c := range conversions {
		c := c
		installBarrier(c.reg, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := c.reg.Read(Request{Kind: Image, Reference: "logo.bin"}); err != nil {
					errCh <- err
				}
			}()
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("a conversion failed: %v", err)
	}
	for i, c := range conversions {
		if got := c.reg.Reads(Image); got != 1 {
			t.Fatalf("conversion %d read %d times, want 1", i, got)
		}
		if got := c.reg.Total(Image); got != int64(len(c.data)) {
			t.Fatalf("conversion %d total = %d, want %d", i, got, len(c.data))
		}
		if opens, closes, bytesRead := c.src.counters("logo.bin"); opens != 1 || closes != 1 || bytesRead != len(c.data) {
			t.Fatalf("conversion %d source opened %d times, closed %d times, read %d bytes; want 1, 1, %d",
				i, opens, closes, bytesRead, len(c.data))
		}
	}
}

// TestAdmittedBytesAreReconciledWithTheRead guards RV5-38: the total follows
// the bytes actually admitted, not the size reported before the read.
func TestAdmittedBytesAreReconciledWithTheRead(t *testing.T) {
	t.Run("declared zero is charged in full", func(t *testing.T) {
		data := bytes.Repeat([]byte("z"), 1<<20)
		source := &lyingSource{name: "liar.bin", data: data, size: 0}
		reg := NewWithSource(nil, source)
		defer func() { _ = reg.Close() }()
		rec, err := reg.Read(Request{Kind: Image, Reference: "liar.bin", MaxTotalBytes: 3 << 19})
		if err != nil {
			t.Fatalf("a file that reported size 0 failed: %v", err)
		}
		if len(rec.Bytes) != len(data) {
			t.Fatalf("read %d bytes, want %d", len(rec.Bytes), len(data))
		}
		if rec.charged != int64(len(data)) {
			t.Fatalf("the record charged %d bytes, want %d", rec.charged, len(data))
		}
		if got := reg.Total(Image); got != int64(len(data)) {
			t.Fatalf("the total is %d bytes, want %d", got, len(data))
		}
		if opens, closes := source.counters(); opens != 1 || closes != 1 {
			t.Fatalf("source opened %d times, closed %d times; want 1, 1", opens, closes)
		}
	})

	t.Run("surplus over the total is refused", func(t *testing.T) {
		data := bytes.Repeat([]byte("z"), 1<<20)
		source := &lyingSource{name: "liar.bin", data: data, size: 0}
		reg := NewWithSource(nil, source)
		defer func() { _ = reg.Close() }()
		_, err := reg.Read(Request{Kind: Image, Reference: "liar.bin", MaxTotalBytes: 512 << 10})
		if !errors.Is(err, budget.ErrExceeded) {
			t.Fatalf("a surplus over the total returned %v, want budget.ErrExceeded", err)
		}
		if got := reg.Total(Image); got != 0 {
			t.Fatalf("the refused read left %d bytes in the total", got)
		}
		if got := reg.Reads(Image); got != 0 {
			t.Fatalf("the refused read was counted as %d successful reads", got)
		}
		if opens, closes := source.counters(); opens != 1 || closes != 1 {
			t.Fatalf("source opened %d times, closed %d times; want 1, 1", opens, closes)
		}
	})

	t.Run("shortfall is refunded at once", func(t *testing.T) {
		data := []byte("0123456789abcdef")
		source := &lyingSource{name: "liar.bin", data: data, size: 64}
		reg := NewWithSource(nil, source)
		defer func() { _ = reg.Close() }()
		rec, err := reg.Read(Request{Kind: Image, Reference: "liar.bin", MaxTotalBytes: 64})
		if err != nil {
			t.Fatalf("a file that shrank failed: %v", err)
		}
		if len(rec.Bytes) != len(data) {
			t.Fatalf("read %d bytes, want %d", len(rec.Bytes), len(data))
		}
		if got := reg.Total(Image); got != int64(len(data)) {
			t.Fatalf("the total is %d bytes, want %d", got, len(data))
		}
	})
}

// TestFailedReadKeepsNoRecordAndRetries guards RV5-37's failure half and the
// accounting rule: every peer receives the same typed cause, nothing is
// committed, and a later caller retries the failed stage and succeeds.
func TestFailedReadKeepsNoRecordAndRetries(t *testing.T) {
	data := []byte("recovered bytes")
	source := &flakySource{name: "flaky.bin", data: data}
	reg := NewWithSource(nil, source)
	defer func() { _ = reg.Close() }()

	const readers = 8
	installBarrier(reg, readers)
	records, errs := concurrentReaders(t, reg, Request{Kind: Image, Reference: "flaky.bin"}, readers)
	for i := range errs {
		if errs[i] == nil {
			t.Fatalf("reader %d accepted the failing read", i)
		}
		if records[i] != nil {
			t.Fatalf("reader %d received a record for the failing read", i)
		}
		if errs[i] != errs[0] {
			t.Fatalf("reader %d received a different cause: %v", i, errs[i])
		}
		var assetErr *Error
		if !errors.As(errs[i], &assetErr) || assetErr.Kind != Image || assetErr.Reference != "flaky.bin" {
			t.Fatalf("reader %d lost the typed cause: %v", i, errs[i])
		}
		if !errors.Is(errs[i], errFlakyRead) {
			t.Fatalf("reader %d lost the underlying cause: %v", i, errs[i])
		}
	}
	if got := reg.Reads(Image); got != 0 {
		t.Fatalf("the failed read was counted as %d successful reads", got)
	}
	if got := reg.Total(Image); got != 0 {
		t.Fatalf("the failed read retained %d bytes", got)
	}
	if opens, closes := source.counters(); opens != 1 || closes != 1 {
		t.Fatalf("the failing attempt opened %d times, closed %d times; want 1, 1", opens, closes)
	}

	// The in-flight entry was removed, so the next caller invokes the failed
	// stage again instead of inheriting the transient failure.
	rec, err := reg.Read(Request{Kind: Image, Reference: "flaky.bin"})
	if err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	if rec.Cached {
		t.Fatal("the retry was served from the cache")
	}
	if string(rec.Bytes) != string(data) {
		t.Fatalf("the retry returned %q", rec.Bytes)
	}
	if opens, closes := source.counters(); opens != 2 || closes != 2 {
		t.Fatalf("the retry opened %d times, closed %d times; want 2, 2", opens, closes)
	}
	if got := reg.Reads(Image); got != 1 {
		t.Fatalf("the retry recorded %d successful reads, want 1", got)
	}
	if got := reg.Total(Image); got != int64(len(data)) {
		t.Fatalf("the retry charged %d bytes, want %d", got, len(data))
	}

	// A later reuse of the successful record adds no read and no charge.
	again, err := reg.Read(Request{Kind: Image, Reference: "flaky.bin"})
	if err != nil {
		t.Fatalf("the cached read failed: %v", err)
	}
	if !again.Cached {
		t.Fatal("the later read was not served from the cache")
	}
	if opens, closes := source.counters(); opens != 2 || closes != 2 {
		t.Fatalf("the cached read opened %d times, closed %d times; want 2, 2", opens, closes)
	}
	if got := reg.Reads(Image); got != 1 {
		t.Fatalf("the cached read changed the successful read count to %d", got)
	}
	if got := reg.Total(Image); got != int64(len(data)) {
		t.Fatalf("the cached read changed the total to %d", got)
	}
}

// TestRefusedReadReleasesReservationAndHandle guards the cancellation path: a
// read the work budget refuses leaves no reservation and closes its handle.
func TestRefusedReadReleasesReservationAndHandle(t *testing.T) {
	data := bytes.Repeat([]byte("r"), 8192)
	source := newCountingSource(fstest.MapFS{"a.bin": &fstest.MapFile{Data: data}})
	// ChargeBytes(8192) is 3 units; a budget of 1 cannot pay for it.
	guard := budget.New(context.Background(), 1)
	reg := NewWithSource(guard, source)
	defer func() { _ = reg.Close() }()
	_, err := reg.Read(Request{Kind: Image, Reference: "a.bin", MaxTotalBytes: 1 << 20})
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("a budget that cannot pay for the read returned %v", err)
	}
	if got := reg.Total(Image); got != 0 {
		t.Fatalf("the refused read retained %d bytes", got)
	}
	if got := reg.Reads(Image); got != 0 {
		t.Fatalf("the refused read was counted as %d successful reads", got)
	}
	if opens, closes, _ := source.counters("a.bin"); opens != 1 || closes != 1 {
		t.Fatalf("source opened %d times, closed %d times; want 1, 1", opens, closes)
	}
}

// TestDiscardRefundsExactlyTheChargedAmount guards the discard half of RV5-38:
// the refund is the amount charged after reconciliation, and repeated discards
// cannot drive the total below zero.
func TestDiscardRefundsExactlyTheChargedAmount(t *testing.T) {
	data := bytes.Repeat([]byte("d"), 4096)
	source := &lyingSource{name: "liar.bin", data: data, size: 0}
	reg := NewWithSource(nil, source)
	defer func() { _ = reg.Close() }()
	rec, err := reg.Read(Request{Kind: Image, Reference: "liar.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Total(Image); got != int64(len(data)) {
		t.Fatalf("the total is %d bytes, want %d", got, len(data))
	}
	reg.Discard(rec)
	if got := reg.Total(Image); got != 0 {
		t.Fatalf("after discard the total is %d, want 0", got)
	}
	// A second discard of the same record is a no-op, never a negative total.
	reg.Discard(rec)
	if got := reg.Total(Image); got != 0 {
		t.Fatalf("the second discard left %d bytes", got)
	}
	// The record is gone, so a later read retries and charges again.
	rec, err = reg.Read(Request{Kind: Image, Reference: "liar.bin"})
	if err != nil {
		t.Fatalf("the read after discard failed: %v", err)
	}
	if rec.Cached {
		t.Fatal("the discarded record was still cached")
	}
	if opens, closes := source.counters(); opens != 2 || closes != 2 {
		t.Fatalf("source opened %d times, closed %d times; want 2, 2", opens, closes)
	}
	if got := reg.Total(Image); got != int64(len(data)) {
		t.Fatalf("the retry charged %d bytes, want %d", got, len(data))
	}
}
