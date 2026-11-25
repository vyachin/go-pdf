# Design

## Context

`MaxDOMDepth` is checked while tokenizing against an open-element stack that does not
model implied end tags, implied elements or merged `html`/`body`. The intern table is a
fixed 65,536-entry array; `intern` returns 0 after it fills. CSS strings keep their
backslash escapes, and `border` parses a colour from a single token.

## Goals / Non-Goals

**Goals:** the depth limit matches the actual tree; no silent stack corruption; CSS
values decode; border colours match `color`.

**Non-Goals:** reimplementing HTML5 tree construction in the token pass; supporting
attribute selectors.

## Decisions

- Keep the token pass as a cheap early budget check. Compute the exact depth with an
  iterative walk over the `*xhtml.Node` tree after `html.Parse`, before the conversion
  model is built, charging one unit per visited element.
- Keep a sound early rule for the canonical over-limit shape: a run of start tags that
  can never be implicitly closed (`div`, `ul`, `ol`, `span`, `b`, `strong`, `em`, `i`),
  reset on any end tag, disabled by `table`/`svg`/`math`/`select`/`template` or an
  unsupported element, with `html`+`body` accounted for. It is a lower bound, so it
  cannot over-reject.
- Grow the intern table by doubling; if it cannot grow, return a dedicated
  `resource/limit` problem naming tag names, never a shared zero.
- Wrap `xhtml.Parse` errors: a depth/stack error becomes `resource/limit`, a charset
  error stays `html/encoding`, anything else becomes a coded error with a position.
- Implement CSS escape decoding in `unquote` for strings and identifiers; reuse it for
  `font-family` and `content`. Split the selector list on token boundaries.
- For `border`, parse the value as a token slice: collect widths/styles, pass the
  remaining tokens to the colour parser; treat `transparent` as zero visible width.
- In the import, resolve `em` widths before subtracting; parse longhands when the
  shorthand fails and warn for `auto` and negative margins.

## Risks / Trade-offs

- The standard tree for an over-limit document is allocated before the exact walk →
  bounded by `MaxInputBytes`; the early rule keeps the canonical nested shape cheap.
- A future x/net release changes implied behaviour → a differential depth-oracle test
  over generated shapes compares the enforced depth with the actual tree.
- Attribute validation may reject documents that convert today → start with a warning
  for unknown attributes; only `hidden` hiding is an error.
- Growing the intern table increases memory for pathological inputs → cap it at a
  documented count and fail explicitly beyond that.
