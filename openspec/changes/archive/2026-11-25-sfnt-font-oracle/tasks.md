# Tasks

## 1. Dependency and corpus

- [x] 1.1 Add `golang.org/x/image v0.47.0` as a test-only requirement and tidy the module — check: `go list -deps ./pdfgen` contains no `x/image`; the only imports are in `_test.go` files
- [x] 1.2 Write the corpus helper: walk `testdata` for `*.ttf`, parse each file with both decoders, apply the three skip rules and require at least ten compared fonts — check: the corpus yields fourteen fonts, every skip is logged with a reason, and a run with an empty corpus fails
- [x] 1.3 Write the shared comparison helpers: the advance tolerance, the contour-to-segment conversion and the start-point canonicalisation — check: unit tests cover an on-curve start, an off-curve start, two consecutive off-curve points and a composite glyph

## 2. Character map and metrics

- [x] 2.1 Compare `GlyphIndex` over U+0000–U+FFFF and the supplementary sample — check: the corpus is green; a copy with a renamed `cmap` table fails with the code point named
- [x] 2.2 Compare `NumGlyphs`, `UnitsPerEm`, advances and vertical metrics with a one-unit tolerance — check: the corpus is green; a copy with one changed `hmtx` entry fails and names the glyph
- [x] 2.3 Confirm the private parser is not used as the reference anywhere in the new tests — check: the new file imports `sfnt` and the production package under test, nothing else font-shaped

## 3. Outlines

- [x] 3.1 Convert the engine's contours to the canonical segment sequence and compare with `sfnt.LoadGlyph` for every glyph of every compared font — check: the corpus is green; a copy with one changed `glyf` byte fails and names the glyph
- [x] 3.2 Benchmark the outline comparison and keep the suite within budget — check: the corpus run stays under two seconds; if it does not, sample glyphs and record the sampling rule

## 4. The subset oracle

- [x] 4.1 Subset the glyphs of a fixed Latin and Cyrillic string, restore the `cmap` and `post` tables the subsetter drops and `sfnt` requires, parse the result with `sfnt`, and assert the glyph count, the advances and the outlines of the retained glyphs — check: green on both bundled fonts, and the restored `cmap` still resolves every rune to the same glyph, which proves the numbering was preserved
- [x] 4.2 Prove the oracle can fail: corrupt a rewritten `loca`/`glyf` byte in a test copy and assert that `sfnt` rejects it or the comparison fails — check: the corrupted case fails for the expected reason

## 5. Fuzzing

- [x] 5.1 Add `FuzzParserAgreement`, seeded with the corpus bytes: the engine must not panic; when both decoders accept the input, `NumGlyphs`, `UnitsPerEm` and a small `cmap` sample must agree — check: the seeds run in `go test ./internal/font`; a short `-fuzz` run reports no failure

## 6. Documentation and acceptance

- [x] 6.1 `CONTRIBUTING.md`: name `sfnt` as the oracle of the font-parser exception, in the list introduced by `go-core-first` — check: the sentence is present and matches the design
- [x] 6.2 CHANGELOG: record the test-only requirement if the release notes list test dependencies — check: the Unreleased section
- [x] 6.3 `go vet ./...`, `go test ./...` and `go test -race ./...` with `PDF_REQUIRE_VALIDATOR=1` — check: green
- [x] 6.4 The pinned `golangci-lint` run — check: green
- [x] 6.5 `openspec validate sfnt-font-oracle --strict` — check: valid
