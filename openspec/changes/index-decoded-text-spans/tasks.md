# Tasks

## 1. Decode text tokens in the token pass

- [x] 1.1 In `internal/html/scan.go`, record the decoded text of a text token when its raw bytes contain `&`, a carriage return or NUL: call the tokenizer's text method, intern the decoded string, and store its id in the span's unused `name` field. Verify with a new scan test that `&amp;` and CRLF decode, that raw-text `<style>&amp;</style>` stays literal, and that `TestScanSpansPartitionInput` and `TestScanChargesOneUnitPerSpan` still pass.
- [x] 1.2 Add the inverse case to the scan test: a text token whose raw bytes contain no `&`, `\r` or NUL keeps a zero id and compares as its raw slice. Verify by asserting the recorded id on a plain text token and on a reference token in the same document.

## 2. Match nodes against decoded text

- [x] 2.1 In `internal/html/parse.go`, change the fast path of `textSpanFor` to compare the candidate span's decoded text with the node's data, falling back to the raw slice when there is no decoded id. Verify with `TestTextNodeWithCharacterReferenceKeepsItsOwnSpan` (`<p>a &amp; b</p>`) and `TestReferenceNextToLiteralAmpersand` (`<p>&amp;</p><p>&</p>`), both of which fail on the parent commit, and confirm `TestFosteredStyleKeepsItsOwnSpan`, `TestStyleTextIsRawSource` and `TestPositions` stay green.
- [x] 2.2 Replace the full-document scan in `textSpanFor` with a lazily built index of decoded text to every occurrence, and add the used marker and per-key cursor so an out-of-source-order node takes the first unused occurrence in document order. Verify `TestFosteredStyleKeepsItsOwnSpan`, `TestParagraphAndFosteredTextKeepTheirSpans` (`<p>A</p><table><style>S</style>A<tr><td>Y</td></tr></table>`), `TestTwoFosteredTextsKeepTheirSpans` (`<table><style>S</style>A<tr><td>Y</td></tr><div></div>A</table>`), and that `go test ./internal/html` reports work within a constant multiple of the token count for 1,000 repeated tables.
- [x] 2.3 Handle a dropped leading newline: in the fast path for a `pre`/`listing`/`textarea` parent, match the decoded text with its first newline removed and start the range after the raw bytes that produced it (LF, CRLF, or a reference such as `&#10;`); add the same variant to the index for out-of-source-order nodes. Verify with `<pre>\nX</pre>`, `<textarea>\r\nX</textarea>`, `<pre>&#10;X</pre>`, and a foster-parented `pre` whose text is looked up through the index; each range must cover exactly `X`.
- [x] 2.4 Handle a NUL: when the decoded text contains NUL, match it with each NUL replaced by U+FFFD and with each NUL removed, and keep the token's whole range; add both variants to the index. Verify `<p>a\x00b</p>` (removed, data `ab`), `<svg>a\x00b</svg>` (replaced, data `a` U+FFFD `b`) and `<table>a\x00b<tr><td>x</td></tr></table>` (removed, fostered data `ab`), each range covering the token including the NUL.
- [x] 2.5 Thread the shared guard into the builder with a sticky failure, charge one unit per text span indexed and one per out-of-source-order lookup, and stop the walk when the guard fails. Verify that a fostered document with a budget below its matching cost returns an error wrapping `budget.ErrExceeded`, that a cancelled context returns `context.Canceled`, and that `go test ./internal/html ./internal/engine -run 'Budget|AdversarialSpend'` passes.
- [x] 2.6 Add `BenchmarkParseEntityFragments` with 16,000 and 32,000 `<p>&amp;</p>` fragments. Verify with `go test ./internal/html -run '^$' -bench ParseEntityFragments` that both sizes report time and allocations per operation (the benchmark reports scaling; it must not assert a threshold).
- [x] 2.7 Update the HTML parsing row of the work-budget table in `docs/profile.md` to state the new unit (one text span indexed or looked up, including its context variants). Verify the row matches the charge implemented in 2.5.

## 3. Adversarial corpus

- [x] 3.1 Add `testdata/adversarial/p1-entity-fragments.html` (repeated `<p>&amp;</p>`) and `testdata/adversarial/p1-fostered-texts.html` (repeated table with unique `<style>` content and unique fostered text), and register both in `internal/engine/adversarial_test.go` with fixed budgets: the first must convert, the second must report `resource/limit`. Verify with `go test ./internal/engine -run 'TestAdversarialManifest|TestAdversarialCorpus|TestAdversarialSpendIsProportional'`; the parser charge itself is pinned by `TestPositionMatchingIsCharged`, which fails on the parent commit.
- [x] 3.2 Verify the corpus in the required gate: `go test -race ./internal/engine -run TestAdversarial` passes and the manifest test names both fixtures.

## 4. Release note

- [x] 4.1 Add an `Unreleased` entry to `CHANGELOG.md` under `### Fixed` stating that a text node with a character reference, a trimmed leading newline or a NUL keeps its own source position, that equal out-of-source-order texts keep distinct positions, and that text position matching is indexed and charged. Verify the entry sits under the correct heading and names no removed API.

## Workflow follow-up

- Run `openspec validate index-decoded-text-spans --json` and fix any reported issue.
- Sync the delta into `openspec/specs/html-parsing/spec.md` and archive the change after the project's review requirements are satisfied.
