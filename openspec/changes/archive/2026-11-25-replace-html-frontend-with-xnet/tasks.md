# Tasks

## 1. Dependency and policy

- [x] 1.1 Add `golang.org/x/net v0.62.0` to `go.mod` and commit the resulting `go.sum`; verify `go mod tidy` leaves exactly `golang.org/x/net` and `golang.org/x/text` as requirements and `go build ./...` passes
- [x] 1.2 Replace the "no dependencies of its own" claim in `README.md` with the actual requirement and the allowlist rule; verify the README states the Go version and the one module it now needs, and that `go doc ./pdfgen` still renders
- [x] 1.3 Add an `Unreleased` entry to `CHANGELOG.md` naming the standard HTML frontend and the replaced dependency rule; verify the entry names both and links no removed document

## 2. Token scan

- [x] 2.1 Implement the token pass that records one span per token and spends one `budget.Guard` unit per token; verify a unit test asserts that the spans partition the input byte for byte and that the token count equals the guard's usage
- [x] 2.2 Enforce `MaxDOMDepth`, the operation budget and context cancellation in the token pass, before the tree is built; verify tests return `resource/limit` for a document over the depth limit and the context error for a cancelled context
- [x] 2.3 Verify the depth limit is decided before tree construction: a test counts the guard units spent on a document far over the depth limit and asserts the count stays below the token count that the tree would have needed

## 3. Standard tree, conversion and positions

- [x] 3.1 Convert the standard tree into `internal/html.Node`, including parent links, attribute keys, namespace prefixes written as `prefix:name` and merged adjacent text nodes; verify a synthetic document asserts each of those four properties on the resulting model
- [x] 3.2 Match tree elements against the recorded spans in document order and assign positions; verify authored elements report their own line and implied nodes report their nearest positioned ancestor
- [x] 3.3 Verify the reordering fixtures `Issue532` and `list-style-type` produce no negative or zero position and inherit positions only in the reordered subtree
- [x] 3.4 Add parsing tests for the character-reference rules: `&rarr;`, `&nbsp` without a semicolon, `&#147;` mapping to a quotation mark, and an undefined name left literal

## 4. Normalization to the print profile

- [x] 4.1 Ignore white-space-only character data directly inside table structure; verify a document with indented rows snapshots and converts identically to the same table written on one line
- [x] 4.2 Ignore `/>` on a non-void element, as the standard does; verify `<div/>text` keeps `text` inside the `div` and a void element still closes
- [x] 4.3 Take an inline `<style>` range from the recorded span and re-slice the document source for its text; verify a CRLF document reports the CSS diagnostic on the correct line and column
- [x] 4.4 Report a document that ends inside a tag as `html/truncated` instead of converting a shorter document; verify a test asserts the diagnostic and its position, and that input ending between tokens still parses

## 5. Switch the engine and remove the old parser

- [x] 5.1 Point `ParseWithOptions` at the new pipeline and delete the hand-written tokenizer and tree builder; verify `go vet ./...` and `go test ./...` pass
- [x] 5.2 Verify `MaxOperations` keeps its unit: the budget tests that were written against token counting pass unchanged in meaning, and a document over the limit still produces `resource/limit`
- [x] 5.3 Run the fuzz target against the new frontend for at least 30 seconds and verify it finds no panic and no new corpus entry

## 6. Snapshots and corpora

- [x] 6.1 Regenerate the document snapshots and review the diff; verify every changed line is an implied element such as `tbody` and that `testdata/golden/render` and the page-text goldens are unchanged
- [x] 6.2 Re-measure the mPDF corpus, update its goldens only where the text difference is explained, and record any fixture that moves between `issue` and `snapshot` together with the reason; verify `go test ./internal/mpdfcompat` passes with `PDF_REQUIRE_VALIDATOR=1`
- [x] 6.3 Run the WeasyPrint corpus and verify page counts and the text-token comparison pass with the committed oracle unchanged

## 7. Integration and documentation

- [x] 7.1 Update `CONTRIBUTING.md`: the package map and the fuzz-target sentence no longer describe an in-house HTML tokenizer; verify the map matches the files under `internal/html`
- [x] 7.2 Benchmark `bigtable` and a generated one-megabyte table before and after the change; verify the recorded numbers show no regression beyond run-to-run noise, and reopen the token-pass decision if they do
- [x] 7.3 Run the full gate — `go vet ./...`, `go test ./...`, `go test -race ./...`, `PDF_REQUIRE_VALIDATOR=1` and `openspec validate --strict` — and verify every command passes

## Workflow follow-up

- Archive the change after the project's review requirements are satisfied.
- Verify the archive created `openspec/specs/html-parsing/spec.md` and that `code-quality` no longer forbids module requirements or `go.sum`.
