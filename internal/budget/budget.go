// Package budget bounds the total work of one conversion and carries context
// cancellation through stages whose signatures return no context error.
//
// A conversion records one work unit per token, box, placed line and drawing
// operation. The engine spends a single Guard across parsing, stylesheet
// collection, layout and PDF serialization, so a document cannot avoid the
// limit by moving its cost to a later stage.
package budget

import (
	"context"
	"errors"
	"fmt"
)

// ErrExceeded reports that a document needs more work than the budget allows.
// It is deliberately separate from context cancellation: callers turn it into
// a resource/limit diagnostic, while a cancelled context is returned as is.
var ErrExceeded = errors.New("budget: work limit exceeded")

// checkEvery is how many Add calls may pass between context checks. A cancelled
// context is checked under a mutex, so checking on every token or line would
// cost more than the work it guards.
const checkEvery = 512

// Guard records the work of one conversion and reports why it must stop.
// A nil *Guard accepts unlimited work and is never cancelled, so code that
// does not care about budgets can pass nil.
type Guard struct {
	ctx     context.Context
	max     uint64
	used    uint64
	ticks   uint64
	failure error
}

// New returns a guard for one conversion. A zero or negative max means no work
// limit; a nil ctx means no cancellation.
func New(ctx context.Context, max int) *Guard {
	g := &Guard{ctx: ctx}
	if max > 0 {
		g.max = uint64(max)
	}
	return g
}

// Err returns why the guard stopped, or nil while work may continue. The first
// failure is kept, so a cancelled conversion keeps reporting context.Canceled
// even if work continues to be recorded.
func (g *Guard) Err() error {
	if g == nil {
		return nil
	}
	if g.failure != nil {
		return g.failure
	}
	if g.ctx != nil {
		if err := g.ctx.Err(); err != nil {
			g.failure = err
			return err
		}
	}
	if g.max > 0 && g.used > g.max {
		g.failure = fmt.Errorf("%w: the document needs more than %d operations", ErrExceeded, g.max)
		return g.failure
	}
	return nil
}

// Add records n work units and reports why the conversion must stop. The work
// limit is checked on every call; the context is checked once every
// checkEvery calls. Callers that need a prompt cancellation check, such as a
// page loop, use Err instead.
func (g *Guard) Add(n uint64) error {
	if g == nil {
		return nil
	}
	if g.failure != nil {
		return g.failure
	}
	if n > 0 {
		g.used += n
	}
	if g.max > 0 && g.used > g.max {
		return g.Err()
	}
	g.ticks++
	if g.ticks%checkEvery == 0 {
		return g.Err()
	}
	return nil
}

// Used returns the number of recorded work units.
func (g *Guard) Used() uint64 {
	if g == nil {
		return 0
	}
	return g.used
}

// Max returns the configured work limit, or zero when unlimited.
func (g *Guard) Max() uint64 {
	if g == nil {
		return 0
	}
	return g.max
}
