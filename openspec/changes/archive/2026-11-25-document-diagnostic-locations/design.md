# Design

## Context

See `proposal.md` for the finding and review reference.

`docs/profile.md` says every error has all location fields; `pdfgen/pdfgen.go` documents zero line/column as unknown. `library-api` currently says both "when a source position exists" and "Only options/* ... MAY lack a position". A serialization budget failure does not necessarily identify one source node.

RV4-27 concerns generated-content documentation and position consistency. This proposal adds an explicit unknown-position contract and does not replace `preserve-layout-source-spans`.

## Goals / Non-Goals

**Goals:**

Give callers a coherent location contract matching the public fields and actionable source errors.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Keep file identification for all document/asset failures and preserve linked stylesheet filenames. Options may omit file and location; genuinely aggregate/output failures may have zero line/column.

2. Clarify the conflicting main API sentence explicitly in the delta; preserve all existing missing-asset/source scenarios and add an aggregate scenario.

3. Document fallback locations with examples and zero-field meaning. Coordinate `preserve-layout-source-spans` so known locations become complete rather than reclassified as unknown.

**Alternative considered:** Weakening every diagnostic to an unknown location would discard actionable information; allow unknown only when the failure has no attributable source.

## Risks / Trade-offs

This is an explicit clarification of an internally inconsistent requirement, not permission to remove existing positions. Review compatibility against the current public godoc and keep diagnostic codes unchanged.

## Migration Plan

Update the documentation and its capability requirements together. Runtime behavior and dependency versions are unchanged; revert the documentation commit if the wording is inaccurate.
