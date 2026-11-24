# Proposal

## Why

An independent review found two defects in the PDF writer that affect documents that already ship:

- `/XObject` resources are serialised by iterating a map, so the same document with several images produces different bytes and a different document `/ID` on every run. This breaks the reproducibility guarantee the engine advertises.
- Glyph advances computed by layout are dropped when the run is written. PDF text is emitted with `Tj`, which uses nominal font widths, so kerning that layout applied is lost and the PDF no longer matches the engine's own raster or the intended spacing.

## What Changes

- Image resources SHALL be written in a deterministic order, derived from the order in which images first appear in the document.
- Glyph runs SHALL carry the advance calculated by layout and the writer SHALL express the difference from the nominal width as `TJ` adjustments, so that positions in the PDF match layout.
- Text extraction used by tests SHALL read `TJ` arrays as well as plain `Tj` strings.
- Regression tests: repeated serialisation of a document with several images is byte-identical; a run with nonzero kerning carries a nonzero adjustment and still extracts the same characters.

Out of scope: the remaining review findings, including per-page resource filtering (only image ordering is in scope, not removing unused images from every page).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: adds a deterministic resource ordering requirement and a glyph positioning requirement (kerning preserved in the PDF).

## Impact

- **Code**: `internal/pdf/writer.go` (resource loop, text run emission, glyph type), `internal/engine/convert.go` (copy advances into PDF ops), `internal/pdfdebug/extract.go` (read `TJ` arrays), tests in `internal/pdf`, `internal/engine`.
- **Output**: PDF bytes change for any text with kerning; document `/ID` changes once, then stays stable. Text extraction remains byte-for-byte compatible with the previous contract (same characters, same order).
- **Risk**: an incorrect adjustment sign would shift glyphs; covered by a test comparing positions with layout advances.
