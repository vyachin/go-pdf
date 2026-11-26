# Proposal

## Why

`Convert` returns the whole PDF in `Result.PDF`, and `pdf.Doc.Bytes` builds every object
body and then copies them into one more buffer. A server that converts a long report
therefore holds the page operations, the object bodies and the complete PDF at the same
time, and the caller needs another copy to hand it to a socket or a file. The public API
offers no way to write the document as it is produced, so the memory ceiling is set by the
largest output rather than by the largest page. `MaxPages` admits 5000 pages, so this is
the dominant allocation for the documents the package targets.

## What Changes

- Add `pdfgen.ConvertTo(ctx, dst io.Writer, src []byte, opts Options) (Result, error)`. It
  runs the same pipeline and reports the same diagnostics as `Convert`, writes the PDF to
  `dst` exactly once, and returns `Result.PDF == nil`.
- Add `pdf.Doc.WriteTo(w io.Writer) (int64, error)` and implement `Bytes` on top of it, so
  both paths share one serialiser and must agree byte for byte.
- Write objects in a deterministic streaming order: page content and page objects as pages
  are produced, then fonts and images discovered while writing those pages, then the
  outline, catalog and info dictionary, then the xref and trailer, with offsets recorded
  from a counting writer.
- Surface a write failure as the ordinary writer error: the engine turns it into
  `pdf/write` (or returns the context error when the writer failed because the conversion
  was cancelled), never a panic and never a silently truncated document.
- Document that `ConvertTo` may leave a prefix in `dst` when the writer fails or the
  context is cancelled, that the caller discards it, and that `RenderPNG` still returns the
  first-page raster while `Result.PDF` stays nil.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the conversion surface gains a streaming entry point with the same
  diagnostics, byte-identical output and a bounded serialisation buffer.

## Impact

- `internal/pdf/writer.go` (new `WriteTo`, shared object emission, xref while writing),
  `internal/engine/convert.go` (shared serialisation sink), `pdfgen/pdfgen.go`
  (new function and `Result` contract).
- Public API: additive `ConvertTo`; `Convert` output and signature unchanged.
- `README.md` (a streaming example), `pdfgen` package documentation, `CHANGELOG.md`.
- Tests: byte equality with `Convert`, writer-error propagation, cancellation between
  pages, a memory benchmark over many pages, and the existing PDF validator and render
  gates.
