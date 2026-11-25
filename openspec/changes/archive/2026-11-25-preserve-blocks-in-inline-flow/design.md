# Design

## Context

See `proposal.md` for the finding and review reference.

`builder.buildInline` in `internal/layout/box.go` emits a newline for a block child and recursively calls `buildInline` on that child instead of `buildElement`. An `img` has no children and disappears; tables and list items lose their special box construction.

No overlapping active change was found for this finding.

## Goals / Non-Goals

**Goals:**

Preserve every supported visible descendant and its block semantics in mixed inline flow.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Normalize the builder result into alternating inline runs and real block boxes, including anonymous wrappers for surrounding inline text.

2. Use `buildElement` for each block descendant and let the enclosing block builder place it in order; avoid special-casing only images.

3. Carry the inline wrapper's computed inherited style and id/anchor ownership across the split. Hidden descendants stay omitted and missing assets remain errors.

**Alternative considered:** Adding only image-specific logic would leave tables and lists flattened; normalizing mixed flow fixes the structural boundary.

## Risks / Trade-offs

Correct block splitting can change line breaks in templates relying on the old flattening. Restrict snapshot changes to affected fixtures and inspect content order independently.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
