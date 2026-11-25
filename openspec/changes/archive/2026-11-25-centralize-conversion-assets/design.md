# Design

## Context

See `proposal.md` for the finding and review reference.

`internal/engine/convert.go:prepared`, `engine/images.go`, `engine/fonts.go` and `profile/styles.go` currently open roots and maintain independent caches. `fsx.Root` already owns confinement and should remain the single low-level reader. Explicit `FallbackFont` is caller-selected and has a different confinement policy from template-provided URLs.

Preserve the independently specified invariants in `cache-image-digests` and `propagate-fallback-font-limits`. RV4-34 already specifies refunding failed image loads; reuse that behavior rather than defining a conflicting accounting rule.

## Goals / Non-Goals

**Goals:**

Unify resource policy without adding a public API, global cache or network access.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Put the shared reader, identities, admission state and typed failures in `internal/assets`, depending only on `internal/fsx`, `internal/budget` and the standard library. Engine preparation owns one registry and passes narrow interfaces to profile, font and image adapters, avoiding an import from profile back into engine. Format parsers and typed parsed-object caches remain in their existing adapters, keyed by the shared record identity.

2. Separate content identity from access authorization. Key successful records by kind, canonical file identity and decoding inputs such as stylesheet charset. Check each reference's root and role policy before looking up or sharing a record. This permits a declared font and explicit fallback to share bytes after both references are authorized, without giving a template the explicit caller path's broader permissions.

3. Share the same font file reached through `@font-face` and `FallbackFont` when both references are permitted, counting bytes once. Cache immutable stylesheet parsing while retaining every inclusion at its original cascade position.

4. Return typed failures carrying resource kind, source reference and cause. Reserve and check the applicable limit before a read or decode, then commit successful accounting or roll the reservation back on failure. Preserve source attribution per reference. Engine preparation must close the registry on preparation errors; `Convert` and `Validate` must defer closure after successful preparation so cancellation and later failures release every root handle.

**Alternative considered:** A process-global cache can cross access policies and conversions; use a conversion-scoped registry around the existing confined reader.

## Risks / Trade-offs

A broad cache can accidentally merge different decoding contexts or change cascade order. Avoid a process-global cache; coordinate with the independent digest and fallback-limit fixes and preserve their tests.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
