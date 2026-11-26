# Design

## Context

See `proposal.md` — Why. The font parser already treats malformed optional tables
as "feature disabled, font usable" and bounds-checks most table reads. The GPOS
lookup selection added for script filtering is the exception: it validates
`ScriptList` but not `FeatureList`.

## Goals / Non-Goals

**Goals:**

- No read outside the GPOS table during lookup selection, for every crafted offset.
- Preserve the existing behaviour for well-formed tables and for fonts whose GPOS
  table is absent or truncated.

**Non-Goals:**

- Rejecting a font whose GPOS table is malformed. The profile keeps such a font
  usable without kerning, as the malformed-optional-table requirement states.
- Adding a diagnostic code for a malformed font table. Kerning is optional and no
  document text is lost.

## Decisions

**Bounds-check at both reads, not only at the header.** `featureLookups` receives
the offset and is the function that slices it, so the check belongs there as well
as in `enabledLookups`. The header check alone would still leave a later feature
record read to `fits`, which is correct, but a defensive check in the function
keeps the invariant local. Alternative considered: pass the whole GPOS slice and a
parsed feature list; rejected because it would allocate and change the parser
shape for a four-line fix.

**Out-of-bounds means empty, not error.** The parser's contract for optional tables
is degrade-not-reject, and `TestTruncatedGPOSDoesNotPanic` already accepts both
outcomes. Returning an empty set (not `nil`) from `enabledLookups` when a usable
`ScriptList` exists but the `FeatureList` is out of bounds keeps the existing
"no ordinary script enables nothing" semantics and prevents the accidental "nil
means all lookups" path from turning a corrupt file into full kerning.

## Risks / Trade-offs

- **A malformed font silently loses kerning** → accepted, and already the
  documented behaviour for optional tables; the panic is the only unacceptable
  outcome.
- **A future reader removes the check** → the committed fuzz seed and the
  end-of-file fixture fail immediately, so the regression is visible.

## Migration Plan

Not applicable: internal parser fix, no public API, no stored data, no output
format change. Release note under `Fixed`.
