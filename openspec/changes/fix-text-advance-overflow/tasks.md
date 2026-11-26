# Tasks

## 1. Width accumulation

- [ ] 1.1 Replace the `int32` accumulation in `text.AdvanceWidth` with an `int64` sum and an explicit clamp to `MaxInt32`; expose saturation to the caller without changing the `int32` geometry contract. Verify with unit tests: 350,000 `ж` yields `MaxInt32` and `Saturated == true`; 1,000 `ж` is exact and not saturated; a run exactly at `MaxInt32` keeps the value and reports saturation only if the true sum is larger.
- [ ] 1.2 Accumulate `Line.Natural` in `int64` with the same helper and pass saturation through the line item. Verify that a mixed line of many runs never yields a negative natural width and that `TestLongWordIsReported` still passes.
- [ ] 1.3 Add a `GOARCH=386` build/test check for the new tests (a CI job or a documented local command) and verify the 350,000 character fixture reports the same width and diagnostics as on the 64-bit host.

## 2. Overflow diagnostic

- [ ] 2.1 Extend the `placeText` overflow condition to `line.Saturated || int64(line.Natural) > int64(width)`, keeping the existing `overflowWarned` guard, position and `layout/overflow` code. Verify that the 350,000 character word reports exactly one warning and the PDF is produced.
- [ ] 2.2 Verify the regression pair: 1,000 characters report `layout/overflow` (as today) and a normal paragraph reports none; add a table-cell case so a saturated cell line also warns once through the shared path.
- [ ] 2.3 Verify the warning does not multiply across fragments or pages: a long word in a split paragraph reports once per owning box, and the diagnostic position names the paragraph, not the page.

## 3. Scaling and face metrics

- [ ] 3.1 Introduce one clamp helper for `float64` millipoint conversion and use it in `Scale`, `scaleFontUnits` and `MetricsOf`, rounding once with `math.Round` before clamping. Verify unit tests for in-range values (unchanged), positive and negative overflow, and a zero `unitsPerEm`.
- [ ] 3.2 Report `layout/overflow` once at the first element that uses a face whose ascent, descent or advance required clamping. Verify that an extreme `font-size` document reports the warning and does not render a negative advance.
- [ ] 3.3 Run `gofmt -l`, `go vet ./...`, `go test -shuffle=on ./...`, `go test -race ./internal/text ./internal/layout ./internal/engine`, and `go test` under `GOARCH=386`; verify the Latin-only golden snapshots are unchanged and `openspec validate fix-text-advance-overflow --strict` passes.

## Workflow follow-up

- Sync the delta into `openspec/specs/text-shaping/spec.md` and archive the change
  after review.
