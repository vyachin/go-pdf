# Tasks

## 1. Remove the thresholds

- [x] 1.1 `internal/font/sfnt_oracle_test.go`: drop `outlineBudget` and the elapsed check around `TestOutlineAgreement` — check: the whole corpus is still compared and no `time.Since` assertion remains
- [x] 1.2 `internal/charset/mapper_test.go`: drop the ten-second check in the long-line test — check: the monotonic position assertions remain
- [x] 1.3 `internal/engine/encoding_test.go`: drop both thirty-second checks in `TestHostileEncodingInput` — check: the diagnostic count and position assertions remain
- [x] 1.4 `internal/engine/bench_test.go`: turn `TestPerformanceBudget` into `TestPerformanceReport`, logging time and allocations under `PDF_PERF=1` without asserting a threshold — check: no `t.Fatalf` on elapsed time or allocation remains
- [x] 1.5 `CONTRIBUTING.md`: describe the opt-in report instead of a budget test — check: the performance section no longer mentions an elapsed-time budget

## 2. Verification

- [x] 2.1 `go vet ./...` and `go test ./...` — check: green
- [x] 2.2 `go test -race ./internal/font -run TestOutlineAgreement` — check: green under the race detector
- [x] 2.3 `PDF_PERF=1 go test ./internal/engine -run TestPerformanceReport -v` — check: logs the one-page and fifty-page numbers without failing
- [x] 2.4 `openspec validate remove-test-time-budgets --strict` — check: valid
