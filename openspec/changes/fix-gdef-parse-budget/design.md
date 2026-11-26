# Design

## Context

See `proposal.md` — Why. `parseGPOS` calls `parseGDEF` before it selects lookups.
`parseGDEF(data)` reads the class definition, expands every class-3 range into a
`map[uint16]bool`, and reads the mark glyph sets with
`(&kerning{}).parseCoverage`, which has a nil guard. Ranges are not required to be
sorted or disjoint, so the map cost is unbounded by the table length. The function
returns only maps, so the coverage loop's `limitf` is reduced to `continue`.

The font parser already stores the conversion's guard and has `check` helpers and
a `maxKerningGlyphs` cap for pair coverage. `docs/fonts.md` documents the caps and
the charging rule.

## Goals / Non-Goals

**Goals:**

- GDEF cost is charged before allocation and is bounded by a documented cap.
- Cancellation during GDEF parsing returns the context error promptly.
- A resource refusal is terminal and visible; malformed optional data still
  degrades.
- Fonts within the documented bounds load exactly as before.

**Non-Goals:**

- Rejecting every malformed GDEF table; the existing optional-table rule keeps a
  font usable without mark data.
- Changing how marks are applied (`IgnoreMarks`, filtering sets); that is the
  lookup-selection path.
- Charging for the plain `parseClassDef` ranges that are not class 3; they are
  bounded by the table and do not expand.

## Decisions

**Pass the guard into `parseGDEF` and charge per expansion.** The concurrency
contract of `budget.Guard` already allows one goroutine per conversion, and the
font parser holds the guard for GPOS. `parseGDEF` charges one unit per range plus
the number of entries it is about to write, before the write loop; `k.check()`
between ranges observes cancellation and budget exhaustion. The alternative, a
separate guard per table, was rejected because the conversion has one budget and
the proposal is about it.

**One cap for expanded mark glyphs.** Reuse the existing coverage-cap family: add
`maxMarkGlyphs` next to `maxKerningGlyphs` (or reuse the latter if the values
agree) and count every entry written, including duplicates that overwrite an
existing key. Counting writes, not distinct keys, keeps the bound independent of
the map's current size and makes overlapping ranges expensive by definition.

**Return an error only for a resource refusal.** `parseGDEF` gains an error
result. A table that is truncated or inconsistent returns nil maps and no error,
which preserves today's "load without mark data" behaviour; a `limitf`
(`budget.ErrExceeded`) is returned and `parseGPOS` aborts. The coverage loop stops
swallowing errors: `errors.Is(err, budget.ErrExceeded)` returns the error, any
other error continues with that set dropped.

**Charge before the map write, not after.** A charge after the loop would let a
hostile table allocate before the budget could refuse it, which is exactly the
failure the existing PairPos rule was written to prevent.

## Behavioral verification

Use independent expected range/entry costs and completed-work counters to prove
that GDEF pays before expansion. Overlapping ranges must charge repeated writes,
not only distinct map keys. Test budgets immediately below and at the next charge
and verify that a refused expansion never starts and no partial mark set escapes.
For cancellation, stop a controlled context at a known mid-parse checkpoint and
assert that the remaining work is not performed, no font is returned, and the
context error reaches both public entry points. The timed reproduction in the
proposal/spec is evidence of the defect, not a wall-clock test threshold. These
tests must detect removed charges or cancellation checks deterministically.

## Risks / Trade-offs

- **A font with a large but legitimate GDEF now fails** → the cap is set far above
  real fonts (DejaVu declares a few dozen mark glyphs) and is documented; the error
  is `resource/limit`, not a silent wrong output.
- **Extra guard calls slow normal parsing** → one `Add` per range, not per glyph;
  the normal table has few ranges, and `check` is throttled as elsewhere.
- **Rejecting a font that previously loaded with partial marks** → intended: a
  partial mark set silently changes attachment; the change makes that visible.
- **The exact cap value is a judgement call** → it is stated in `docs/fonts.md`
  next to the other caps and can be raised with a measurement.

## Migration Plan

No API change. A document whose font declares a GDEF beyond the cap now fails
with `resource/limit` instead of loading with partial mark data; the changelog
names the new bound. Fonts within the bound are unaffected.
