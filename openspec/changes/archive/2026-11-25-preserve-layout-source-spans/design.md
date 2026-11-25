# Design

## Context

See `proposal.md` for the finding and review reference.

`internal/layout/lines.go:piece` has no node/span and `buildLines` reports failures at `diag.Position{}`. `Box` retains its node but `warnOverflow` also discards position. `PageStyle.Boxes` retains content tokens without declaration positions. The HTML document already maps decoded offsets back to original bytes.

RV4-10 and RV4-39 own parser spans and column semantics, and RV4-37 owns missing font-file declarations. This change preserves that provenance through layout rather than changing the parser or font-loader contracts.

## Goals / Non-Goals

**Goals:**

Make attributable layout diagnostics point to the text, element or CSS declaration that caused them.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Attach a small immutable provenance record to pieces and generated-content values: original node/source span and element path, resolved lazily when reporting.

2. For a body text error use the originating text node or closest owning element. Use the `content` declaration for margin-generated text and the actual owner for overflow; keep source offsets in the existing decoded coordinate space until mapping.

3. If NFC composition, entities or whitespace folding prevents a precise glyph byte mapping, report the known owning source location rather than inventing a character column. Keep genuinely aggregate/writer failures distinct.

**Alternative considered:** Preformatted paths on every glyph waste memory and lose mapping flexibility; keep shared provenance and resolve it when reporting.

## Risks / Trade-offs

Copied full paths per glyph would inflate memory. Carry shared references and charge any path construction. Source mapping changes under RV4-10/RV4-39 must be reused, not reimplemented.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
