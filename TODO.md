# TODO from the code review

Open work is tracked as OpenSpec changes under `openspec/changes/`; this file records
the review passes and the permanent fixtures they left behind. The adversarial corpus
lives in `testdata/adversarial/` and is registered in
`internal/engine/adversarial_test.go`. It runs in the unit and race jobs, and its
manifest test fails when a listed fixture is missing or an HTML fixture is not
registered.

## Third pass — November 25, 2026

Reviewed the parser, CSS, font, image, layout and pagination limits and rechecked the
P1/P2 findings of the earlier passes. Every confirmed hostile case now has a permanent
fixture with a bounded outcome — a diagnostic code, one embedded object or a page
count — and none asserts elapsed time.

| Finding | Fixture | Expected outcome |
|---|---|---|
| P1 nested `@media` | `p1-nested-media.html` | `resource/limit`, also in a subprocess |
| P1 adjacent selector | `p1-adjacent-selector.html` | converts; the second paragraph is red |
| P1 many-spelling image | `p1-many-spellings.html` | one image object under `MaxOperations: 5000` |
| P1 crafted GPOS | `p1-crafted-gpos.html`, `gpos-bomb.ttf` | `resource/limit` under `MaxOperations: 250` |
| P2 wide image | `p2-wide-image.html`, `wide.png` | converts; no negative image scale |
| P2 DOM depth | `p2-dom-depth.html` | `resource/limit` at `MaxDOMDepth: 8` |
| P2 rules × elements | `p2-rules-elements.html` | `resource/limit` at `MaxOperations: 2000` |
| P2 repeated data URI | `p2-repeated-data-uri.html` | one image object |
| P2 page breaks | `p2-page-breaks.html` | two pages |
| P2 table grid | `p2-table-grid.html` | `resource/limit` at `MaxTableCells: 4` |

The reproductions that failed before each fix are recorded in the corresponding archived
changes under `openspec/changes/archive/2026-11-25-*`; the corpus itself was added by
`add-adversarial-regression-suite`.

## Second pass — November 24, 2026

Reviewed snapshot `12b26c4`: HTML/CSS parsers, fonts and text, layout and pagination, PDF writer, public API and CLI. **30 tasks closed: 5 P1, 18 P2, 7 P3** (RV2-01 … RV2-30). Every scenario was moved into a permanent regression test of the corresponding package, and the fixes went in as separate commits after `docs(todo): record the second review pass findings`.

## Previous pass — November 24, 2026

Reviewed snapshot `ef0e0d4`; **31 tasks closed: 2 P1 and 29 P2** (REV-01 … REV-34) plus three P3 proposals. All items were closed before the second pass; their evidence and acceptance records remain in the git history of `TODO.md`.

## History of earlier checks

Passes before November 24, 2026: 19 confirmed P2 bugs and improvement proposals, all closed. This section also preserved the checks' limitations (for example, no Poppler for independent text extraction and `staticcheck` unavailable for Go 1.27). Current status: `go vet ./...`, `go test ./...` and `go test -race ./...` with `PDF_REQUIRE_VALIDATOR=1` pass; PNG alpha and the resulting PDFs are checked by `pdfcpu` in strict mode.

## Improvements for separate follow-up

Resource budgets (HTML/CSS/fonts/images, pages, table cells, operations) and cancellation in all expensive stages are implemented and covered by tests; there are no open improvements in this section.

## Review addendum — 2026-11-24

The addendum's findings are closed: conversion cancellation, resource layout boundaries and the CI extension with an independent PDF check. There are no separate open items.

## Addendum: full review of the current project — November 24, 2026

P3 proposals are closed: shared geometry computation for blocks and cells (ARCH-01) and independent visual/text comparison of the resulting PDFs in CI (ENG-09). There are no open items.
