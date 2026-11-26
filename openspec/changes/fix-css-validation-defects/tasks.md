# Tasks

## 1. Length and colour values

- [ ] 1.1 In `internal/css/props.go`, reject a `line-height` that resolves to zero or below for percent, `em` and absolute forms. Verify with table-driven cases in `internal/css/style_test.go`: `-50%`, `0%`, `-2em`, `0em` rejected; `1.5`, `18pt`, `120%` accepted.
- [ ] 1.2 Make `currentColor` the initial border colour and accept the keyword in every border colour position, resolving it from the element's `color` at style resolution. Verify with unit tests in `internal/css/harden_test.go` and one rendered case in `internal/engine/convert_test.go` (red border under `color:#c00`).
- [ ] 1.3 Confirm an explicit `border-color` still wins and that a `transparent` border stays invisible. Verify the existing `TestBorderAcceptsEveryColorForm` plus new explicit-colour and transparent cases.

## 2. Positions, at-rule placement and the problem cap

- [ ] 2.1 Carry the owning element's position into inline declaration validation so a rejected inline declaration reports the element's file, line and column. Verify with `internal/profile/profile_test.go` and `internal/engine/convert_test.go` asserting a non-zero position equal to the element's.
- [ ] 2.2 Pass the parent at-rule context into margin-box parsing and reject a margin box outside `@page` with `css/unsupported-at-rule`. Verify with `internal/css/nesting_test.go` and `internal/profile/profile_test.go`; confirm a margin box inside `@page` still paints.
- [ ] 2.3 Count each parser problem once: one append helper increments, and merging a nested block does not re-count. Verify with sixty malformed declarations in one rule and in one `@media` block; both report sixty `css/syntax` and no `resource/limit`; the stray-brace cap test still returns `resource/limit`.

## 3. Regression and documentation

- [ ] 3.1 Regenerate any golden snapshot whose border colour changes and review the diff page by page. Verify with `go test ./internal/engine -run 'TestDocumentSnapshots|TestRenderSnapshots' -update` followed by a clean re-run.
- [ ] 3.2 Add adversarial fixtures for a negative `line-height` and a top-level margin box, registered with their expected codes. Verify with `go test ./internal/engine -run 'TestAdversarial|TestAdversarialManifest'`.
- [ ] 3.3 Update `docs/profile.md` (`line-height` values and border colour default) and `CHANGELOG.md` under `Changed`. Verify the prose matches the tests and `go test -race -shuffle=on ./internal/css ./internal/profile ./internal/engine ./pdfgen` passes.

## Follow-up

- Run `openspec validate fix-css-validation-defects --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/css-profile/spec.md` and archive the change after review.
