package pdfgen

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vyachin/go-pdf/internal/charset"
	"github.com/vyachin/go-pdf/internal/diag"
	"github.com/vyachin/go-pdf/internal/engine"
)

// Options controls a single conversion. The zero value is valid: A4, a 20mm
// margin, a fixed creation date and the default limits.
type Options struct {
	// PageSize is the fallback page size used when the document does not set
	// @page { size }. Values are "A4", "A5", "Letter" or "<w>x<h><unit>", for
	// example "210x297mm". Empty means "A4".
	PageSize string
	// Margin is the fallback page margin, for example "20mm" or "15mm 20mm".
	// Empty means "20mm".
	Margin string
	// FontDir is the directory used to resolve relative @font-face paths.
	FontDir string
	// BaseDir is the directory used to resolve relative image paths. When it
	// is set, reads are confined to that directory: a path or symlink that
	// leaves it is rejected with a diagnostic.
	BaseDir string
	// Date replaces the PDF creation date. The zero value selects a fixed
	// constant so that repeated runs stay byte identical; set it for documents
	// that must carry the real date.
	Date time.Time
	// Title overrides the document title taken from <title>.
	Title string
	// RenderPNG asks the converter to also rasterize the first page into
	// Result.PNG. It is meant for previews and tests.
	RenderPNG bool
	// File is the path used in diagnostics. When empty a placeholder is used.
	File string
	// FallbackFont is a TrueType font used for families that are not declared
	// with @font-face. When empty a small list of system fonts is tried; a
	// document whose font cannot be found fails with a diagnostic instead of
	// silently replacing glyphs.
	FallbackFont string
	// Charset names the input encoding explicitly and wins over the document's
	// own declaration and its byte order mark. Empty means detect: a byte
	// order mark, then a <meta charset> or http-equiv declaration in the first
	// 1024 bytes, then UTF-8. The supported encodings are "utf-8",
	// "windows-1251", "koi8-r" and "ibm866" and their aliases; any other
	// value, including "utf-16" and "windows-1252", fails with an
	// options/charset diagnostic before any asset is read.
	Charset string
	// Strict turns profile warnings into errors, so a document that would
	// render with warnings is rejected instead. Callers use it to gate a
	// template in continuous integration.
	Strict bool
	// Limits bounds the work a single conversion may do. Zero fields mean the
	// defaults.
	Limits Limits
}

// Limits bounds the resources a conversion may consume. A limit is checked
// before the expensive step, so an over-limit document fails with a diagnostic
// instead of exhausting memory.
type Limits struct {
	// MaxInputBytes is the largest accepted HTML document. Default 8 MiB.
	// A single-byte input encoding decodes to at most three times its size, so
	// a document at the limit holds 24 MiB of decoded text before parsing.
	MaxInputBytes int
	// MaxStylesheetBytes bounds one external stylesheet. Default 8 MiB.
	MaxStylesheetBytes int
	// MaxPages is the largest number of pages a document may produce.
	// Default 5000.
	MaxPages int
	// MaxImagePixels bounds width*height of a decoded image, checked before
	// the pixels are decoded. Default 40,000,000.
	MaxImagePixels int
	// MaxImageBytes bounds the encoded size of one image file, checked while
	// reading. Default 32 MiB.
	MaxImageBytes int
	// MaxRasterPixels bounds width*height of a debug page raster, checked
	// before the buffer is allocated. Default 40,000,000.
	MaxRasterPixels int
	// MaxFontBytes bounds the size of a font file. Default 16 MiB.
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
	// parsing. Default 256.
	MaxDOMDepth int
	// MaxTableCells bounds the grid area of one table: its column count times
	// its row count. Default 200,000.
	MaxTableCells int
	// MaxOperations bounds the total work of a conversion: parsed tokens,
	// stylesheet rules and compounds matched by the cascade, font parsing,
	// image decoding, layout boxes, placed lines, copied drawing operations,
	// diagnostic paths and PDF writing. It is the backstop against a document
	// that is cheap to store but expensive to process. Default 20,000,000.
	MaxOperations int
}

// Severity classifies a diagnostic.
type Severity string

const (
	// SeverityError means the document cannot be rendered.
	SeverityError Severity = "error"
	// SeverityWarning means the document can be rendered but the result may
	// be surprising, for example content that overflows its page.
	SeverityWarning Severity = "warning"
)

