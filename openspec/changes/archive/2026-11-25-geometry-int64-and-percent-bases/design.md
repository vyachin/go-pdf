# Design

## Context

Layout stores millipoints in `int32`; several sums are computed ad hoc. The cascade
computes styles without a containing-block width, so the layout cannot resolve a
percentage against it. Page margins are parsed by the same box shorthand as element
margins, which accepts `auto` and percentages.

## Goals / Non-Goals

**Goals:** no wrapped geometry; CSS-correct percentage bases; a validated page margin
grammar.

**Non-Goals:** changing the internal unit; implementing percentage heights against an
indefinite height (they remain auto).

## Decisions

- Add `satAdd`/`satMul` helpers returning an int32 plus an overflow flag; callers
  report a `layout/overflow` diagnostic on overflow and clamp.
- Store the containing-block width on `Box` during `usedGeometry` and use it for
  `width`, padding, margins and `text-indent`; remove the `b.width`/`avail` bases.
- Keep `width: 0` as zero and clamp only negative or over-wide values to the available
  width.
- Parse `@page` margins with a page-specific validator: lengths only, no `auto`, no
  percentages, no negatives; resolve `em` at the initial 12 pt.
- For `size: portrait|landscape`, swap the resolved fallback when its orientation
  differs.

## Risks / Trade-offs

- Percentage changes move existing layouts → regenerate the goldens for fixtures that
  use percentages and review each page.
- Saturation hides a layout bug → always emit the diagnostic, and test the wide-image
  and huge-padding cases explicitly.
