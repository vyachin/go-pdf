# Proposal

## Why

The suite fails on wall-clock thresholds rather than on behavior: the font
outline comparison exceeded its two-second budget by 65 ms (2.065 s) on the
`race` job while the code was unchanged. Timings depend on the machine, the race
detector and the runner load, so a fixed threshold turns a healthy build red and
hides the signal it was meant to provide.

## What Changes

- Remove every elapsed-time threshold from the test suite: the font oracle's
  two-second `outlineBudget`, the charset mapper's ten-second limit, the two
  thirty-second limits in `TestHostileEncodingInput`, and the opt-in
  performance test's 50 ms, 800 ms and 200 MB checks.
- Keep performance visible. The benchmarks stay, and `PDF_PERF=1` runs a report
  that logs timings and allocations without asserting a threshold; it replaces
  `TestPerformanceBudget` as `TestPerformanceReport`.
- Keep the correctness checks — compared font tables, decoded positions,
  diagnostics and page counts — and the resource `internal/budget` guard, which
  counts work rather than time.
- `CONTRIBUTING.md` describes the opt-in report instead of a budget test.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: adds a requirement that elapsed time never fails the suite,
  with performance carried by benchmarks and the opt-in report.

## Impact

- **Tests**: `internal/font/sfnt_oracle_test.go`,
  `internal/charset/mapper_test.go`, `internal/engine/encoding_test.go` and
  `internal/engine/bench_test.go`.
- **Documentation**: the performance section of `CONTRIBUTING.md`.
- **Product code**: none; the engine and `internal/budget` are unchanged.
- **CI**: the same jobs and gate, but a slow runner no longer fails the suite on
  time.
