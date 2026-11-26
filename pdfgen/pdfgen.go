package pdfgen

import (
	"context"
	"fmt"
	"io"
	"io/fs"
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
	// FontDir is the directory used to resolve relative @font-face paths. A
	// relative src resolves against FontDir, then BaseDir, then the directory
	// of File, then the process working directory. FontDir confines the fonts
	// the document may load: a path or symlink that leaves it is rejected. An
	// absolute src is accepted only when it lies inside the configured root,
	// and is rejected when no root is configured unless AllowAbsoluteAssets is
	// set. A configured directory that cannot be opened is reported as
	// options/asset-root before any font is read.
	//
	// When Source is set, FontDir is a directory inside that source and
	// defaults to its root (BaseDir is not consulted); an absolute src is
	// rejected instead of being read as written.
	FontDir string
	// BaseDir is the directory used to resolve relative image and stylesheet
	// paths. When it is set, reads are confined to that directory: a path or
	// symlink that leaves it is rejected with a diagnostic, and an absolute
	// reference is accepted only when it lies inside it.
	//
	// When BaseDir is empty, the root is the directory of File and then the
	// process working directory. Relative references stay confined to that
	// root, and an absolute reference is rejected unless AllowAbsoluteAssets is
	// set.
	//
	// When Source is set, BaseDir is a directory inside that source and
	// defaults to its root, and the same confinement applies to the source
	// namespace.
	BaseDir string
	// AllowAbsoluteAssets accepts an absolute image, stylesheet or @font-face
	// reference that no configured root covers, so the file is read as written.
	// It restores the pre-3.x behaviour for a caller that assembles a template
	// and its assets in a directory it did not configure, and it weakens the
	// default confinement: leave it false for untrusted templates.
	//
	// It has no effect when BaseDir or FontDir is set, because a configured
	// root already decides containment, and no effect on
	// FallbackFont/FallbackFonts, whose paths are caller-selected and already
	// read as written. A source path is relative by definition, so the option
	// does not apply to Options.Source either.
	AllowAbsoluteAssets bool
	// Source is a caller-supplied file system that replaces the host filesystem
	// for the conversion. When it is non-nil, every document-referenced image,
	// stylesheet and @font-face source, and FallbackFont/FallbackFonts, is read
	// through it; BaseDir and FontDir name directories inside it and default to
	// its root, and a fallback path is read from its root. No host file and no
	// installed host font is consulted. Source paths are
	// slash-separated and relative: an absolute or escaping reference is
	// rejected with the asset's load diagnostic, and AllowAbsoluteAssets does
	// not apply because a source has no operating-system paths. A nil Source
	// keeps the filesystem behaviour unchanged.
	Source fs.FS
	// Date replaces the PDF creation date. The zero value selects a fixed
	// constant so that repeated runs stay byte identical; set it for documents
	// that must carry the real date.
	Date time.Time
	// Title overrides the document title taken from <title>.
	Title string
	// RenderPNG asks the converter to also rasterize the first page into
	// Result.PNG. It is a debug preview, not production output: only the first
	// page is rendered, at 96 dpi, and the raster is bounded by
	// Limits.MaxRasterPixels. A failing rasterizer or PNG encoder leaves
	// Result.PNG nil and is reported as an error.
	//
	// Deprecated: RenderPNG is a debug-only preview with no supported
	// replacement in the 2.x series. It stays for the rest of 2.x and may be
	// removed only in the next major release, when a debug package or a preview
	// entry point can replace it.
	RenderPNG bool
	// File is the document name used in diagnostics and, when no asset root is
	// configured, the source of the default asset root: its directory is used
	// only for that, so a label such as "<stdin>" keeps the working directory
	// as the root. When empty a placeholder is used.
	File string
	// FallbackFont is a TrueType or OpenType/CFF font used for families that
	// are not declared with @font-face. It is tried before FallbackFonts and
	// wins regardless of the requested weight or slant. When both this field
	// and FallbackFonts are empty, a family the document does not declare
	// fails with font/unavailable instead of reading an installed host font,
	// so the same input renders identically on every host.
	//
	// When Source is set, FallbackFont is a path inside that source, read from
	// its root, and no host file is read.
	FallbackFont string
	// FallbackFonts is an ordered list of TrueType or OpenType/CFF files used
	// for families that are not declared with @font-face. The first file that
	// loads becomes the fallback family, with its conventional bold and italic
	// siblings discovered next to it; a missing or unparseable file is skipped
	// and the next one is tried. FallbackFont is tried first. The list is
	// caller-selected bytes, so it keeps the output deterministic; pass
	// [SystemFallbackFonts] to opt into the host-dependent platform search.
	//
	// When Source is set, every path is inside that source, read from its root.
	FallbackFonts []string
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

// SystemFallbackFonts returns the conventional system font paths: Arial and
// Times New Roman under the macOS supplemental directory, DejaVu Sans and
// Liberation Serif under the Linux font directories. The list covers both
// platforms, and paths that do not exist are skipped at conversion time.
//
// The result is a fresh copy, so a caller may modify or truncate it without
// affecting another call. The list is host-dependent and belongs to an
// explicit opt-in: passing it as Options.FallbackFonts restores the convenience
// of rendering with an installed font, at the cost of output that differs
// between machines. The zero Options value never consults it.
func SystemFallbackFonts() []string {
	return []string{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf",
	}
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
	// MaxTotalStylesheetBytes bounds the encoded bytes of every linked
	// stylesheet one conversion reads, in addition to MaxStylesheetBytes per
	// file. A stylesheet that would cross it is rejected before it is read.
	// Default 16 MiB.
	MaxTotalStylesheetBytes int
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
	// caller set none. A document or asset problem names its file, including a
	// linked stylesheet. Only an option error may leave it empty.
	File string
	// Line is the 1-based line of the problem, or zero when unknown. A
	// conversion-wide budget or a writer failure has no single source node, so
	// it keeps File and leaves Line and Column zero instead of inventing a
	// position.
	Line int
	// Column is the 1-based column of the problem, or zero when unknown. It
	// counts characters, not bytes.
	Column int
	// Element is a human-readable path such as "body > table > tr", set when an
	// element owns the problem.
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

// Err returns an *Error holding only the error-severity diagnostics, or nil
// when the list contains only warnings or is empty. The full list, including
// warnings, stays available through [Result.Diagnostics] and Validate.
func (d Diagnostics) Err() error {
	errorsOnly := make(Diagnostics, 0, len(d))
	for _, item := range d {
		if item.Severity != SeverityWarning {
			errorsOnly = append(errorsOnly, item)
		}
	}
	if len(errorsOnly) == 0 {
		return nil
	}
	return &Error{Diagnostics: errorsOnly}
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

// Result is the outcome of Convert or ConvertTo.
type Result struct {
	// PDF is the rendered document. It is set by Convert and always nil for
	// ConvertTo, which streams the PDF to its writer instead; it is valid only
	// when the returned error is nil.
	PDF []byte
	// PNG is the rasterized first page. It is set only when
	// Options.RenderPNG was requested and the preview was produced; a
	// rasterizer or encoder failure leaves it nil and is returned as an error.
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
	return publicResult(ctx, res, err, opts)
}

// ConvertTo renders a template and streams the PDF to dst.
//
// It runs the same pipeline as [Convert] and reports the same diagnostics, but
// writes the document to dst as it is serialised instead of returning it in
// [Result].PDF, which stays nil. Serialisation holds at most one page's
// content stream, so the memory it needs does not grow with the number of
// pages. The bytes written to dst equal the bytes Convert returns for the same
// input.
//
// A layout or validation error is reported before the first byte. A write error
// or a cancelled context can leave a prefix of the PDF in dst; the call returns
// an error and the caller must discard the destination. A write failure is
// reported as a pdf/write diagnostic, and a cancelled context is returned as
// the context error. dst must not block indefinitely: cancellation is checked
// between pages, not inside one Write.
func ConvertTo(ctx context.Context, dst io.Writer, src []byte, opts Options) (Result, error) {
	res, err := engine.ConvertTo(ctx, dst, src, opts.toEngine())
	return publicResult(ctx, res, err, opts)
}

// publicResult maps an engine result to the public one and applies the
// documented error order: a cancelled context wins over a template error, and
// error-severity diagnostics are returned as the typed error.
func publicResult(ctx context.Context, res engine.Result, err error, opts Options) (Result, error) {
	out := Result{PDF: res.PDF, PNG: res.PNG, Diagnostics: fromDiags(res.Diagnostics, diagnosticFile(opts))}
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
// reports the same problems as Convert, including a missing image, a character
// without a glyph and an invalid metadata value, so it belongs in a CI check.
//
// Serialisation is the only stage Validate does not run: a document rejected
// only by the writer's work budget is reported by Convert as resource/limit, and
// that is the one difference between the two entry points.
//
// Like Convert it returns an *Error when the template is invalid and the
// context error when ctx is done.
func Validate(ctx context.Context, src []byte, opts Options) (Diagnostics, error) {
	list, err := engine.Validate(ctx, src, opts.toEngine())
	out := fromDiags(list, diagnosticFile(opts))
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
// both report true, and the empty name is valid because an empty
// Options.Charset asks the engine to detect the encoding. A caller that wants to
// check only named encodings writes name != "" && CharsetSupported(name).
// The reference command uses it to reject a bad -charset as a usage error
// before reading the input.
func CharsetSupported(name string) bool {
	if name == "" {
		return true
	}
	_, err := charset.Validate(name)
	return err == nil
}

func (o Options) toEngine() engine.Options {
	return engine.Options{
		PageSize:            o.PageSize,
		Margin:              o.Margin,
		FontDir:             o.FontDir,
		BaseDir:             o.BaseDir,
		AllowAbsoluteAssets: o.AllowAbsoluteAssets,
		Source:              o.Source,
		Date:                o.Date,
		Title:               o.Title,
		RenderPNG:           o.RenderPNG,
		File:                o.File,
		FallbackFont:        o.FallbackFont,
		FallbackFonts:       o.FallbackFonts,
		Charset:             o.Charset,
		Strict:              o.Strict,
		Limits:              o.Limits.toEngine(),
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
		MaxInputBytes:           l.MaxInputBytes,
		MaxStylesheetBytes:      l.MaxStylesheetBytes,
		MaxTotalStylesheetBytes: l.MaxTotalStylesheetBytes,
		MaxPages:                l.MaxPages,
		MaxImagePixels:          l.MaxImagePixels,
		MaxImageBytes:           l.MaxImageBytes,
		MaxRasterPixels:         l.MaxRasterPixels,
		MaxFontBytes:            l.MaxFontBytes,
		MaxTotalImageBytes:      l.MaxTotalImageBytes,
		MaxTotalImagePixels:     l.MaxTotalImagePixels,
		MaxTotalFontBytes:       l.MaxTotalFontBytes,
		MaxDOMDepth:             l.MaxDOMDepth,
		MaxTableCells:           l.MaxTableCells,
		MaxOperations:           l.MaxOperations,
	}
}

func fromEngineLimits(l engine.Limits) Limits {
	return Limits{
		MaxInputBytes:           l.MaxInputBytes,
		MaxStylesheetBytes:      l.MaxStylesheetBytes,
		MaxTotalStylesheetBytes: l.MaxTotalStylesheetBytes,
		MaxPages:                l.MaxPages,
		MaxImagePixels:          l.MaxImagePixels,
		MaxImageBytes:           l.MaxImageBytes,
		MaxRasterPixels:         l.MaxRasterPixels,
		MaxFontBytes:            l.MaxFontBytes,
		MaxTotalImageBytes:      l.MaxTotalImageBytes,
		MaxTotalImagePixels:     l.MaxTotalImagePixels,
		MaxTotalFontBytes:       l.MaxTotalFontBytes,
		MaxDOMDepth:             l.MaxDOMDepth,
		MaxTableCells:           l.MaxTableCells,
		MaxOperations:           l.MaxOperations,
	}
}

// diagnosticFile is the name a document diagnostic carries when the engine did
// not set one. options/* diagnostics stay without a file.
func diagnosticFile(opts Options) string {
	if opts.File != "" {
		return opts.File
	}
	return "document.html"
}

func fromDiags(list diag.List, file string) Diagnostics {
	if len(list) == 0 {
		return nil
	}
	out := make(Diagnostics, len(list))
	for i, item := range list {
		severity := SeverityError
		if item.Severity == diag.Warning {
			severity = SeverityWarning
		}
		name := item.Pos.File
		if name == "" && !strings.HasPrefix(item.Code, "options/") {
			name = file
		}
		out[i] = Diagnostic{
			Severity: severity,
			Code:     item.Code,
			Message:  item.Message,
			File:     name,
			Line:     item.Pos.Line,
			Column:   item.Pos.Col,
			Element:  item.Element,
		}
	}
	return out
}
