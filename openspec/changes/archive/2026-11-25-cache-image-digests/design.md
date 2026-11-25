# Design

## Context

See `proposal.md` for the finding and review reference.

`internal/engine/images.go` already caches an `ImageData` by canonical file path or data-URI content. `internal/engine/convert.go:toPDFOps` discards that identity and calls `sha256.Sum256(img.Data)` per operation. `pdf-output` requires one decode and object, but not one hash.

The later `centralize-conversion-assets` proposal must preserve this invariant. RV4-06 in `fourth-review-hardening` charges writer bytes; this finding concerns the earlier `toPDFOps` hashing stage and is independently actionable.

## Goals / Non-Goals

**Goals:**

Keep hashing proportional to unique encoded asset bytes rather than placement count.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Store the digest on an immutable per-conversion image record or memoize it by image-data pointer; use content digests to retain deduplication across distinct files with identical bytes.

2. Perform the first hash through a budget-aware chunked helper; charge unique byte blocks before hashing and retain a constant placement charge.

3. Preserve first-use resource order, content-based deduplication and byte-identical output for requests within budget.

**Alternative considered:** Memoizing only the PDF pointer would miss identical content from different files; retaining a digest on the loaded record preserves content deduplication.

## Risks / Trade-offs

A pointer-only cache is insufficient for different files containing the same bytes. Additional legitimate byte charges can reject documents previously accepted under a small custom budget; document that correction.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
