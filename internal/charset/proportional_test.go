package charset

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
)

// TestDecodeChargesMaterialisedBytes guards the decoded-text stage: a legacy
// decode charges the buffers it is about to allocate before they exist, so a
// budget one unit short refuses the decode and a budget that covers it finishes.
func TestDecodeChargesMaterialisedBytes(t *testing.T) {
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	src := cp1251(t, strings.Repeat("Счёт ", 100))
	if len(src) != 500 {
		t.Fatalf("the fixture is %d bytes, want 500", len(src))
	}
	// 4 bytes of allocation per source byte: ceil(2000/4096)+1 = 2.
	const charge = 2
	exact := budget.New(context.Background(), charge)
	dec, err := DecodeWithGuard(enc, src, exact)
	if err != nil {
		t.Fatalf("a budget of %d units refused the decode: %v", charge, err)
	}
	if got := exact.Used(); got != charge {
		t.Fatalf("the decode charged %d units, want %d", got, charge)
	}
	if string(dec.Text) != strings.Repeat("Счёт ", 100) {
		t.Fatalf("the decoded text is wrong: %q", dec.Text)
	}

	short := budget.New(context.Background(), charge-1)
	dec, err = DecodeWithGuard(enc, src, short)
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("a budget one unit short returned %v, want ErrExceeded", err)
	}
	if dec != nil {
		t.Fatal("the refused decode materialised a result")
	}

	// A UTF-8 source is used as is, so it needs no byte charge.
	utf8Guard := budget.New(context.Background(), 0)
	if _, err := DecodeWithGuard(nil, []byte("<p>x</p>"), utf8Guard); err != nil {
		t.Fatal(err)
	}
	if got := utf8Guard.Used(); got != 0 {
		t.Fatalf("a UTF-8 decode charged %d units, want 0", got)
	}
}

// TestMapperResolvesPastTheLineCap guards the mapper's bounded indexes: a
// legacy document with more lines than MaxLineStarts still reports the correct
// line and column past the cap.
func TestMapperResolvesPastTheLineCap(t *testing.T) {
	const lines = budget.MaxLineStarts + 5_000
	var text strings.Builder
	for i := 0; i < lines; i++ {
		text.WriteString("abc\n")
	}
	src := cp1251(t, text.String())
	enc, err := Validate(Windows1251)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc, src)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMapperWithGuard(src, dec.Text, dec.Widths, dec.Skip, nil)
	offset := (lines - 1) * 4
	line, col := m.Position(offset)
	if line != lines || col != 1 {
		t.Fatalf("Position(%d) = %d:%d, want %d:1", offset, line, col, lines)
	}
	if line, col = m.Position(offset + 2); line != lines || col != 3 {
		t.Fatalf("Position(%d) = %d:%d, want %d:3", offset+2, line, col, lines)
	}
}
