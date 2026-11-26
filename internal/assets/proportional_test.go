package assets

import (
	"context"
	"errors"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestReadChargesTheBytesItMaterialises guards the asset stage: a read charges
// one unit per started 4 KiB block plus one before it reads the file, so a
// budget one unit short refuses the read and no record is produced.
func TestReadChargesTheBytesItMaterialises(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sheet.css", make([]byte, 5000))
	// ceil(5000/4096)+1 = 3.
	const charge = 3

	guard := budget.New(context.Background(), 0)
	reg := New(guard)
	defer func() { _ = reg.Close() }()
	rec, err := reg.Read(Request{Kind: Stylesheet, Reference: "sheet.css", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Bytes) != 5000 {
		t.Fatalf("read %d bytes, want 5000", len(rec.Bytes))
	}
	if got := guard.Used(); got != charge {
		t.Fatalf("the read charged %d units, want %d", got, charge)
	}

	short := budget.New(context.Background(), charge-1)
	reg = New(short)
	defer func() { _ = reg.Close() }()
	rec, err = reg.Read(Request{Kind: Stylesheet, Reference: "sheet.css", Dir: dir})
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("a budget one unit short returned %v, want ErrExceeded", err)
	}
	if rec != nil {
		t.Fatal("the refused read returned a record")
	}
	if got := reg.Reads(Stylesheet); got != 0 {
		t.Fatalf("the refused read was counted: %d successful reads", got)
	}
	if got := reg.Total(Stylesheet); got != 0 {
		t.Fatalf("the refused read reserved %d bytes", got)
	}
}

// TestCachedReadIsChargedOnce guards that the byte charge follows the read, not
// the reference: a second reference to the same file is served from the cache.
func TestCachedReadIsChargedOnce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sheet.css", make([]byte, 5000))
	guard := budget.New(context.Background(), 0)
	reg := New(guard)
	defer func() { _ = reg.Close() }()
	req := Request{Kind: Stylesheet, Reference: "sheet.css", Dir: dir}
	if _, err := reg.Read(req); err != nil {
		t.Fatal(err)
	}
	charged := guard.Used()
	rec, err := reg.Read(req)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Cached {
		t.Fatal("the second read is not cached")
	}
	if got := guard.Used(); got != charged {
		t.Fatalf("the cached read charged %d more units", got-charged)
	}
}
