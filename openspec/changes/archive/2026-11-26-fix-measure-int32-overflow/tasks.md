# Tasks

## 1. Saturating measurement

- [x] 1.1 In `internal/layout/paginate.go`, accumulate every branch of `measure`
  in `int64` (declared height, table, image, lines, child boxes and their margins)
  and clamp once at the return; raise a negative result to zero. Verify with
  `padding:2000000pt;height:2000000pt`: the result is non-negative and
  `measure` called twice returns the same value.
- [x] 1.2 Convert the flow's bottom insets in `afterBlock` to `satAdd`/`satSub`.
  Verify a block with 2,000,000 pt bottom padding does not place the next block at
  a wrapped Y.

## 2. Saturating painting

- [x] 2.1 Replace plain additions in `paintLineAt` (`internal/layout/paint.go`)
  with `satAdd`, including the baseline arithmetic. Verify a line at an extreme Y
  is either painted inside the page or reported, never at a wrapped coordinate.
- [x] 2.2 Replace plain additions and the `bottom-top` subtraction in
  `decorationOps` with `satAdd`/`satSub`; clipping a table's decoration to the page
  is owned by `fix-layout-decoration-defects` (RV5-43). Verify a box whose
  background crosses the page edge emits no rect with a wrapped coordinate.
- [x] 2.3 Replace the row-height sums in `paintRowAt` (`internal/layout/table.go`)
  with `satAdd`. Verify a row with an extreme line height does not produce a
  negative row height and the following row is not painted above it.
- [x] 2.4 In `paintStaticBlock` (`internal/layout/table.go`), report
  `layout/overflow` once through `warnOverflow` when a line starts at or below the
  page limit instead of skipping it silently. Verify the huge line-height cell
  reports exactly one diagnostic and the rest of the document still renders.

## 3. Regression tests

- [x] 3.1 Add a test for the huge line-height cell: `A<br>B` must both be
  extractable or the loss must be reported, and the following paragraph's baseline
  must be below the cell. Verify the test fails on the parent commit.
- [x] 3.2 Add a test for `padding:2000000pt;height:2000000pt`: the measured height
  is non-negative and exactly one `layout/overflow` is reported. Verify the test
  fails on the parent commit.
- [x] 3.3 Run `go test -shuffle=on ./internal/layout ./internal/engine` and the
  render snapshots; add a `CHANGELOG.md` entry under `### Fixed`.

## Workflow follow-up

- Run `openspec validate fix-measure-int32-overflow --strict` and fix any reported
  issue.
- Sync the delta into `openspec/specs/document-layout/spec.md` and archive the
  change after review.
