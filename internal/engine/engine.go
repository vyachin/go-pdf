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
	"io/fs"
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
	// Source, when non-nil, is a caller-supplied file system that replaces the
	// host filesystem for the conversion: every image, stylesheet and font is
	// read through it. BaseDir and FontDir are then directories inside it and
	// default to its root, and FallbackFont is a path inside it. A nil Source
	// keeps the host filesystem behaviour unchanged.
	Source fs.FS
	// Date replaces the PDF creation date. The zero value selects a fixed
	// constant so that repeated runs stay byte identical.
	Date time.Time
	// Title overrides the document title taken from <title>.
	Title string
	// RenderPNG asks the converter to also rasterize the first page into a
	// debug preview.
	RenderPNG bool
	// File is the path used in diagnostics. When empty a placeholder is used.
	File string
	// FallbackFont is a TrueType font used for families that are not declared
	// with @font-face. When empty, a small list of system fonts is tried.
	FallbackFont string
	// Strict turns profile warnings into errors.
	Strict bool
	// Charset names the input encoding explicitly and wins over the document's
	// declaration and its byte order mark. Empty means detect. A value outside
	// the supported set fails with options/charset before any asset is read.
	Charset string
	// Limits bounds the work a single conversion may do. Zero means the
	// defaults documented on Limits.
	Limits Limits
}

// Limits bounds the resources a conversion may consume.
type Limits struct {
	// MaxInputBytes is the largest accepted HTML document. A legacy
	// single-byte encoding decodes to at most three times its size, so a
	// document at the limit holds 24 MiB of decoded text before parsing.
	MaxInputBytes int
	// MaxStylesheetBytes bounds one external stylesheet.
	MaxStylesheetBytes int
	// MaxTotalStylesheetBytes bounds the encoded bytes of every linked
	// stylesheet one conversion reads, in addition to MaxStylesheetBytes per
	// file. A stylesheet that would cross it is rejected before it is read.
	// Default 16 MiB.
	MaxTotalStylesheetBytes int
	// MaxPages is the largest number of pages a document may produce.
	MaxPages int
	// MaxImagePixels bounds width*height of a decoded image, checked before
	// the pixels are decoded.
	MaxImagePixels int
	// MaxImageBytes bounds the encoded size of one image file, checked while
	// reading so an oversized file is never loaded into memory.
	MaxImageBytes int
	// MaxRasterPixels bounds width*height of a debug page raster, checked
	// before the buffer is allocated.
	MaxRasterPixels int
	// MaxFontBytes bounds the size of a font file.
	MaxFontBytes int
	// MaxTotalImageBytes bounds the encoded bytes of every image one
	// conversion reads, in addition to MaxImageBytes per file. An image that
	// would cross it is rejected before it is opened. Default 64 MiB.
	MaxTotalImageBytes int
	// MaxTotalImagePixels bounds the decoded pixels of every image one
	// conversion decodes, in addition to MaxImagePixels per file. Default
	// 80,000,000.
	MaxTotalImagePixels int
	// MaxTotalFontBytes bounds the bytes of every font one conversion parses,
	// in addition to MaxFontBytes per file. Default 32 MiB.
	MaxTotalFontBytes int
	// MaxDOMDepth bounds element nesting. A deeper document is rejected while
	// parsing, before the tree allocates one node per level.
	MaxDOMDepth int
	// MaxTableCells bounds the grid area of one table: its column count times
	// its row count.
	MaxTableCells int
	// MaxOperations bounds the total work of a conversion: parsed tokens,
	// stylesheet rules and compounds matched by the cascade, font parsing,
	// image decoding, layout boxes, placed lines, copied drawing operations,
	// diagnostic paths and PDF writing. It is the backstop against a document
	// that is cheap to store but expensive to process.
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
	if l.MaxTotalStylesheetBytes <= 0 {
		l.MaxTotalStylesheetBytes = 16 << 20
	}
	if l.MaxPages <= 0 {
		l.MaxPages = 5000
	}
	if l.MaxImagePixels <= 0 {
		l.MaxImagePixels = 40_000_000
	}
	if l.MaxImageBytes <= 0 {
		l.MaxImageBytes = 32 << 20
	}
	if l.MaxRasterPixels <= 0 {
		l.MaxRasterPixels = 40_000_000
	}
	if l.MaxFontBytes <= 0 {
		l.MaxFontBytes = 16 << 20
	}
	if l.MaxTotalImageBytes <= 0 {
		l.MaxTotalImageBytes = 64 << 20
	}
	if l.MaxTotalImagePixels <= 0 {
		l.MaxTotalImagePixels = 80_000_000
	}
	if l.MaxTotalFontBytes <= 0 {
		l.MaxTotalFontBytes = 32 << 20
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

// DefaultLimits returns the limits a conversion uses when Options.Limits has
// zero fields. Callers that read an input themselves can use it to reject an
// oversized file before loading it.
func DefaultLimits() Limits { return Limits{}.resolved() }

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

// Validate parses and checks a template without writing a PDF. It runs the same
// preparation and layout as Convert, including the metadata checks, so it
// reports the same problems: a missing image, an unsupported colour space, a
// character without a glyph or an invalid title fails here exactly as it fails
// in Convert.
//
// PDF serialisation is the only stage Validate does not run. That stage decides
// very little — whether the document fits the writer's work budget and whether
// the output could be assembled — so a document that fails in Convert only for a
// write-side reason is reported as a resource/limit from serialisation. Any
// check decided before serialisation is shared, so a gate built on Validate
// cannot pass a document Convert rejects.
func Validate(ctx context.Context, src []byte, opts Options) (diag.List, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, diags, err := prepare(ctx, src, opts)
	if err != nil {
		return diags, err
	}
	// Preparation opened the conversion's roots; layout and every early return
	// release them through this defer.
	if p != nil {
		defer func() { _ = p.assets.Close() }()
	}
	if err := ctx.Err(); err != nil {
		return diags, err
	}
	if diags.HasErrors() {
		return diags, nil
	}
	if _, err := layoutDocument(ctx, p, opts, &diags); err != nil {
		return diags, err
	}
	if err := ctx.Err(); err != nil {
		return diags, err
	}
	diags.Sort()
	return diags, nil
}

func sourceName(opts Options) string {
	if opts.File != "" {
		return opts.File
	}
	return "document.html"
}
