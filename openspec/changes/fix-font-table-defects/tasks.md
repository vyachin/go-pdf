# Tasks

## 1. Lookup selection and coverage caps

- [ ] 1.1 Change `enabledLookups` in `internal/font/gpos.go` to return an empty non-nil set when a usable `ScriptList` names no ordinary script, keeping `nil` only for a missing or unusable list. Verify with a crafted only-`arab` GPOS: Latin `AV` receives no kerning, while the same font with `DFLT`/`latn` still kerns.
- [ ] 1.2 Verify the fallback case explicitly: a GPOS with no usable `ScriptList` still enables its lookups, so the existing single-script crafted fixtures and `TestLookupSelectionUsesTheOrdinaryScripts` stay green.
- [ ] 1.3 Return `limitf` from each coverage-cap site in `parseGPOS` instead of `nil`, and abort the font load. Verify a crafted four-lookup, 60,000-glyph-coverage font fails with `resource/limit` and that a three-lookup font within the cap loads with all lookups enabled.
- [ ] 1.4 Verify a truncated lookup is still "no lookups, font loads" and is not reported as `resource/limit`; add both cases as unit tests with the expected error class.

## 2. Composite and glyph validation

- [ ] 2.1 In `internal/font/glyph.go`, validate every composite component id against `numGlyphs` inside `components` and return an error naming the glyph. Verify a crafted composite with component id equal to `numGlyphs` reports the error instead of an empty component.
- [ ] 2.2 In `internal/font/subset.go`, replace the silent skip of an out-of-range component with the propagated error, so a subset is never written with a dangling reference. Verify the subsetter returns the error and the conversion produces no PDF; a valid composite still embeds every component.
- [ ] 2.3 Distinguish an empty glyph from a truncated one in `simpleGlyph` and `compositeGlyph`: equal `loca` offsets return an empty outline, a non-zero range that ends inside its data returns an error. Verify with three fixtures: legal empty glyph, truncated simple glyph, truncated composite; the last two are errors and the first is not.
- [ ] 2.4 Verify the error reaches the user through the existing diagnostic path (the engine maps a font glyph error to a diagnostic naming the element) and that the process does not panic on any of the crafted inputs.

## 3. CFF GDEF and regression coverage

- [ ] 3.1 Add `GDEF` to the table list copied by `internal/font/cff_convert.go` when the source has it, and keep the deterministic table ordering. Verify with a Barlow-style CFF fixture: the converted font contains `GDEF`, and a pair lookup with `IgnoreMarks` skips the mark glyph as the source does.
- [ ] 3.2 Update `TestConvertedFontKeepsTables` to assert the full table set including the conditional `GDEF`, and verify a CFF font without `GDEF` still converts with the previous table set.
- [ ] 3.3 Extend `docs/fonts.md` with the script-selection rule, the coverage-limit rule, the glyph-validation rule and the CFF table set. Verify the statements match the tests and that no documented cap number is invented.
- [ ] 3.4 Add an adversarial or corpus fixture for each crafted case where the public boundary is reachable, and verify `go test ./internal/font ./internal/engine -shuffle=on` and `-race` pass.
- [ ] 3.5 Add a `Fixed` entry to `CHANGELOG.md` naming the five defects, then run `gofmt -l`, `go vet ./...`, `go test ./...` and `openspec validate fix-font-table-defects --strict`; verify all pass.

## Workflow follow-up

- Sync the delta into `openspec/specs/text-shaping/spec.md` and archive the change
  after review.
