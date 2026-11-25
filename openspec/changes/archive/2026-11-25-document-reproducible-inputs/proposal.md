# Proposal

## Why

The README and package comment promise identical output for the same document and options without stating that external stylesheets, images and font selection are inputs too. Identical paths can hold different bytes, and system fallback can select different fonts on different machines.

Review reference: **D03** (documentation), architecture/code/documentation review of `7c7ac70`.

## What Changes

- State the complete input boundary of byte reproducibility in the README, package comment and font guide.
- Align the output requirement with unchanged engine build, resolved resources and resolution choices.
- Show explicit font/resource pinning for production and preserve the convenience system fallback.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: Define reproducibility over the engine build and all resolved asset bytes.
- `library-docs`: Make reproducibility prerequisites visible wherever the guarantee is stated.

## Impact

`README.md` and `pdfgen/doc.go` make the abbreviated guarantee. `docs/fonts.md` already recommends shipping exact fonts and explains system-font variability. The current `pdf-output` requirement mentions fonts but omits external CSS and image bytes.

The fourth-review design mentions the same-font condition but has no dedicated complete-input documentation delta. RV4-57 separately removes a test environment hook; this proposal does not duplicate that implementation.

No new public Go identifiers or external dependencies are required.
