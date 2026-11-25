# Design

## Context

See `proposal.md` for motivation. Five wall-clock thresholds exist today: the
font oracle's `outlineBudget` (2 s) over the whole corpus, the charset mapper's
10 s limit on 20,000 positions, two 30 s limits inside
`TestHostileEncodingInput`, and the opt-in `TestPerformanceBudget` (`PDF_PERF=1`)
with 50 ms, 800 ms and a 200 MB ceiling. The outline check failed at 2.065 s on
the `race` job of an otherwise unrelated pull request. The suite already has
benchmarks for one page, fifty pages and the outline comparison, plus
deterministic correctness checks. `internal/budget` is a product guard that
counts work units, not time.

## Goals / Non-Goals

**Goals:**

- The suite outcome depends only on behavior, not on machine speed.
- Performance numbers stay one command away.
- The resource guard and every correctness assertion stay.

**Non-Goals:**

- Removing benchmarks or the opt-in report.
- Changing the engine's limits or the `internal/budget` API.
- Adding a scheduled performance CI job.

## Decisions

### 1. Delete fixed thresholds instead of raising them

A threshold has to hold on the slowest runner. Raising 2 s to 5 s only delays the
same flake; the 2.065 s measurement on a normal runner shows the margin is noise
rather than signal.

### 2. Benchmarks and an opt-in report carry performance

`go test ./internal/engine -bench .` stays. `PDF_PERF=1` runs
`TestPerformanceReport`, which logs time and allocations for one page and fifty
pages and asserts nothing, so it is safe on any machine. `TestPerformanceBudget`
is renamed because it no longer enforces a budget.

### 3. The allocation ceiling goes with the time thresholds

The 200 MB check was a budget in the same test. Keeping one budget would
reintroduce environment-dependent failure on a different axis. Allocation
numbers stay in the report and in the benchmarks.

### 4. Keep `internal/budget` and the correctness checks

`internal/budget` counts work units and is part of the engine's cancellation
contract; it is deterministic and unrelated to wall-clock time. Position,
diagnostic, page-count and font-table assertions stay, so removing timeouts does
not remove coverage.

## Risks / Trade-offs

- **A super-linear regression no longer fails CI** → the benchmarks and the
  opt-in report expose the numbers; the job's fifteen-minute timeout is a coarse
  backstop, and review compares benchmark output when a change touches hot paths.
- **A slow test becomes a slow job** → the suite is bounded by the job timeout,
  not by per-test thresholds; the fix is to sample or optimize, recorded in the
  change that introduces the slowdown.
- **Allocation growth is no longer gated** → the benchmarks report allocations
  per operation, so a doubling is visible in review.

## Migration Plan

None: tests and one documentation paragraph. Reverting restores the thresholds.

## Open Questions

None.
