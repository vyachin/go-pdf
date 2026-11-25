package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/font"
)

// TestFontLimitBecomesResourceLimit guards the mapping the amplifier fixture
// depends on: a parse that crosses a documented bound must be reported as
// resource/limit, not as an unavailable font.
func TestFontLimitBecomesResourceLimit(t *testing.T) {
	err := loadFontError("hostile.ttf", fmt.Errorf("%w: GPOS amplifier", font.ErrLimit))
	if !errors.Is(err, budget.ErrExceeded) {
		t.Fatalf("font.ErrLimit mapped to %v, want budget.ErrExceeded", err)
	}
}
