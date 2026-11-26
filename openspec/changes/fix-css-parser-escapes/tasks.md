# Tasks

## 1. Selector escapes

- [x] 1.1 Add tests for `.a\.b`, `.a\ b`, `.a\2e b` and `.a.b` that assert the parsed compound structure and matching against `class="a.b"`, `class="a b"` and separate elements. Verify with `go test ./internal/css -run 'TestSelectorEscape'`.
- [x] 1.2 In `internal/css/selector.go`, escape the decoded name when selector text is built so a decoded punctuation character cannot become syntax. Verify the new tests fail on the parent commit (`.a\.b` matches `class="a b"`) and pass here.
- [x] 1.3 Confirm `.a.b` still needs both classes and an unescaped space still separates compounds. Verify with the existing selector suite in `internal/css/css_test.go`.

## 2. url() escapes

- [x] 2.1 Decode escapes in the URL token, quoted and unquoted, and make an unquoted `\)` close the argument. Verify with tokenizer tests for `url("a\20 b.png")`, `url(a\)b.png)` and an unterminated `url(a\(b.png)`.
- [x] 2.2 Verify end-to-end that `background-image: url("a\20 b.png")` loads a file named `a b.png` and that a decoded escape never reaches the resolver. Verify with `internal/css/image_hints_test.go` and an `internal/engine/convert_test.go` case asserting no `img/load`.
- [x] 2.3 Confirm remote and unsupported `data:` sources are still rejected with their existing codes. Verify with the existing `TestBackgroundImageRejectsUnsupportedSources` plus an escaped-path variant.

## 3. Regression and documentation

- [x] 3.1 Add a fixture with a class name containing a dot and an escaped image path, and assert the rendered output. Verify with `go test ./internal/engine -run 'Escaped'`.
- [x] 3.2 Run `go test -race -shuffle=on ./internal/css ./internal/profile ./internal/engine ./pdfgen`.
- [x] 3.3 Extend the CSS escape section of `docs/profile.md` with selector and `url()` decoding, and add a `Fixed` entry to `CHANGELOG.md` naming the selector and URL behaviour. Verify the prose matches the tests and `gofmt -l` stays clean.

## Follow-up

- Run `openspec validate fix-css-parser-escapes --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/css-profile/spec.md` and archive the change after review.
