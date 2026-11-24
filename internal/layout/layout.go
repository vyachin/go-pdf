package layout

import (
	"errors"

	"github.com/vyachin/go-pdf/internal/budget"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/html"
)

type layouter struct {
	ctx       Context
	diags     diag.List
	doc       *Document
	pageStyle PageStyle
	faces     map[string]*FontFace
	anchors   map[string]Anchor
	// ops counts every drawing operation emitted so far; pagination uses it to
	// detect that a page made no progress.
	ops     int
	targets map[string]int
	// maxIterations bounds the page loop; zero means the default.
	maxIterations int
	// limitReported records that the work budget was already reported, so a
	// stopped layout adds one diagnostic instead of one per loop.
	limitReported bool
}

// Run lays out a validated document and returns its pages.
func Run(doc *html.Document, ctx Context) (*Document, diag.List) {
	l := &layouter{
		ctx:       ctx,
		doc:       &Document{Anchors: map[string]Anchor{}},
		pageStyle: ctx.PageStyle,
		faces:     map[string]*FontFace{},
		anchors:   map[string]Anchor{},
		targets:   ctx.TargetPages,
	}
	root, buildDiags := Build(doc, ctx)
	l.diags = append(l.diags, buildDiags...)
	for _, d := range buildDiags {
		if d.Code == "resource/limit" {
			l.limitReported = true
		}
	}
	if root == nil {
		l.doc.Diags = l.diags
		return l.doc, l.diags
	}
	if l.stopReason() != nil {
		// A cancelled or exhausted build must not paginate a partial tree.
		l.doc.Diags = l.diags
		l.doc.Diags.Sort()
		return l.doc, l.doc.Diags
	}
	l.paginate(root)
	l.doc.Anchors = l.anchors
	return l.doc, l.doc.Diags
}

// stopReason reports why layout must stop, or nil while it may continue. It
// adds the resource/limit diagnostic once for an exhausted budget; a cancelled
// context is reported by the engine after Run returns.
func (l *layouter) stopReason() error {
	err := l.ctx.Budget.Err()
	if err == nil {
		return nil
	}
	if errors.Is(err, budget.ErrExceeded) && !l.limitReported {
		l.limitReported = true
		l.diags.Errorf("resource/limit", diag.Position{}, "", "%v", err)
	}
	return err
}

// tick records one unit of pagination work and reports why layout must stop.
func (l *layouter) tick() error {
	err := l.ctx.Budget.Add(1)
	if err == nil {
		return nil
	}
	l.stopReason()
	return err
}
