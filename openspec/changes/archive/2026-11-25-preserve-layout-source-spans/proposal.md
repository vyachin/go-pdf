# Proposal

## Why

Layout text pieces retain strings and computed styles but lose their originating node. As a result `text/glyph` and `text/font` are emitted with an empty internal position even when the offending text has a known source, making diagnostics hard to act on.

Review reference: **A03** (improvement), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Carry source provenance from DOM text and generated-content declarations through pieces, lines and font/glyph diagnostics.
- Report known element positions for layout overflow and retain source mapping for legacy encodings.
- Do not fabricate a precise character location when normalization or entity expansion makes it unavailable.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: Require layout diagnostics to retain known source provenance.

## Impact

`internal/layout/lines.go:piece` has no node/span and `buildLines` reports failures at `diag.Position{}`. `Box` retains its node but `warnOverflow` also discards position. `PageStyle.Boxes` retains content tokens without declaration positions. The HTML document already maps decoded offsets back to original bytes.

RV4-10 and RV4-39 own parser spans and column semantics, and RV4-37 owns missing font-file declarations. This change preserves that provenance through layout rather than changing the parser or font-loader contracts.

No new public Go identifiers or external dependencies are required.
