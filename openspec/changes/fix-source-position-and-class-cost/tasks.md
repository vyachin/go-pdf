# Tasks

## 1. Linear position lookup

- [ ] 1.1 In `internal/html/node.go`, add an incremental per-line cursor to `Document.Position` for the no-mapper path and charge the bytes it examines. Verify with a unit test that queries positions in ascending and descending order on one long line and gets the same line/column as the existing implementation.
- [ ] 1.2 Add `BenchmarkPositionSingleLine` at 1,000 and 8,000 queries over a 112 KB line. Verify with `go test ./internal/html -run '^$' -bench PositionSingleLine` that per-query cost does not grow with the offset.
- [ ] 1.3 Add a regression test that 8,000 validation diagnostics on a one-line document finish with a budget sized for the multiline equivalent; verify `go test ./internal/profile -run 'PositionQuadratic'` passes and fails on the parent commit.

## 2. Linear element path

- [ ] 2.1 In `internal/html/node.go`, build `Path` with `strings.Builder` and charge one unit per byte written instead of one per class. Verify with a test that a 40,000-class element produces the same path as before and with a benchmark showing linear growth.
- [ ] 2.2 Verify the charged work: a tiny `MaxOperations` around the path cost returns `resource/limit`, and a path built below the budget completes.

## 3. Class tokens

- [ ] 3.1 Add `splitClasses` that splits on `' '`, `\t`, `\n`, `\f`, `\r` and use it in `Class`; cache the result on the node. Verify with tests for `class="a\u00a0b"` (one token, `.a` does not match), `class="a \t b\nc"` (three tokens), and an empty or whitespace-only attribute (no tokens).
- [ ] 3.2 Verify that repeated matches reuse the cache: a test with fifty class selectors against one element asserts the split happened once (for example by observing that the cached slice is returned unchanged), and `go test -race ./internal/html ./internal/css` passes.

## 4. Documentation and release note

- [ ] 4.1 State in `docs/profile.md` that class tokens split on ASCII whitespace only, and add the position-lookup unit to the work-budget table. Verify the text matches the implemented split and charge.
- [ ] 4.2 Add an `Unreleased` / `### Fixed` entry to `CHANGELOG.md` for the linear position lookup and the no-break-space class token. Verify no API is named as removed.

## Workflow follow-up

- Run `openspec validate fix-source-position-and-class-cost --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/html-parsing/spec.md` and archive the change after review.
