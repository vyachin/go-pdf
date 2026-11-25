# Proposal

## Why

Images, declared fonts, explicit fallback fonts and stylesheets each implement their own identity, loading and error paths. This duplication allows repeated hashing, duplicate font accounting and loss of resource errors, and makes the conversion-wide resource contract difficult to audit.

Review reference: **A01** (improvement), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Provide a conversion-scoped `internal/assets` registry with shared confined-read, identity and typed-failure handling and narrow adapter interfaces.
- Keep format-specific parsers and limits in their existing owners.
- Reuse admitted image/font records across declarations and pagination passes while preserving stylesheet inclusion order.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: Specify consistent per-conversion asset identity, accounting and failure provenance.

## Impact

`internal/engine/convert.go:prepared`, `engine/images.go`, `engine/fonts.go` and `profile/styles.go` currently open roots and maintain independent caches. `fsx.Root` already owns confinement and should remain the single low-level reader. Explicit `FallbackFont` is caller-selected and has a different confinement policy from template-provided URLs.

Preserve the independently specified invariants in `cache-image-digests` and `propagate-fallback-font-limits`. RV4-34 already specifies refunding failed image loads; reuse that behavior rather than defining a conflicting accounting rule.

No new public Go identifiers or external dependencies are required.
