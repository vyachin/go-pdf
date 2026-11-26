# Tasks

## 1. Parse a style attribute as a declaration list

- [ ] 1.1 Add declaration-list mode to the block parser: end of input terminates it, and `{` or `}` records a `css/syntax` problem and resumes after the token. Verify with table-driven tests in `internal/css/css_test.go` for a leading brace, a middle brace and a trailing brace.
- [ ] 1.2 Report an at-rule in an attribute as `css/unsupported-at-rule` without descending into it or discarding it silently. Verify with `@media`, `@page` and `@font-face` cases; each produces the code once.
- [ ] 1.3 Keep preceding declarations: `color:red} display:flex` leaves `color:red` in the returned list. Verify the declaration list in a parser test and the applied colour in a cascade test.

## 2. Surface the problems through validation and conversion

- [ ] 2.1 Confirm `profile.ValidateSheets` and the engine's invalid-option path translate the new problems into `css/syntax` and `css/unsupported-at-rule` diagnostics with the attribute's position. Verify with `internal/profile/profile_test.go` cases asserting the code, line and column.
- [ ] 2.2 Verify the public outcome: `<p style="} display:flex">` and the nested `@media` form return an `*pdfgen.Error`, produce no PDF, and name both the brace and the unsupported declaration. Verify with `internal/engine/convert_test.go` and `go test ./pdfgen -run 'Validate|Convert'`.
- [ ] 2.3 Confirm well-formed attributes are untouched: `style="color:#c00;font-size:10pt"` produces no diagnostics and the same PDF bytes as the parent commit. Verify with the existing style-attribute suite and a byte comparison in `internal/engine/convert_test.go`.

## 3. Regression coverage and documentation

- [ ] 3.1 Add adversarial fixtures `p2-style-attribute-brace.html` and `p2-style-attribute-at-rule.html`, registered with `Want: "css/syntax"` and `Want: "css/unsupported-at-rule"`. Verify with `go test ./internal/engine -run 'TestAdversarial|TestAdversarialManifest'`.
- [ ] 3.2 Run `go test -race -shuffle=on ./internal/css ./internal/profile ./internal/engine ./pdfgen`.
- [ ] 3.3 Update the style-attribute paragraph in `docs/profile.md` to state that an attribute is a declaration list and that braces and at-rules are reported, and add a `Changed` entry to `CHANGELOG.md`. Verify the prose matches the tests and `openspec validate fix-style-attribute-profile-bypass --strict` passes.

## Follow-up

- Run `openspec validate fix-style-attribute-profile-bypass --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/css-profile/spec.md` and archive the change after review.
