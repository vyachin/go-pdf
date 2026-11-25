# Design

## Context

Engine diagnostics are built with an empty `diag.Position` in several places. Options
are read after stylesheet collection. `referencedTargets` records every anchor, not
only the ones `target-counter` uses. `Diagnostics.Err` copies the full list.

## Goals / Non-Goals

**Goals:** every document diagnostic is attributable; option errors are reported
first and together; no false `unknown-target`; error lists contain errors.

**Non-Goals:** adding positions to `options/*` (they have no source); changing the
diagnostic order within a file.

## Decisions

- Add `Pos` at the diagnostic sites where a node is available (`doc.Pos(n)`), and fill
  `File` from `Options.File` in `fromDiags` for every code except `options/*`.
- Parse `PageSize`, `Margin`, `Date` and `Charset` at the top of `prepare` with the
  existing parsers, collect their diagnostics, and continue only when they are valid.
- Validate `Date` by formatting and checking a four-digit year; report `options/date`.
- Record whether the document contains a `target-counter` during stylesheet
  validation; `referencedTargets` returns only the ids it uses, and the warning carries
  the anchor position.
- Filter warnings in `Diagnostics.Err` (copy the error entries) and document that
  `Result.Diagnostics` keeps both.
- Validate attributes against an allow-list derived from the profile docs; map
  `hidden` to `display: none` during the profile pass.

## Risks / Trade-offs

- Attribute validation may reject documents that currently convert → start with a
  warning for unknown attributes, and make `hidden` hiding the only error.
- Adding positions can reorder diagnostics → the sort key already includes file, line
  and column; update the goldens once.