// Diagnostic is a single problem report with a 1-based source position. A zero
// Line or Column means the position is unknown.
type Diagnostic struct {
	// Severity is SeverityError or SeverityWarning. An empty value means
	// SeverityError.
	Severity Severity
	// Code is a stable machine-readable identifier, for example
	// "css/unsupported-value". Matching on Code is safer than matching on
	// Message, which is meant for humans.
	Code string
	// Message explains the problem for a human. It is not stable: match on
	// Code instead.
	Message string
	// File is the name passed in Options.File, or a placeholder when the
	// caller set none. Empty means the file is unknown.
	File string
	// Line is the 1-based line of the problem, or zero when unknown.
	Line int
	// Column is the 1-based column of the problem, or zero when unknown.
	Column int
	// Element is a human-readable path such as "body > table > tr".
	Element string
}

// String renders the diagnostic in the form the reference command prints:
//
//	file:line:col: error[code]: message (at body > p)
func (d Diagnostic) String() string {
	var b strings.Builder
	if d.File != "" {
		b.WriteString(d.File)
		if d.Line > 0 {
			fmt.Fprintf(&b, ":%d", d.Line)
			if d.Column > 0 {
				fmt.Fprintf(&b, ":%d", d.Column)
			}
		}
		b.WriteString(": ")
	}
	severity := d.Severity
	if severity == "" {
		severity = SeverityError
	}
	b.WriteString(string(severity))
	if d.Code != "" {
		b.WriteString("[")
		b.WriteString(d.Code)
		b.WriteString("]")
	}
	b.WriteString(": ")
	b.WriteString(d.Message)
	if d.Element != "" {
		b.WriteString(" (at ")
		b.WriteString(d.Element)
		b.WriteString(")")
	}
	return b.String()
}

// Diagnostics is the ordered list of problems found in a template. The order is
// deterministic: by file, line, column, code and message.
type Diagnostics []Diagnostic

// HasErrors reports whether at least one diagnostic has SeverityError.
func (d Diagnostics) HasErrors() bool {
	for _, item := range d {
		if item.Severity != SeverityWarning {
			return true
		}
	}
	return false
}

// Lines renders every diagnostic on its own line, in the current order.
func (d Diagnostics) Lines() []string {
	out := make([]string, len(d))
	for i, item := range d {
		out[i] = item.String()
	}
	return out
}

// Err returns an *Error holding the error-severity diagnostics, or nil when the
// list contains only warnings or is empty.
func (d Diagnostics) Err() error {
	if !d.HasErrors() {
		return nil
	}
	return &Error{Diagnostics: d}
}

// Error is the error returned by Convert and Validate when the template cannot
// be rendered. Use errors.As to reach the diagnostics:
//
//	var verr *pdfgen.Error
//	if errors.As(err, &verr) {
//		for _, d := range verr.Diagnostics {
//			log.Print(d)
//		}
//	}
type Error struct {
	// Diagnostics holds the error-severity diagnostics that made the document
	// unusable, in the order Convert or Validate reported them.
	Diagnostics Diagnostics
}

// Error joins the error-severity diagnostics, one per line.
func (e *Error) Error() string {
	lines := make([]string, 0, len(e.Diagnostics))
	for _, d := range e.Diagnostics {
		if d.Severity != SeverityWarning {
			lines = append(lines, d.String())
		}
	}
	return strings.Join(lines, "\n")
}

// Result is the outcome of Convert.
type Result struct {
	// PDF is the rendered document. It is valid only when the returned error
	// is nil.
	PDF []byte
	// PNG is the rasterized first page. It is set only when
	// Options.RenderPNG was requested and the rasterizer succeeded.
	PNG []byte
	// Diagnostics contains the warnings produced while rendering. Errors are
	// returned as the error value instead, and are also present here.
	Diagnostics Diagnostics
}

// Convert renders a template into a PDF.
//
// It returns an *Error when the template is invalid or fails during rendering,
// and the context error when ctx is cancelled or its deadline passes. Warnings
// are reported through the result. Nothing is written to disk.
func Convert(ctx context.Context, src []byte, opts Options) (Result, error) {
	res, err := engine.Convert(ctx, src, opts.toEngine())
	out := Result{PDF: res.PDF, PNG: res.PNG, Diagnostics: fromDiags(res.Diagnostics)}
	if err != nil {
		// Cancellation wins over a template error: a template that already
		// produced validation errors must not hide the fact that the caller
		// asked the conversion to stop. Without this order a cancelled request
		// looked like an invalid template.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, ctxErr
		}
		if out.Diagnostics.HasErrors() {
			return out, out.Diagnostics.Err()
		}
		return out, err
	}
	return out, nil
}

