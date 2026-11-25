# Tasks

## 1. Fixtures

- [x] 1.1 Add the P1 fixtures (nested `@media`, adjacent selector, many-spelling image, crafted GPOS) with expected diagnostics under small budgets — verify: each fails on the parent commit and passes after the corresponding fix.
- [x] 1.2 Add the P2 fixtures (wide image, DOM depth, rules × elements, repeated data URI, page breaks, table grid) — verify: each asserts its code or byte-identical output.
- [x] 1.3 Add a subprocess helper for fatal-error cases — verify: the nested-media fixture exits 0 with a diagnostic and the helper reports a non-zero exit as a failure.

## 2. Gate and documentation

- [x] 2.1 Run the corpus in the unit and race jobs and fail on a missing fixture — verify: removing one fixture fails CI locally.
- [x] 2.2 Update `TODO.md` with this review pass and the fixture list — verify: the file names every fixture and no longer claims there are no open tasks.
- [x] 2.3 Document the corpus in `CONTRIBUTING.md` — verify: a contributor can add a fixture from the instructions.
