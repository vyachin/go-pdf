# Proposal

## Why

An independent review of the engine found three defects that break correctness or availability rather than features: the embedded font object graph is malformed in every document with text, a truncated GPOS table crashes font parsing, and pagination can loop until its iteration cap while producing a "successful" PDF with no content. All three are reachable from ordinary input.

## What Changes

- **PDF font objects.** A font SHALL be written as five linked objects (Type0, CIDFontType2, FontDescriptor, FontFile2, ToUnicode). The current writer allocates four and overwrites the CIDFontType2 object with the descriptor, so `/DescendantFonts` points at a descriptor. This is fixed, and a test follows the object graph instead of trusting strings in the file.
- **Malformed font tables.** Parsing a truncated or inconsistent optional table SHALL NOT panic. Bounds are checked per table format before reading, and a corrupt optional table is ignored (the font loads without kerning) while a corrupt required table is an error.
- **Pagination without progress.** If a page cannot accept any content, pagination SHALL stop with an error diagnostic and SHALL NOT return a successful PDF. The iteration cap becomes an error instead of a warning.
- Regression tests for each confirmed reproduction from the review are added and kept.

Out of scope: the remaining review findings (P2/P3) — kerning in PDF, image resource ordering, rowspan columns, `thead` rows, `@media` nesting, parent content box positioning, bottom padding, inheritance through `html`/`tr`, `white-space: pre`, `text-indent`, unitless `line-height`, symlink boundary, cancellation and resource budgets, public API, importer cleanup. They need their own changes; listing them here keeps this one completable.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: the font object graph requirement is added, with a test contract of following references rather than searching for strings.
- `pagination`: a requirement for terminating without progress and never returning an empty successful document.
- `text-shaping`: a requirement that malformed optional font tables do not crash parsing.

## Impact

- **Code**: `internal/pdf/writer.go` (font object allocation and linking), `internal/font/gpos.go` (bounds checks), `internal/layout/paginate.go` (progress detection), plus the callers that must surface the new error.
- **Behaviour**: documents that previously produced a malformed font graph now produce a valid one; a template whose content cannot fit on a page now fails with `layout/no-progress` instead of emitting thousands of empty pages.
- **Tests**: object-graph test, truncated GPOS regression, pagination reproduction cases, a fuzz seed; the mpDF compatibility corpus and the document corpus must stay green.
- **Process**: this change is created under the new documentation language rule, so its artifacts are written in English even though the archived specs are still Russian; those specs are translated when they are next edited.
