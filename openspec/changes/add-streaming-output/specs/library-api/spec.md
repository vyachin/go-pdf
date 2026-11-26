# Spec Delta

## ADDED Requirements

### Requirement: A conversion can stream its PDF to an io.Writer (RV5-49)

`pdfgen` SHALL export `ConvertTo(ctx, dst io.Writer, src []byte, opts Options) (Result, error)`. It SHALL run the same pipeline as `Convert`, report the same diagnostics, write the PDF to `dst` once and return `Result.PDF == nil`. Serialisation MUST NOT materialise the complete PDF and MUST hold at most one page's content stream. A write error MUST be returned, output MUST equal `Convert`'s bytes, and cancellation MUST stop the write between pages.

#### Scenario: Byte equality with Convert

- **WHEN** the same template and options are converted with `Convert` and with
  `ConvertTo` into a buffer
- **THEN** the buffer equals `Result.PDF` from `Convert` byte for byte
- **AND** both results carry the same diagnostics

#### Scenario: The writer fails

- **WHEN** `dst` returns an error after a few kilobytes
- **THEN** `ConvertTo` stops writing and returns that error, which the engine reports as
  `pdf/write`
- **AND** it does not panic and does not return a `Result` with a PDF

#### Scenario: A document with many pages stays bounded

- **WHEN** a document produces thousands of pages and is streamed to a discard writer
- **THEN** the serialiser does not hold the assembled PDF in memory
- **AND** its heap stays within one page's content plus the offsets, independent of the
  total output size

#### Scenario: Cancellation between pages

- **WHEN** the context is cancelled while the writer is emitting pages
- **THEN** `ConvertTo` returns the context error
- **AND** it stops writing at the next page boundary

#### Scenario: A preview does not change the stream

- **WHEN** `RenderPNG` is set and the document is streamed
- **THEN** `Result.PNG` carries the first-page raster
- **AND** the bytes written to `dst` are identical to the same conversion without the
  option