// Validate parses, lays out and checks a template without writing a PDF. It
// reports the same problems as Convert, including a missing image and a
// character without a glyph, so it belongs in a CI check.
//
// Like Convert it returns an *Error when the template is invalid and the
// context error when ctx is done.
func Validate(ctx context.Context, src []byte, opts Options) (Diagnostics, error) {
	list, err := engine.Validate(ctx, src, opts.toEngine())
	out := fromDiags(list)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, ctxErr
		}
		return out, err
	}
	// The engine reports validation problems through the list and leaves the
	// error nil; the public API turns them into an error so that a caller
	// cannot forget to check HasErrors.
	if out.HasErrors() {
		return out, out.Err()
	}
	return out, nil
}

// CharsetSupported reports whether name is a supported input encoding. It
// matches the labels Options.Charset accepts, so "cp1251" and "windows-1251"
// both report true. The reference command uses it to reject a bad -charset as a
// usage error before reading the input.
func CharsetSupported(name string) bool {
	_, err := charset.Validate(name)
	return err == nil
}

func (o Options) toEngine() engine.Options {
	return engine.Options{
		PageSize:     o.PageSize,
		Margin:       o.Margin,
		FontDir:      o.FontDir,
		BaseDir:      o.BaseDir,
		Date:         o.Date,
		Title:        o.Title,
		RenderPNG:    o.RenderPNG,
		File:         o.File,
		FallbackFont: o.FallbackFont,
		Charset:      o.Charset,
		Strict:       o.Strict,
		Limits:       o.Limits.toEngine(),
	}
}

// DefaultLimits returns the limits a zero Limits resolves to. A caller that
// reads its own input can reject an oversized file before loading it with
// DefaultLimits().MaxInputBytes, and the values always match the conversion
// defaults.
func DefaultLimits() Limits {
	return fromEngineLimits(engine.DefaultLimits())
}

func (l Limits) toEngine() engine.Limits {
	return engine.Limits{
		MaxInputBytes:       l.MaxInputBytes,
		MaxStylesheetBytes:  l.MaxStylesheetBytes,
		MaxPages:            l.MaxPages,
		MaxImagePixels:      l.MaxImagePixels,
		MaxImageBytes:       l.MaxImageBytes,
		MaxRasterPixels:     l.MaxRasterPixels,
		MaxFontBytes:        l.MaxFontBytes,
		MaxTotalImageBytes:  l.MaxTotalImageBytes,
		MaxTotalImagePixels: l.MaxTotalImagePixels,
		MaxTotalFontBytes:   l.MaxTotalFontBytes,
		MaxDOMDepth:         l.MaxDOMDepth,
		MaxTableCells:       l.MaxTableCells,
		MaxOperations:       l.MaxOperations,
	}
}

func fromEngineLimits(l engine.Limits) Limits {
	return Limits{
		MaxInputBytes:       l.MaxInputBytes,
		MaxStylesheetBytes:  l.MaxStylesheetBytes,
		MaxPages:            l.MaxPages,
		MaxImagePixels:      l.MaxImagePixels,
		MaxImageBytes:       l.MaxImageBytes,
		MaxRasterPixels:     l.MaxRasterPixels,
		MaxFontBytes:        l.MaxFontBytes,
		MaxTotalImageBytes:  l.MaxTotalImageBytes,
		MaxTotalImagePixels: l.MaxTotalImagePixels,
		MaxTotalFontBytes:   l.MaxTotalFontBytes,
		MaxDOMDepth:         l.MaxDOMDepth,
		MaxTableCells:       l.MaxTableCells,
		MaxOperations:       l.MaxOperations,
	}
}

func fromDiags(list diag.List) Diagnostics {
	if len(list) == 0 {
		return nil
	}
	out := make(Diagnostics, len(list))
	for i, item := range list {
		severity := SeverityError
		if item.Severity == diag.Warning {
			severity = SeverityWarning
		}
		out[i] = Diagnostic{
			Severity: severity,
			Code:     item.Code,
			Message:  item.Message,
			File:     item.Pos.File,
			Line:     item.Pos.Line,
			Column:   item.Pos.Col,
			Element:  item.Element,
		}
	}
	return out
}
