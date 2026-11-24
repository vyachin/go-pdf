# Design

## Context

See proposal.md. `paintMarginBoxes` shapes the label and does `if err != nil { continue }`. `placeBlock` handles `Table`, text blocks, and default children; a `Kind == Image` box has no children, so it completes in one step and paints nothing.

## Goals / Non-Goals

**Goals:**

- No silent loss: every shaping failure and every image box either renders or reports.
- Block images paginate like text and tables.

**Non-Goals:**

- Image scaling rules beyond the existing `imageSize` helper.
- Background images or figures.

## Decisions

### 1. Margin box failure is an error diagnostic

On a shaping error the painter emits `text/glyph` with the box name and the underlying error, then skips that box. The diagnostic is an error, not a warning, so it matches body text and strict mode has nothing extra to escalate.

*Why the box name*: several margin boxes can exist; the message must say which one.

### 2. A dedicated branch for replaced blocks

`placeBlock` gains `case b.Kind == Image`, calling `placeImageBlock`, which:

1. resolves `w, h` with `imageSize(style, image, width, ctx)`;
2. if the image does not fit and the page is not empty, starts a new page and returns incomplete;
3. paints one image operation at the content-box origin;
4. advances `f.y` by the image height.

`finishFragment` and `afterBlock` then add padding, border and margin as for any block.

*Why reuse `imageSize`*: inline images already honour width, height and aspect ratio; block images must behave the same.

### 3. Overflow follows the block rule

An image taller than the page is clipped with the existing `layout/overflow` warning when the page is empty, and paginated otherwise, matching the table-row behaviour.

## Risks / Trade-offs

- **A broken footer now fails conversion** where it used to be dropped; that is the contract of the review and of body text.
- **Image-only documents** keep working; the new branch covers them as well, so the previous accidental path is no longer required.
