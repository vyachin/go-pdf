package layout

import (
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
	targets   map[string]int
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
	if root == nil {
		l.doc.Diags = l.diags
		return l.doc, l.diags
	}
	l.paginate(root)
	l.doc.Anchors = l.anchors
	return l.doc, l.doc.Diags
}
