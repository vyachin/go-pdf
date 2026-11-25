# Proposal

## Why

Body text positions combining marks using the shaper's offsets, but margin boxes copy only glyph id, rune and advance. The word `сло́во` therefore has different relative glyph positions in a header and the body even at the same font and size.

Review reference: **R07 and A02** (P2), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Preserve combining-mark offsets in headers and footers.
- Extract one internal shaped-run emitter for body text, margin boxes and markers so positioning fixes have one implementation.
- Keep kerning, text extraction, alignment and plain-text output stable.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: Apply mark attachment consistently to every painted shaped run, including margin boxes.

## Impact

`internal/layout/paint.go:paintLineAt` splits a run around glyphs with `XOffset`/`YOffset`. `internal/layout/margin.go:paintMarginBoxes` converts directly to `layout.Glyph`, losing those fields. `paintListMarkerAt` also has its own glyph-copy loop. The public PDF operation need not gain vertical text state because body text already handles offsets at layout emission.

No overlapping active change was found for this finding.

No new public Go identifiers or external dependencies are required.
