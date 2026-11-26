# Tasks

## 1. Bounds-check lookup selection

- [x] 1.1 In `internal/font/gpos.go`, require `fits(featureList, 0, 2, len(gpos))` in `enabledLookups` and return an empty (non-nil) set when a usable `ScriptList` exists but the `FeatureList` does not fit. Verify with a unit test that a crafted GPOS with `FeatureList` past the table disables every lookup and does not panic.
- [x] 1.2 In `featureLookups`, check the header offset with `fits` before slicing and every feature-record read after it. Verify with a table whose header fits but whose record offset is out of bounds; the function returns no lookups and the test process stays alive.
- [x] 1.3 Keep the existing behaviour when the `ScriptList` is missing or unusable: `enabledLookups` still returns `nil`, meaning every lookup applies. Verify `TestLookupSelectionUsesTheOrdinaryScripts` and the kerning tests stay green.

## 2. Regression coverage

- [x] 2.1 Add a font fixture builder in `internal/font/font_test.go` that takes the bundled test font, moves its `GPOS` table to the end of the file, updates the directory entry and writes a `FeatureList` offset of `0xFFFF`. Verify that `Load` returns without panic and that kerning is disabled.
- [x] 2.2 Add the same fixture with a smaller offset that stays inside the file but outside the table, and assert that no lookup is enabled. Verify both fixtures fail on the parent commit (a panic or a non-empty lookup set) and pass here.
- [x] 2.3 Add the crafted bytes to `internal/font/testdata/fuzz/FuzzParse` as a seed. Verify with `go test ./internal/font -run '^$' -fuzz '^FuzzParse$' -fuzztime 20s` that the seed is exercised and no panic is reported.

## 3. Documentation and release note

- [x] 3.1 Extend the malformed-table paragraph in `docs/fonts.md` with the `FeatureList` case: an out-of-bounds feature list disables kerning and never panics. Verify the text names the same behaviour the test asserts.
- [x] 3.2 Add an `Unreleased` / `### Fixed` entry to `CHANGELOG.md` stating that a malformed GPOS feature list no longer panics and loses only kerning. Verify the entry names no removed API.

## Workflow follow-up

- Run `openspec validate fix-gpos-featurelist-bounds --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/text-shaping/spec.md` and archive the change after review.
