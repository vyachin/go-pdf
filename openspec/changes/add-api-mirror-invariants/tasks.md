# Tasks

## 1. Field-set invariants

- [ ] 1.1 Add `TestPublicTypesMirrorInternals` in `pdfgen`: reflect over `pdfgen.Options`, `pdfgen.Limits`, `pdfgen.Diagnostic` and the corresponding `engine.Options`, `engine.Limits`, `diag.Diagnostic`; compare the ordered list of `(name, type)` pairs and the field count, and fail with the field and type name. Verify the test fails when a temporary field is added to `engine.Limits` only, and passes once it is removed.
- [ ] 1.2 Extend the test to `pdfgen.Diagnostic` against `diag.Diagnostic`, including that `Severity` is mirrored and no field is unexported. Verify with a temporary added field on each side.

## 2. Round trip and defaults

- [ ] 2.1 Add `TestOptionsReachTheEngine`: set every `pdfgen.Options` field to a unique marker by kind, call `toEngine`, and compare every `engine.Options` field reflectively. Verify a field removed from `toEngine` makes the test fail for that field.
- [ ] 2.2 Add `TestLimitsAndDiagnosticsRoundTrip`: every `pdfgen.Limits` field survives `toEngine` and `fromEngineLimits`; every `diag.Diagnostic` field survives `fromDiags` in both directions of the public conversion. Verify by temporarily dropping one mapping.
- [ ] 2.3 Replace the manual field chain in `TestDefaultLimitsMatchesConversion` with a reflective loop over the limits, keeping the oversized-input assertion. Verify `TestDocumentedLimitsMatchTheCode` and the conversion assertion stay green.

## 3. Correct the mirrored godoc

- [ ] 3.1 In `internal/engine/engine.go`, change the `MaxTableCells` comment from "the number of cells in one table" to the grid area (column count times row count), matching the public field and `tableGridColumns`. Verify the sentence names the multiplication.
- [ ] 3.2 In the same file, replace the `RenderPNG` sentence "used by tests and the CLI debug flag" with a debug preview of the first page. Verify `grep -n 'CLI debug' internal/engine` returns nothing.
- [ ] 3.3 Add a source-level test in `internal/engine` that reads `engine.go` and requires the grid-area phrase for `MaxTableCells` and the debug-preview phrase for `RenderPNG`, so the CLI claim cannot return. Verify the test fails on the parent commit and passes here.

## Workflow follow-up

- Run `openspec validate add-api-mirror-invariants --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/code-quality/spec.md` and archive the change after review.
