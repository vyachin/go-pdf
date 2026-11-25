# Proposal

## Why

Several PDF-level defects are confirmed but user-visible only in the file: the debug
raster ignores the image clip that the PDF applies, so `Result.PNG` shows a background
spilling outside its box; `pdfdebug` treats any inflated stream containing `TJ` as page
content, so a JPEG or font adds phantom pages and shifts test page indices, and its
content-stream order is nondeterministic; the subset is embedded but not named as a
subset (`pdffonts` reports `sub no`); `/CapHeight` is always the ascent and
`/ItalicAngle` is always 0; an invalid UTF-8 `Options.Title` is silently replaced with
U+FFFD; more than 65,535 (glyph, rune) pairs wrap the `uint16` CID; a nil `Text.Font`
panics; and outline levels below 1 are compared unnormalised.

## What Changes

- The debug raster SHALL honour `ImageOp.Clip` exactly as the PDF does.
- `pdfdebug` SHALL derive pages from the page tree and return streams in sorted order.
- An embedded subset SHALL carry the six-letter subset tag in `/BaseFont` and
  `/FontName`; `/CapHeight` and `/ItalicAngle` SHALL come from the font.
- Metadata strings SHALL be valid UTF-8 or rejected, never substituted.
- More than 65,535 distinct CID pairs SHALL fail with a diagnostic instead of
  wrapping; a nil font SHALL return an error.
- Outline levels SHALL be normalised before they are compared.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: fonts, metadata, CIDs, outlines and the debug raster match what the
  file and the preview promise.
- `golden-tests`: page and text extraction use the page tree, not stream sniffing.

## Impact

- **Code**: `internal/pdf/writer.go`, `outline.go`, `internal/backend/raster`,
  `internal/pdfdebug/extract.go`.
- **Output**: subset names and descriptor values change (goldens regenerate); a
  hostile or invalid title, CID set or outline fails with a diagnostic.
- **Tests**: pdfcpu strict, `pdffonts` subset flag, non-BMP extraction and the
  clip-raster comparison become assertions.
