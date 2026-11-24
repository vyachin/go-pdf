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
