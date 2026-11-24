// Package engine is the pipeline behind the pdfgen package and the CLI.
//
// It is internal: callers outside the module import github.com/vyachin/go-pdf/pdfgen,
// which validates the options and re-exports the diagnostics. Inside the module,
// callers validate a template and then convert it. Both operations share the
// same pipeline and the same diagnostics, so a template that validates cleanly
// is exactly a template that converts without validation errors.
package engine

import (
	"context"
	"errors"
	"time"

	"github.com/vyachin/go-pdf/internal/diag"
)

// Options controls a single conversion.
type Options struct {
	// PageSize is the fallback page size used when the document does not set
	// @page { size }. Values are "A4", "A5", "Letter" or "<w>x<h><unit>".
	PageSize string
	// Margin is the fallback page margin, for example "20mm".
	Margin string
	// FontDir is the directory used to resolve relative @font-face paths.
	FontDir string
	// BaseDir is the directory used to resolve relative image paths.
	BaseDir string
	// Date replaces the PDF creation date. The zero value selects a fixed
	// constant so that repeated runs stay byte identical.
	Date time.Time
	// Title overrides the document title taken from <title>.
	Title string
	// RenderPNG asks the converter to also rasterize the first page. It is
	// used by tests and the CLI debug flag.
	RenderPNG bool
	// File is the path used in diagnostics. When empty a placeholder is used.
	File string
	// FallbackFont is a TrueType font used for families that are not declared
	// with @font-face. When empty, a small list of system fonts is tried.
	FallbackFont string
	// Strict turns profile warnings into errors.
	Strict bool
	// Limits bounds the work a single conversion may do. Zero means the
	// defaults documented on Limits.
	Limits Limits
}

// Limits bounds the resources a conversion may consume.
type Limits struct {
	// MaxInputBytes is the largest accepted HTML document.
	MaxInputBytes int
	// MaxStylesheetBytes bounds one external stylesheet.
	MaxStylesheetBytes int
	// MaxPages is the largest number of pages a document may produce.
	MaxPages int
	// MaxImagePixels bounds width*height of a decoded image, checked before
	// the pixels are decoded.
	MaxImagePixels int
	// MaxFontBytes bounds the size of a font file.
	MaxFontBytes int
	// MaxDOMDepth bounds element nesting. A deeper document is rejected while
	// parsing, before the tree allocates one node per level.
	MaxDOMDepth int
	// MaxTableCells bounds the number of cells in one table.
	MaxTableCells int
	// MaxOperations bounds the total work of a conversion: parsed tokens,
	// layout boxes, placed lines and drawing operations. It is the backstop
	// against a document that is cheap to store but expensive to lay out.
	MaxOperations int
}

// resolved fills zero fields with their defaults.
func (l Limits) resolved() Limits {
	if l.MaxInputBytes <= 0 {
		l.MaxInputBytes = 8 << 20
	}
	if l.MaxStylesheetBytes <= 0 {
		l.MaxStylesheetBytes = 8 << 20
	}
	if l.MaxPages <= 0 {
		l.MaxPages = 5000
	}
	if l.MaxImagePixels <= 0 {
		l.MaxImagePixels = 40_000_000
	}
	if l.MaxFontBytes <= 0 {
		l.MaxFontBytes = 16 << 20
	}
	if l.MaxDOMDepth <= 0 {
		l.MaxDOMDepth = 256
	}
	if l.MaxTableCells <= 0 {
		l.MaxTableCells = 200_000
	}
	if l.MaxOperations <= 0 {
		l.MaxOperations = 20_000_000
	}
	return l
}

// Result is the outcome of a successful conversion.
type Result struct {
	// PDF is the rendered document.
	PDF []byte
	// PNG is the rasterized first page, when Options.RenderPNG was set.
	PNG []byte
	// Diagnostics contains warnings produced while rendering. Errors are
	// returned as the error value instead.
	Diagnostics diag.List
}

// ErrNotImplemented is returned by pipeline stages that are not wired up yet.
var ErrNotImplemented = errors.New("engine: not implemented")

// Validate parses and checks a template without rendering it.
func Validate(ctx context.Context, src []byte, opts Options) (diag.List, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, diags, err := prepare(ctx, src, opts)
	if err != nil {
		return diags, err
	}
	if err := ctx.Err(); err != nil {
		return diags, err
	}
	return diags, nil
}

func sourceName(opts Options) string {
	if opts.File != "" {
		return opts.File
	}
	return "document.html"
}
