# TODO from the code review

There are no open tasks. Below are summaries of the review passes; the detailed items, reproductions and edit history were removed from this file and remain in git.

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
