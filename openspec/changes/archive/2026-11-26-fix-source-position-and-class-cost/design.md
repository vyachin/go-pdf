# Design

## Context

See `proposal.md` — Why. `charset.Mapper` already tracks an incremental line and
column cursor for decoded documents, but the common UTF-8/no-BOM path in
`html.Document.Position` recomputes the column with `utf8.RuneCount` from the line
start. `Node.Path` builds each class segment with `+=`, and `Node.Class` calls
`strings.Fields` on every class check. Attributes are immutable after parsing, so
the token slice is cacheable.

## Goals / Non-Goals

**Goals:**

- Position lookup and path building become linear in the bytes they actually need.
- The work budget bounds both, with the same unit the rest of the parser uses.
- Class tokenization follows HTML's ASCII-whitespace rule.

**Non-Goals:**

- A byte-offset-to-rune-column index for every line. The incremental cursor is
  enough for the single-line worst case and allocates nothing.
- A general selector-match cache. Only the class token split is cached; the match
  itself stays stateless.
- Changing the column contract for legacy single-byte encodings; their byte and
  rune columns already agree.

## Decisions

**Incremental cursor keyed by line, not a precomputed rune table.** A table costs
one entry per character (the same amplification as `lineStarts`); a cursor stores
the last line and column and advances with `utf8.RuneLen` over the bytes between
the previous offset and the requested one. Alternative considered: compute the
column at scan time and store it on every node; rejected because most nodes never
need a position and the node struct is already large.

**`strings.Builder` plus a byte charge.** A builder still copies on growth, but
amortised growth is linear and the charge follows the bytes written rather than the
number of segments. Alternative considered: precompute the total length and write
into one buffer; rejected as unnecessary complexity for a diagnostic path.

**Cache class tokens on the node after parsing.** Attributes cannot change after
the tree is built, so the cache has no invalidation problem. Alternative
considered: cache in the matcher keyed by attribute string; rejected because it
would retain strings for the document's lifetime and duplicate the node identity.

## Risks / Trade-offs

- **A stale cursor after a node moves** → positions are computed from immutable
  byte offsets; the cursor only remembers the line of the last query. A test
  queries positions in both ascending and descending order.
- **Builder growth charge understates a worst case** → the charge is per byte
  written, which is the final path length, so the bound is correct within a
  constant factor.
- **A cached token slice adds memory per element with classes** → it is only
  allocated on first class access, and only for elements a selector asks about.

## Migration Plan

No public API or output change. Release note under `Fixed`. No stored data.
