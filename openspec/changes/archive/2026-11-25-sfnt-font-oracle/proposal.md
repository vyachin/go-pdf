# Proposal

## Why

The font package is the riskiest hand-written code in the engine: it walks a binary table directory, reads `cmap`, `GPOS`, `glyf` and composite glyphs, and rewrites `loca`, `glyf`, `hmtx` and `head` for the subset — all under the promise that a hostile font never panics. It is currently checked against itself: `TestSubset` re-reads the generated subset with the same parser that wrote it, and `pdfcpu` validates the PDF around the font, not the font's tables. An independent decoder of the same format removes that circularity. The Go project maintains one — `golang.org/x/image/font/sfnt` — and the repository already carries fourteen real TrueType fonts, so the differential suite costs a test-only dependency and no fixtures.

## What Changes

- A test-only dependency on `golang.org/x/image/font/sfnt` SHALL compare the engine's font parser with the Go project's decoder on every TrueType font committed under `testdata`.
- The comparison SHALL cover `cmap` lookups over a fixed code point range, advances, vertical metrics and glyph outlines, with a one-unit tolerance for rounding and a canonical start point for contours.
- The subsetter SHALL be verified independently: its rewritten tables MUST parse with `sfnt` once the two tables `sfnt` requires and the subsetter drops on purpose (`cmap` and `post`) are restored from the original font, keep the glyph count, and preserve the advances and the outlines of the retained glyphs.
- A font the engine rejects, or whose `cmap` format it does not support, SHALL be skipped with a logged reason; the suite MUST fail when it compares no font at all.
- Kerning SHALL NOT be compared: `sfnt` implements a Latin-only subset of `GPOS` with a different lookup model, so a difference there is not a defect.
- `CONTRIBUTING.md` SHALL name `sfnt` as the oracle of the font-parser exception, next to the exception list introduced by `go-core-first`.

Not included: using `sfnt` in production (it cannot subset and does not expose raw tables), comparing rasterised glyphs, and changing any runtime behavior or PDF output.

## Capabilities

None. The change adds tests and a test-only dependency and changes no observable behavior; it sets `skip_specs: true` in its metadata.

## Impact

- **Dependencies**: `golang.org/x/image v0.47.0` joins `go.mod` and `go.sum` as a test-only requirement. It is maintained by the Go project, so the `code-quality` allowlist covers it, and it stays out of the package's runtime graph.
- **Tests**: a differential test file in `internal/font`, a subset test that uses `sfnt` instead of the engine's parser, a corpus walk over the committed fonts and a fuzz target seeded with them.
- **Code**: no production code changes.
- **Documentation**: the oracle line in `CONTRIBUTING.md`; a changelog note for the test dependency only if the release notes list test-only modules.
- **CI**: no new tool and no network at test time; the fonts and the module version are already pinned in the repository.
