# Proposal

## Why

Two ways the engine drops content without telling anyone:

- margin boxes (headers and footers) ignore a shaping error: `text.Shape` fails for a character missing from the font, the loop continues, and the document is produced without the header and without a diagnostic;
- a block-level `<img>` among block siblings is turned into an `Image` box, but block placement has no branch for it, so it walks its empty children and draws nothing. An image-only document works only because it reaches the inline path.

## What Changes

- A margin box that cannot be shaped SHALL emit a `text/glyph` error diagnostic naming the box, consistent with body text, so conversion fails in normal and strict modes.
- Block-level replaced images SHALL be placed with their resolved dimensions, advance the flow, and paginate like any other block.

Out of scope: the remaining review findings.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pagination`: margin boxes report shaping failures; block images participate in pagination.
- `document-layout`: block images are placed as replaced blocks.

## Impact

- **Code**: `internal/layout/margin.go`, `internal/layout/paginate.go`.
- **Behaviour**: documents that silently lost a footer or an image now fail or render correctly.
