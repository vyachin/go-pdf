# Design

## Context

See `proposal.md` — Why. Five independent defects share one theme: the font parser
turns "I cannot represent this" into "silently carry on".

- `enabledLookups` returns `nil` both for "no usable `ScriptList`" and for "a
  usable list that matched no ordinary script". `parseGPOS` reads `nil` as "enable
  every lookup", so the second case applies foreign-script pairs.
- The coverage cap sites return `nil` for a lookup/subtable that crosses the cap,
  which the caller reads as "this lookup enabled nothing"; the remaining lookups
  are dropped.
- `components` and `Subset` skip a component id `>= numGlyphs`; `loca`-driven
  glyph data for such a component is absent, leaving a dangling reference.
- `simpleGlyph` and `compositeGlyph` return an empty or partial outline on a
  truncated range; the caller cannot tell "legal empty glyph" from "malformed".
- `cff_convert` builds the output table list explicitly and omits `GDEF`.

## Goals / Non-Goals

**Goals:**

- A pair defined under a non-ordinary script never applies to ordinary text.
- Resource caps are terminal and visible; a partial lookup set is impossible.
- No dangling component references and no silently empty malformed glyphs.
- A converted CFF font keeps its mark classes and filtering sets.
- Structurally valid fonts keep their current output.

**Non-Goals:**

- Implementing GSUB or applying GDEF beyond the existing `IgnoreMarks` and
  filtering-set handling.
- Rejecting every malformed optional table at load; the "degrade without the
  feature" rule stays for non-resource problems.
- Changing glyph numbering, subset layout or the PDF-visible glyph ids.

## Decisions

**Selection returns a set plus a "usable list" flag, not a nil sentinel.**
`enabledLookups` distinguishes an absent/unusable `ScriptList` (fallback: every
lookup, as today) from a usable list with no ordinary script (empty, non-nil set:
nothing enabled). `parseGPOS` keeps `if enabled != nil && !enabled[i]`, so an empty
non-nil map disables all lookups without a special case in the loop. Alternative:
an explicit `(set, ok)` return; rejected because the existing `nil` check already
has the right shape and the empty-map value is harder to misuse by accident.

**A cap crossing is an error, a malformed table is not.** The three coverage-cap
sites return `limitf` (`budget.ErrExceeded`) and `parseGPOS` aborts. Truncated or
inconsistent data keeps returning "no lookups" so the font still loads, preserving
the optional-table requirement. The distinction is `errors.Is(err,
budget.ErrExceeded)`, not the message.

**Validate components where they are read.** `components` already has the font and
the glyph id, so it returns an error for a component outside `numGlyphs`; the
outline reader and the subsetter both propagate it. The subsetter's `continue`
becomes a returned error, so a PDF never embeds a subset with a dangling
reference. This is lazy (only used glyphs are validated), which avoids rejecting a
font because of an unused broken glyph while still refusing to paint or embed it.

**Distinguish an empty glyph from a malformed one.** `loca[start] == loca[end]`
is the legal empty glyph (`space`, `.notdef` in some fonts) and returns an empty
outline with no error. A non-zero range that ends inside its data returns an
error naming the glyph. `compositeGlyph` applies the same rule to its component
records and returns a partial result only on success.

**Copy `GDEF` in CFF conversion.** Glyph ids are preserved by the converter, so
`GDEF` class definitions and filtering sets remain valid. The output table list
gains `GDEF` when the source has it; the converter keeps its existing
checksum/directory builder and its deterministic ordering.

## Risks / Trade-offs

- **A font with a malformed unused glyph still loads** → it fails when the glyph
  is used or subset, which is the point where the wrong output would appear.
- **Stricter font acceptance can break a customer's file** → the error names the
  glyph and the table; the changelog says malformed fonts that previously loaded
  with dropped data now fail. Valid fonts are unaffected.
- **Copying `GDEF` grows the subset slightly** (a few hundred bytes) and may
  change user-visible kerning for fonts whose `IgnoreMarks` was previously
  ignored → that is the intended correction; the fixture asserts it.
- **The empty-vs-malformed rule depends on `loca` monotonicity** → a decreasing
  `loca` remains a parse error, as today.

## Migration Plan

No API change. Re-render documents that used a malformed font: they now fail with
a diagnostic instead of producing wrong output. CFF documents may produce a
slightly larger PDF because `GDEF` is embedded; the change is recorded under
`Fixed` and `Changed` in the changelog.
