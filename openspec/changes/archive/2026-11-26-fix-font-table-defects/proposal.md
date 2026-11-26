# Proposal

## Why

Five independent font-table defects produce wrong output or silently drop data:

- A valid GPOS `ScriptList` that names only a non-ordinary script (for example
  `arab`) makes lookup selection return `nil`, which means "every lookup". Latin
  text then receives the Arabic `kern` pair; a crafted font measured -300 between
  `A` and `V`.
- Exceeding the coverage cap during lookup parsing drops the remaining lookups
  silently: four `kern` lookups covering 60,000 glyphs load as three, with no
  diagnostic and different kerning.
- A composite glyph whose component id is at or beyond `numGlyphs` is skipped by
  both the outline reader and the subsetter, producing a dangling component
  reference in the embedded font.
- A truncated simple or composite glyph returns an empty or partial outline with
  no error, so a malformed glyph vanishes instead of being reported.
- CFF-to-TrueType conversion copies `cmap`, `GPOS`, `name`, `OS/2` and `post` but
  not `GDEF`; a converted font therefore loses mark classes and filtering sets,
  changing which marks `IgnoreMarks` skips.

Each defect is reachable from a document-supplied font and none of them is
covered by the current suite.

## What Changes

- Distinguish "no usable `ScriptList`" (keep the all-lookups fallback) from "a
  usable list that names no ordinary script" (enable nothing). A lookup that
  belongs to another script MUST NOT apply to Latin or Cyrillic text.
- Treat every coverage-cap crossing as `resource/limit`; never return a partial
  lookup set.
- Reject a composite component id outside `numGlyphs` at parse and subset time
  instead of dropping it.
- Distinguish an empty glyph (equal `loca` offsets, legal) from a truncated glyph
  (malformed), and return an error for the latter instead of an empty outline.
- Copy `GDEF` in CFF conversion so mark classes and `IgnoreMarks` behaviour
  survive, with the glyph numbering already preserved.
- Add crafted fixtures for each case and update the font documents.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: script selection, coverage limits, composite and glyph
  validation, and the table set preserved by CFF conversion are specified.

## Impact

- `internal/font/gpos.go` (`enabledLookups`, coverage cap sites),
  `internal/font/glyph.go`, `internal/font/subset.go`,
  `internal/font/cff_convert.go`.
- Tests in `internal/font` (crafted GPOS, dangling component, truncated glyph,
  CFF table set) and the font corpus where relevant.
- `docs/fonts.md` (script selection, caps, malformed glyphs, CFF table set);
  `CHANGELOG.md`.
- No exported Go API change. Fonts that are structurally valid keep their output;
  malformed fonts that previously loaded with dropped data now fail with a
  diagnostic or `resource/limit`.
