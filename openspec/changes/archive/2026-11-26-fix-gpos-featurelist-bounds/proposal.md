# Proposal

## Why

`enabledLookups` accepts the GPOS `FeatureList` offset whenever it is non-zero and
never checks it against the table, so `featureLookups` slices
`gpos[featureList:featureList+2]`. Go checks only the capacity, and a table slice
retains the capacity of the rest of the file: an offset past the table either reads
unrelated font bytes (silently disabling every lookup and losing kerning) or, when
the GPOS table sits near the end of the file, panics. The panic escapes
`pdfgen.Convert`, which violates the package's "No panics" guarantee and makes a
malformed `@font-face` a remote crash for the host process.

## What Changes

- Bounds-check the `FeatureList` header offset against the GPOS table before it is
  read, and check every later read inside `featureLookups` as well.
- Treat an out-of-bounds `FeatureList` as an unusable feature list: the font stays
  loadable without kerning, the same way the existing malformed-optional-table
  requirement already allows, and no byte outside the table is interpreted.
- Add a regression fixture built from a real font whose GPOS table is moved to the
  end of the file, plus a fuzz seed so the shape stays covered.
- Record the case in `docs/fonts.md` next to the existing malformed-table note.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: the malformed-optional-table requirement gains a specific,
  testable case for the GPOS `FeatureList` offset; the font-parsing work bound
  keeps applying to lookup selection.

## Impact

- `internal/font/gpos.go` (`enabledLookups`, `featureLookups`).
- `internal/font/font_test.go`, `internal/font/testdata/fuzz/FuzzParse` seeds.
- `docs/fonts.md`.
- No public API, output format or diagnostic code changes.
