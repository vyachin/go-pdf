# Proposal

## Why

System-font discovery converts resource-limit errors into an unavailable face. With `MaxTotalFontBytes: 1`, the reviewed call returns `text/font` rather than the documented `resource/limit`, and a refused variant can silently change the selected face.

Review reference: **R08** (P2), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Distinguish an absent fallback candidate from a limit or cancellation failure.
- Propagate per-font, aggregate and parser limits through discovery and face selection.
- Stop trying alternatives after a resource failure and suppress dependent missing-family diagnostics.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: Preserve resource and cancellation causes while searching system fallback fonts.

## Impact

`fontRegistry.loadFontFile` in `internal/engine/fonts.go` returns only `*font.Font` and discards `admitFont`, limited-read and parse errors. `loadSystemFamily` treats every nil as absence. The shared guard records parser exhaustion but not a per-file or aggregate refusal, so the comment that the guard carries the reason is inaccurate.

RV4-44 covers work charges for fallback parsing and RV4-62 suppresses errors after guard exhaustion. This change separately preserves per-file and aggregate-limit errors that never enter the guard; coordinate the same functions without duplicating those requirements.

No new public Go identifiers or external dependencies are required.
