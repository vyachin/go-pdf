# Design

## Context

See `proposal.md` for the finding and review reference.

`internal/layout/paint.go:paintLineAt` splits a run around glyphs with `XOffset`/`YOffset`. `internal/layout/margin.go:paintMarginBoxes` converts directly to `layout.Glyph`, losing those fields. `paintListMarkerAt` also has its own glyph-copy loop. The public PDF operation need not gain vertical text state because body text already handles offsets at layout emission.

No overlapping active change was found for this finding.

## Goals / Non-Goals

**Goals:**

Use one offset-aware shaped-run emission path without changing the public API or the PDF writer contract.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Extract the existing body segmentation into a layout helper taking origin, face, color and `text.Run`; keep offsets until operations are emitted.

2. Route body, margin boxes and list markers through the helper; retain caller-owned line alignment, baseline selection and warning attribution.

3. Compare normalized per-glyph positions at equal face and size; assert positive/negative X/Y offsets, stacked marks and extraction. Keep plain runs emitted as before.

**Alternative considered:** A separate margin-box offset patch duplicates the body emitter and risks another divergence; share the offset-aware emission operation.

## Risks / Trade-offs

Changing segmentation can affect PDF text-object order or byte output. Preserve the existing body representation and require independent extraction plus position checks, not only string equality.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
