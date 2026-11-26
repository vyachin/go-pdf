# Tasks

## 1. Cluster state machine

- [x] 1.1 Refactor the glyph state in `internal/text/shape.go` into `prev`, `base` and a per-cluster `mark`, clearing the mark on every non-mark glyph. Verify with a table-driven test over (previous kind, current kind) that a base always starts a fresh cluster.
- [x] 1.2 Implement the attachment precedence: previous mark and same cluster → `MarkMark` then `MarkBase`; previous base → `MarkBase` then `MarkLig`; otherwise the existing `MissingMarkError`. Verify the offset of the second acute in `о́о́` and `а́б́` equals the `MarkBase` offset the first acute receives, relative to its own base.
- [x] 1.3 Keep the fallback for a font without `MarkMark`: two consecutive marks on one base use `MarkMark` when present and the current base's `MarkBase` otherwise. Verify with a synthetic font fixture that the second mark is never placed at a stale anchor.

## 2. Anchor validation

- [x] 2.1 After resolving an anchor, check that the mark's painted position lies over the owning base's glyph cell and inside the shaped run, with the documented side-bearing tolerance in font units. Verify a normal DejaVu and Liberation fixture passes unchanged.
- [x] 2.2 Report a crafted anchor that places the mark outside the base or the run as the existing missing-mark diagnostic, carrying the base rune. Verify the engine maps it to `text/glyph` with the element position and produces no misplaced glyph.
- [x] 2.3 Verify advances and extraction are untouched: the run width equals the sum of the glyph advances for every fixture, and extracted text keeps the combining sequence.

## 3. Regression coverage and release note

- [x] 3.1 Add a render/position fixture for `о́о́`, `а́б́`, `о́` + U+0302 and a leading mark in `internal/engine`, asserting the second mark's x-position lies within the second base's cell. Verify the test fails on the parent commit and passes here.
- [x] 3.2 Add unit tests in `internal/text` for the state table and for a mark with no base, and verify `go test ./internal/text ./internal/engine` passes with `-shuffle=on -race`.
- [x] 3.3 Add a `Fixed` entry to `CHANGELOG.md` describing the cluster reset, and verify `gofmt -l`, `go vet ./...`, `go test ./...` and `openspec validate fix-combining-mark-attachment --strict` pass.

## Workflow follow-up

- Sync the delta into `openspec/specs/text-shaping/spec.md` and archive the change
  after review.
