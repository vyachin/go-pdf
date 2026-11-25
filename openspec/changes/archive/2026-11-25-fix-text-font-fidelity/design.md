# Design

## Context

`text.Shape` maps one rune to one glyph with no mark logic. `font.gpos` parses only
pair positioning. `text.Split` handles LF, CR and the hyphen but not the Unicode
separators. The CFF converter writes int16 deltas without a range check and the
composite reader ignores the offset flags.

## Goals / Non-Goals

**Goals:** correct rendering for decomposed marks and separators; corrupt CFF values
fail; common CFF fonts load.

**Non-Goals:** full OpenType shaping (GSUB, cursive attachment); bidi.

## Decisions

- Parse GPOS lookup types 4–6 into a mark-attachment table keyed by
  (markClass, baseCoverage) and apply it in `Shape` after mapping the runes; when a
  mark has no attachment, return a `MissingGlyphError`-style diagnostic.
- Add U+000C, U+2028 and U+2029 to the break set in both `Split` branches.
- Check each encoded delta against `math.MinInt16..math.MaxInt16` and return an error
  from `encodeSimpleGlyph`.
- Read flags 0x0800/0x1000 in composite placement and transform the offset when
  `SCALED_COMPONENT_OFFSET` is set.
- Treat a missing `Private` operator as an empty private dictionary.

## Risks / Trade-offs

- Mark positioning is a sizeable addition to the shaper → ship it behind a table
  parser that tolerates absent lookups, and keep the existing kerning path untouched.
- Treating separators as breaks changes line counts in fixtures → update the affected
  goldens in the same change and add the fixtures to the corpus.
