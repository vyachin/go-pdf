# Design

## Context

The writer builds `/BaseFont`, the descriptor and CIDs from the parsed font plus the
used set. `pdfdebug` inflates every object and sniffs operators. The raster backend
draws `ImageOp` without its clip.

## Goals / Non-Goals

**Goals:** the file and the preview describe the same document; a hostile value fails
with a diagnostic; no panic.

**Non-Goals:** full OpenType name-table subset naming; changing the font subsetting
algorithm.

## Decisions

- Compute a deterministic six-letter tag from the subset bytes (first six letters of a
  SHA-256 base32) and prefix `/BaseFont` and `/FontName`; expose `CapHeight()` and
  `ItalicAngle()` from `internal/font` (`OS/2` `sCapHeight`, `post` `italicAngle`).
- Validate metadata with `utf8.ValidString` before writing; report `options/title` and
  the equivalent codes.
- Return `resource/limit` from `buildCIDs` when `len(keys) > 65535`, and validate
  `op.Text.Font != nil` when collecting fonts.
- Normalise outline levels into a slice used for both the tree and the comparison.
- Intersect the raster destination with the scaled clip; if the intersection is
  empty, skip the image.
- `pdfdebug` walks `/Pages` → `/Kids` and reads each `/Contents`; sort object ids.

## Risks / Trade-offs

- Subset names change every generated PDF → regenerate the document and render
  goldens in one dedicated commit and keep the diff reviewable.
- Reading descriptors adds font accessors → keep them optional and default to the
  previous values when the table is absent.
