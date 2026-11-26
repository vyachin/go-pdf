# Tasks

## 1. Guarantee tokenizer progress

- [x] 1.1 Add tokenizer tests that call the lexer on `\`, on `color:red\`, on a lone `\` repeated many times and on a valid escaped identifier, asserting that each call terminates and reports `css/syntax` for the trailing escape. Verify with `go test ./internal/css -run 'TestTokenizerProgress'`.
- [x] 1.2 In `internal/css/token.go`, make `lexer.next` guarantee an advancing offset: when the scanner returns the same offset, consume one byte and emit a delimiter, recording a `css/syntax` problem for a trailing lone backslash. Verify the new tests fail on the parent commit (timeout) and pass here.
- [x] 1.3 Cover the block path: `<style>p{color:red\</style>` terminates with a problem rather than looping. Verify with a test in `internal/css/harden_test.go` that completes under `-timeout 30s`.

## 2. Charge and bound declaration parsing

- [x] 2.1 Pass the guard (and through it the context) into `ParseDeclarations` and charge one unit per produced token before it is stored. Verify with a test that a long but valid declaration list records units and a tiny budget returns `resource/limit`.
- [x] 2.2 Verify cancellation: with an already cancelled context, parsing a style attribute returns the context error instead of completing. Verify with a table-driven test covering both `ParseDeclarations` and the document entry point.
- [x] 2.3 Confirm the profile and engine call sites pass the conversion guard, so `Validate` and `Convert` share the same accounting. Verify with `go test ./internal/profile ./internal/engine -run 'Style|Budget'`.

## 3. End-to-end regression and documentation

- [x] 3.1 Add `testdata/adversarial/p1-trailing-backslash-style.html` and `testdata/adversarial/p1-trailing-backslash-sheet.html` and register both in `adversarialCases()` with `Want: "css/syntax"` or `resource/limit` plus small limits. Verify with `go test ./internal/engine -run 'TestAdversarial|TestAdversarialManifest'`.
- [x] 3.2 Run the race and shuffle suites for the touched packages: `go test -race -shuffle=on ./internal/css ./internal/profile ./internal/engine`.
- [x] 3.3 Record the fix in `CHANGELOG.md` under `Fixed`: a trailing lone backslash in CSS now reports `css/syntax` instead of hanging, and declaration parsing is charged. Verify the entry states the new failure mode and `go test ./internal/diag -run TestEveryEmittedCodeIsRegistered` stays green because no code is added.

## Follow-up

- Run `openspec validate fix-css-tokenizer-progress --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/css-profile/spec.md` and archive the change after review.
