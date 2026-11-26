# Proposal

## Why

Four defects in the PDF writer change what a reader sees and no existing test
catches them: palette and gray+alpha PNG samples are written premultiplied and
then multiplied again by the separate `/SMask` (a red pixel becomes `80 00 00`
instead of `ff 00 00`), every `/FontDescriptor` says `/Flags 4` even for an
italic face, the `ToUnicode` map skips a CID whose rune is U+0000, and the zlib
writer is not closed on the error path. The first two are visibly wrong output,
the third breaks the promise that every emitted glyph is copyable, and the
fourth can leak a writer or emit a truncated stream.

## What Changes

- Unpremultiply colour samples for palette and gray+alpha PNGs before writing
  them; the alpha stays in the separate `/SMask`, so a viewer's second multiply
  produces the source colour (RV5-33).
- Derive `/Flags` from the embedded face: bit 7 (64) for an italic or oblique
  face, consistent with the non-zero `/ItalicAngle` in the same dictionary
  (RV5-34).
- Write a `bfchar` entry for every emitted CID, including U+0000 instead of
  skipping it (RV5-35).
- Close the zlib writer on every path, handle its error, and fall back to an
  uncompressed stream when compression fails or does not shrink the data
  (RV5-36).

No public API, diagnostic code or deterministic-output rule changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: transparent image colour, font descriptor flags, text-layer
  coverage and stream compression gain explicit, testable requirements.

## Impact

- `internal/pdf/png.go` (palette and gray+alpha sampling) and
  `internal/pdf/writer.go` (descriptor flags, `ToUnicode`, `streamObject`).
- `internal/pdf` tests: byte-level assertions for the PNG samples, a flags
  assertion, a U+0000 mapping test and a failing-encoder seam for compression.
- Existing golden PDFs whose images use palette or gray+alpha PNGs are
  regenerated and reviewed; documents with NRGBA images are byte-identical.
