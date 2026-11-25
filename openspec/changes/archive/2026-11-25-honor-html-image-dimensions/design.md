# Design

## Context

See `proposal.md` for the finding and review reference.

`internal/profile/profile.go` accepts the attributes and `docs/profile.md` lists them. `internal/layout/box.go:buildImage` ignores them, and `lines.go:imageSize` sees only CSS lengths. Presentation hints must be applied before author declarations, not after computed CSS where `auto` cannot be distinguished from an unspecified value.

No overlapping active change was found for this finding.

## Goals / Non-Goals

**Goals:**

Make the documented HTML sizing surface work with explicit and testable CSS precedence.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Parse trimmed ASCII non-negative integer attributes as CSS pixels (750 millipoints each); range-check in 64 bits. Zero is a valid non-visible dimension.

2. Apply the hints at author-presentation-hint priority before author CSS; an explicit CSS `auto` overrides the attribute and ordinary CSS remains authoritative.

3. Ignore malformed, negative, percentage and out-of-range HTML dimension attributes as absent, matching presentational-hint handling; state this rule in the profile. Do not extend table-cell attributes.

**Alternative considered:** Applying attributes after style computation cannot distinguish an explicit CSS `auto` from the initial value; use presentation-hint precedence before author declarations.

## Risks / Trade-offs

The correction intentionally changes PDFs that previously relied on ignored image attributes. CSS tests must cover `auto`, because overwriting the computed style afterward would produce the wrong precedence.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
