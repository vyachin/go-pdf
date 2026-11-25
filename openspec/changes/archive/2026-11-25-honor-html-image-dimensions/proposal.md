# Proposal

## Why

The profile advertises `img[width]` and `img[height]`, but `buildImage` reads only `src`. Images with HTML dimensions 10 by 10 and 200 by 200 currently produce byte-identical PDFs, while equivalent CSS sizing works.

Review reference: **R04** (P2), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Treat valid non-negative integer image dimension attributes as CSS-pixel presentation hints.
- Let authored CSS, including `auto`, override each corresponding presentation hint.
- Preserve aspect ratio when only one effective dimension is set and document invalid-attribute handling.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: Define HTML image dimension attributes and their precedence relative to CSS.

## Impact

`internal/profile/profile.go` accepts the attributes and `docs/profile.md` lists them. `internal/layout/box.go:buildImage` ignores them, and `lines.go:imageSize` sees only CSS lengths. Presentation hints must be applied before author declarations, not after computed CSS where `auto` cannot be distinguished from an unspecified value.

No overlapping active change was found for this finding.

No new public Go identifiers or external dependencies are required.
