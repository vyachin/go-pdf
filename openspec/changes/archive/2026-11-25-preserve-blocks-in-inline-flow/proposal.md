# Proposal

## Why

A block-level image inside an inline wrapper is silently discarded. The reviewed `<span><img src="logo.png" style="display:block"></span>` returns success with no image object, while the same image without the wrapper is present.

Review reference: **R03** (P2), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Split an inline flow around block descendants while preserving each descendant box.
- Keep replaced images, nested tables and list structure instead of reducing a block to its text descendants.
- Preserve content order, anchors, inherited styles and asset diagnostics.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: Require block descendants of inline elements to survive box-tree construction.

## Impact

`builder.buildInline` in `internal/layout/box.go` emits a newline for a block child and recursively calls `buildInline` on that child instead of `buildElement`. An `img` has no children and disappears; tables and list items lose their special box construction.

No overlapping active change was found for this finding.

No new public Go identifiers or external dependencies are required.
