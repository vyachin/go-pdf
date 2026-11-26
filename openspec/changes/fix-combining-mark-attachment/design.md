# Design

## Context

See `proposal.md` — Why. `ShapeWithOptions` walks the normalized runes and keeps
three pieces of state: `prev` for kerning, `baseState`/`hasBase` for mark
attachment, and `markState`/`hasMark` for mark-to-mark attachment. The non-mark
branch updates `baseState` and `lastBase` but leaves `hasMark` and `markState`
untouched. A mark therefore prefers `MarkMark` against the previous cluster's
mark, producing the verified `о́о́` misplacement. The existing requirement
"Combining marks are positioned over their base" states the intended behaviour;
the implementation does not carry the cluster boundary.

## Goals / Non-Goals

**Goals:**

- Attachment is computed per cluster: a new base starts a new cluster.
- `MarkMark` is used only inside a cluster, with a `MarkBase` fallback.
- An anchor that would paint the mark away from its base is a diagnostic, not a
  silent misplacement.
- Correctly attached text is unchanged byte for byte.

**Non-Goals:**

- Reordering or splitting mark sequences, or changing normalization.
- Full GPOS mark filtering sets (`IgnoreMarks`, `UseMarkFilteringSet`); that is a
  separate lookup-selection concern already tracked for the GPOS path.
- Changing the missing-mark diagnostic code or the text layer; both stay as they
  are.

## Decisions

**One cluster state, reset on every non-mark glyph.** Replace the two independent
flags with a small state: `prev` (always), `base` (the last non-mark glyph) and
`clusterMark` (the last mark since that base, if any). The non-mark branch clears
`clusterMark`; the mark branch decides from it. This makes the invariant
local and testable with a table of (previous kind, current kind) cases.

**Attachment precedence.** If the previous glyph is a mark of the current cluster,
try `MarkMark(prev)` and then `MarkBase(base)`; if the previous glyph is the base,
try `MarkBase(base)` then `MarkLig(base)` for ligature components. There is no
third fallback: a mark without any of these anchors is the existing
`MissingMarkError`, which the engine already turns into `text/glyph`.

**Validate the resolved position, not the lookup.** A font may return an anchor
that is syntactically valid and geometrically wrong. After resolving `offX`, the
shaper checks that the mark's painted pen lies inside the owning base's glyph
cell (its advance box expanded by the font's side bearings for the mark) and
inside the shaped run. A failure is reported as `MissingMarkError` with the base
rune so the message names the real problem. The tolerance is expressed in font
units and documented in a constant next to the check, so a later reader can widen
it with evidence.

**Keep offsets in font units until the end.** The existing `scaleFontUnits` already
clamps the final conversion; the validation runs before scaling, in the same units
as the anchors, so no rounding decision changes the outcome.

## Risks / Trade-offs

- **A legitimate overhanging accent could be refused** → the tolerance uses the
  font's side bearings, and the regression corpus includes real fonts
  (DejaVu, Liberation) whose accents overhang; the check is on the owning base's
  cell, not on a fixed box.
- **Mark-to-mark fallback changes existing output** → only sequences whose font
  lacks a `MarkMark` anchor change; those were previously drawn at the pen or at a
  stale anchor.
- **The cluster rule is easy to regress** → a table-driven unit test enumerates
  the state transitions, and the render fixture asserts the second acute's
  x-position relative to the second base.

## Migration Plan

No API or stored-data change. Documents with decomposed accents re-render with the
mark over the correct base; the changelog records the fix. No caller action is
required.
