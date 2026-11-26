package budget

import (
	"context"
	"errors"
	"testing"
)

func TestNilGuardAcceptsWork(t *testing.T) {
	var g *Guard
	if err := g.Add(1_000_000); err != nil {
		t.Fatalf("a nil guard must accept work: %v", err)
	}
	if err := g.Err(); err != nil {
		t.Fatalf("a nil guard must never fail: %v", err)
	}
	if g.Used() != 0 || g.Max() != 0 {
		t.Fatal("a nil guard reports no usage and no limit")
	}
}

func TestGuardReportsWorkLimit(t *testing.T) {
	g := New(context.Background(), 10)
	if err := g.Add(10); err != nil {
		t.Fatalf("work up to the limit must pass: %v", err)
	}
	if err := g.Add(1); !errors.Is(err, ErrExceeded) {
		t.Fatalf("Add past the limit returned %v, want ErrExceeded", err)
	}
	if err := g.Err(); !errors.Is(err, ErrExceeded) {
		t.Fatalf("Err returned %v, want ErrExceeded", err)
	}
	// The first failure is sticky.
	if got := g.Used(); got != 11 {
		t.Fatalf("Used = %d, want 11", got)
	}
}

func TestGuardUnlimitedWork(t *testing.T) {
	g := New(context.Background(), 0)
	if err := g.Add(1 << 40); err != nil {
		t.Fatalf("a zero limit must not bound work: %v", err)
	}
}

func TestGuardReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	g := New(ctx, 0)
	if err := g.Err(); err != nil {
		t.Fatalf("a fresh guard must pass: %v", err)
	}
	cancel()
	// Cancellation is observed at the next Err call, not only on entry.
	if err := g.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Err returned %v, want context.Canceled", err)
	}
	if err := g.Add(1); !errors.Is(err, context.Canceled) {
		t.Fatalf("Add returned %v, want context.Canceled", err)
	}
}

func TestGuardChecksCancellationPeriodically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	g := New(ctx, 0)
	cancel()
	// The bulk of the work uses Add, which throttles the context check; it must
	// still stop within one check window.
	for i := 0; i < checkEvery; i++ {
		err := g.Add(1)
		if err == nil {
			continue
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Add returned %v, want context.Canceled", err)
		}
		if i+1 > checkEvery {
			t.Fatalf("cancellation was noticed after %d calls", i+1)
		}
		return
	}
	t.Fatal("Add never noticed the cancellation")
}

// TestChargeBytes pins the documented byte charge: one unit per started 4 KiB
// block plus one for the allocation, and a nil guard is a no-op.
func TestChargeBytes(t *testing.T) {
	g := New(context.Background(), 0)
	cases := []struct {
		n    int
		want uint64
	}{
		{0, 1},
		{1, 2},
		{4095, 2},
		{4096, 2},
		{4097, 3},
		{8192, 3},
	}
	for _, tc := range cases {
		before := g.Used()
		if err := g.ChargeBytes(tc.n); err != nil {
			t.Fatalf("ChargeBytes(%d): %v", tc.n, err)
		}
		if got := g.Used() - before; got != tc.want {
			t.Fatalf("ChargeBytes(%d) cost %d units, want %d", tc.n, got, tc.want)
		}
	}
	var nilGuard *Guard
	if err := nilGuard.ChargeBytes(4096); err != nil {
		t.Fatalf("a nil guard must accept a byte charge: %v", err)
	}
}

// TestLinesCapAndChargedScan guards the bounded line index: it stores at most
// MaxLineStarts entries, charges every stored entry, marks itself capped, and
// still resolves a line past the cap by charging the scan.
func TestLinesCapAndChargedScan(t *testing.T) {
	lines := MaxLineStarts + 1
	var src []byte
	for i := 1; i <= lines; i++ {
		src = append(src, "0123456789"...)
		if i < lines {
			src = append(src, '\n')
		}
	}
	guard := New(context.Background(), 0)
	index, err := NewLines(src, guard)
	if err != nil {
		t.Fatalf("a %d-line source returned %v, want a capped index", lines, err)
	}
	if !index.Capped() || index.Len() != MaxLineStarts {
		t.Fatalf("index stores %d starts (capped %v), want %d", index.Len(), index.Capped(), MaxLineStarts)
	}
	if got, want := guard.Used(), uint64(MaxLineStarts); got != want {
		t.Fatalf("building the index charged %d units, want %d stored entries", got, want)
	}
	// A position on the line after the cap is resolved by the scan and the
	// scanned bytes are charged.
	offset := MaxLineStarts*11 + 5 // line MaxLineStarts+1 (0-based)
	scan := offset - (MaxLineStarts-1)*11
	before := guard.Used()
	line, start := index.Resolve(offset)
	if line != MaxLineStarts || start != MaxLineStarts*11 {
		t.Fatalf("Resolve(%d) = line %d start %d, want line %d start %d", offset, line, start, MaxLineStarts, MaxLineStarts*11)
	}
	if got := guard.Used() - before; got != uint64(scan) {
		t.Fatalf("the past-cap lookup charged %d units, want the %d scanned bytes", got, scan)
	}
}

// TestLinesChargeOneUnitPerEntry checks the stage-local spend: the index costs
// one unit per stored start, so a budget one unit short stops before the last
// entry is stored.
func TestLinesChargeOneUnitPerEntry(t *testing.T) {
	src := []byte("a\nb\nc\n")
	const starts = 4
	exact := New(context.Background(), starts)
	index, err := NewLines(src, exact)
	if err != nil {
		t.Fatalf("a budget of %d units failed: %v", starts, err)
	}
	if got := exact.Used(); got != starts {
		t.Fatalf("index charged %d units for %d starts", got, starts)
	}
	if line, _ := index.Resolve(len(src)); line != starts-1 {
		t.Fatalf("last offset resolved to line %d, want %d", line, starts-1)
	}
	short := New(context.Background(), starts-1)
	index, err = NewLines(src, short)
	if !errors.Is(err, ErrExceeded) {
		t.Fatalf("a budget of %d units returned %v, want ErrExceeded", starts-1, err)
	}
	if got := index.Len(); got != starts-1 {
		t.Fatalf("the refused build stored %d starts, want %d", got, starts-1)
	}
}
