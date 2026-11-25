# Tasks

## 1. One break decision

- [x] 1.1 Replace the eager `newPage` for `break-before` in `placeChild` and the pending check in `placeBlock` with one pending flag consumed when body content is placed, cleared by `newPage` — verify: `break-after` followed by `break-before` produces exactly two pages with the second paragraph on page 2, and the test fails before the fix.
- [x] 1.2 Track `pageHasBody` separately from `f.empty` and the operation count, and consult it in the single decision — verify: `break-before` on the first content yields one page, a pending break survives an empty block, and a repeated header does not count as body content.
- [x] 1.3 Cover a pending break satisfied by a transition taken for another reason (orphans/widows) — verify: no empty page is produced.
- [x] 1.4 Consume the break only when the box paints content — verify: the empty-div and height-only-div reproductions paginate as authored; no blank page for a trailing `break-before`.
- [x] 1.5 Remove the duplicated `applyFixedHeight` calls and the redundant break checks — verify: `go vet ./...` and the pagination suite are green with no behaviour change.

## 2. Rows, tables and lines

- [x] 2.1 Route row `break-after`/`break-before` through the same pending decision and keep repeated headers from marking the page as having body content — verify: a two-row `thead` with a row break pair makes exactly one transition; the table-background and body-background reproductions paint on the correct page and before the cells.
- [x] 2.2 Build the grid before `measureTable` uses `computeColumns`, charging the build once — verify: the two-column `break-inside: avoid` table moves whole to the next page.
- [x] 2.3 Include the indent and leading image in `Line.Natural` — verify: the right-aligned indent+image line ends at the content edge.
- [x] 2.4 Emit one `layout/overflow` diagnostic per box — verify: the tall-div fixture reports one warning.

## 3. Tests and documentation

- [x] 3.1 Add all reproductions as regressions, including the adjacent-break pair, trailing break, repeated header and empty page scan — verify: each fails before its fix and no forced-break fixture produces an empty page.
- [x] 3.2 Update `docs/profile.md` where break and indent semantics are described — verify: the documented behaviour matches the new scenarios.
- [x] 3.3 Add an Unreleased/Fixed entry to `CHANGELOG.md` — verify: the entry matches the page counts pinned by the tests.
