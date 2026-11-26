# Design

## Context

Layout already produces a `layout.Document` holding every page and its drawing
operations; `engine.Convert` converts those operations to `pdf.Op`, appends pages to a
`pdf.Doc`, and calls `Doc.Bytes`. `Bytes` collects fonts and images in first-use order,
assigns object ids, fills `bodies`, then `assemble` copies every body into a
`bytes.Buffer`, appends the xref and trailer and returns `buf.Bytes()`. The complete PDF
therefore exists at least twice at the peak, and `Result.PDF` keeps it until the caller
writes it somewhere.

## Goals / Non-Goals

**Goals:**

- An additive streaming entry point with the same diagnostics and byte-identical output.
- One serialiser shared by `Convert` and `ConvertTo`, so the two cannot drift.
- A write error and cancellation are reported, never swallowed.
- Peak serialisation memory independent of the number of pages.

**Non-Goals:**

- Streaming layout itself: pages and their operations stay in memory, bounded by
  `MaxPages` and the work budget. That is a larger refactor and is not needed to remove
  the second full-PDF copy.
- HTTP range requests, encryption, incremental update of an existing PDF.
- Changing `Convert`'s signature or `Result.PDF` for existing callers.

## Decisions

**A function, not a method on `Result`.** `Result.WriteTo` would run after `Convert`, when
the PDF already exists, so it cannot reduce memory. `ConvertTo(ctx, dst, src, opts)` mirrors
`Convert` and `Validate`, reports the same `Result` with `PDF` nil, and leaves room for
`PNG`. Alternative: an `Encoder` type; rejected for now because it adds a second stateful
surface without a second use case.

**Page ids reserved before writing; other objects appended as discovered.** The page count
is known after layout, so ids `1..N` are reserved for pages, which lets the page objects be
written while their content is produced. Fonts and images become known from the operations
of those pages; their objects are appended after the last page, followed by the outline,
catalog and info dictionary. The xref is written last, so an object's offset is recorded
by the counting writer when it is emitted, not recomputed. Alternative: emit in the old
id order; rejected because it requires every body to exist before the first byte is
written, which is the memory problem.

**`Bytes` is implemented over `WriteTo`.** `Doc.Bytes` becomes `WriteTo` into a
`bytes.Buffer` and returns its bytes. One code path means `Convert` and `ConvertTo` produce
the same bytes by construction, and a future ordering change cannot update only one side.
Alternative: keep two serialisers and compare them in a test; rejected because the test
would pass only until the next change.

**`engine` takes a sink, not a buffer.** The shared conversion path ends in
`serialise(w io.Writer)`; `Convert` passes a `bytes.Buffer`, `ConvertTo` passes the
caller's writer. The writer error is wrapped like any serialisation failure, so the engine
reports `pdf/write`, while a context error still takes precedence. A short write is a
write error (`io.Writer` contract), handled by one `writeAll` helper.

**Document the partial-output contract.** A layout or validation error is detected before
the first byte. A write error or cancellation can leave a prefix in `dst`; the function
returns an error, and the caller discards the destination. This is stated in the godoc and
the README example, because a caller that ignores the error could otherwise publish a
truncated PDF.

## Risks / Trade-offs

- **Object numbering changes once.** The streamed order differs from today's id order, so
  PDF bytes for a given document change while remaining deterministic; render and text
  goldens are unchanged, and the structure is re-validated with pdfcpu. `ConvertTo` and
  `Convert` agree after the change.
- **A slow writer blocks the conversion.** Cancellation is checked between pages, not
  inside a single `Write`; a writer that blocks forever is the caller's contract violation.
  The godoc states that `dst` must not block indefinitely.
- **`Result.PDF` nil is a new contract.** A caller that passes a `Result` from `ConvertTo`
  to code expecting `PDF` gets nil; the godoc and the `Result` type comment call it out,
  and `Convert` is unchanged.
- **`Bytes` now allocates through a buffer with a growing slice.** The existing path adds
  one copy back for `Convert`; that is the same cost as today and only for the buffered
  entry point.

## Migration Plan

Additive: no existing function changes signature or behaviour. Add `ConvertTo` to the
package documentation and the README, list it under `CHANGELOG.md` `Unreleased / Added`,
and state the `Result.PDF == nil` contract. `Validate` is unchanged. No stored data or
diagnostic changes.
