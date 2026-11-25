# Proposal

## Why

`go-core-first` replaced the largest private substitutes and the sort helpers. A handful of smaller ones remain: a manual `data:` URI split, a hand-written file-existence branch, benchmarks in the old loop form, and a test run whose order is never varied. None changes behavior; together they finish the same rule, and leaving them makes the next sweep harder to justify.

## What Changes

- The `data:` URI parsing in the image loader SHALL use `strings.Cut` and `strings.CutSuffix` instead of `IndexByte` and `TrimSuffix`.
- `fsx` SHALL test a missing file with `errors.Is(err, fs.ErrNotExist)` instead of `os.IsNotExist`.
- The engine benchmarks SHALL use `testing.B.Loop` instead of `for i := 0; i < b.N; i++`.
- The test job SHALL run with `-shuffle=on`; an order-dependent test it reveals SHALL be fixed, not skipped.

Not included: `t.Context()` in existing tests (churn without a defect), `encoding/xml` for `docdump` (the snapshot needs canonical formatting), and any behavior change.

## Capabilities

None. The change is a refactor and test tooling only; its metadata sets `skip_specs: true`.

## Impact

- **Code**: `internal/engine/images.go`, `internal/fsx`, `internal/engine/bench_test.go`.
- **CI**: the test and race steps gain `-shuffle=on`.
- **Tests**: no assertion changes; an order dependence may surface and must be fixed in this change.
- **Documentation**: `CONTRIBUTING.md` records the shuffled test command.
- **Compatibility**: none; output and diagnostics are identical.
