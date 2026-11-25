# Tasks

## 1. Decode text tokens in the token pass

- [ ] 1.1 In `internal/html/scan.go`, record the decoded text of a text token when its raw bytes contain `&`, a carriage return or NUL: call the tokenizer's text method, intern the decoded string, and store its id in the span's unused `name` field. Verify with a new scan test that `&amp;` and CRLF decode, that raw-text `<style>&amp;</style>` stays literal, and that `TestScanSpansPartitionInput` and `TestScanChargesOneUnitPerSpan` still pass.
- [ ] 1.2 Add the inverse case to the scan test: a text token whose raw bytes contain no `&`, `\r` or NUL keeps a zero id and compares as its raw slice. Verify by asserting the recorded id on a plain text token and on a reference token in the same document.

## 2. Match nodes against decoded text

- [ ] 2.1 In `internal/html/parse.go`, change the fast path of `textSpanFor` to compare the candidate span's decoded text with the node's data, falling back to the raw slice when there is no decoded id. Verify with `TestTextNodeWithCharacterReferenceKeepsItsOwnSpan` (`<p>a &amp; b</p>`) and `TestReferenceNextToLiteralAmpersand` (`<p>&amp;</p><p>&</p>`), both of which fail on the parent commit, and confirm `TestFosteredStyleKeepsItsOwnSpan`, `TestStyleTextIsRawSource` and `TestPositions` stay green.
- [ ] 2.2 Replace the full-document scan in `textSpanFor` with a lazily built `map[string]int32` from decoded text to the first text span, built once over the recorded spans and used only after a fast-path miss. Verify `TestFosteredStyleKeepsItsOwnSpan`, a new test with repeated tables whose fostered text is unique, and a unit test that the index returns the first occurrence and keeps distinct texts apart.
- [ ] 2.3 Thread the shared guard into the builder with a sticky failure, charge one unit per text span indexed and one per out-of-order lookup, and stop the walk when the guard fails. Verify that a fostered document with a budget below its matching cost returns an error wrapping `budget.ErrExceeded`, that a cancelled context returns `context.Canceled`, and that `go test ./internal/html ./internal/engine -run 'Budget|AdversarialSpend'` passes.
- [ ] 2.4 Add `BenchmarkParseEntityFragments` with 16,000 and 32,000 `<p>&amp;</p>` fragments. Verify with `go test ./internal/html -run '^$' -bench ParseEntityFragments` that both sizes report time and allocations per operation (the benchmark reports scaling; it must not assert a threshold).
- [ ] 2.5 Update the HTML parsing row of the work-budget table in `docs/profile.md` to state the new unit (one text span indexed or looked up), and add the trimmed-token range (`<pre>`/`<textarea>` leading newline, removed NUL) to the known gaps. Verify the table and the gap text match the charge implemented in 2.3.

## 3. Adversarial corpus

- [ ] 3.1 Add `testdata/adversarial/p1-entity-fragments.html` (repeated `<p>&amp;</p>`) and `testdata/adversarial/p1-fostered-texts.html` (repeated table with unique `<style>` content and unique fostered text), and register both in `internal/engine/adversarial_test.go` with fixed budgets: the first must convert, the second must report `resource/limit`. Verify with `go test ./internal/engine -run 'TestAdversarialManifest|TestAdversarialCorpus|TestAdversarialSpendIsProportional'`, and record the parent-commit result of the fostered fixture (it must not report the limit there).
- [ ] 3.2 Verify the corpus in the required gate: `go test -race ./internal/engine -run TestAdversarial` passes and the manifest test names both fixtures.

## 4. Release note

- [ ] 4.1 Add an `Unreleased` entry to `CHANGELOG.md` under `### Fixed` stating that a text node with a character reference keeps its own source position and that text position matching is indexed and charged. Verify the entry sits under the correct heading and names no removed API.

## Workflow follow-up

- Run `openspec validate index-decoded-text-spans --json` and fix any reported issue.
- Sync the delta into `openspec/specs/html-parsing/spec.md` and archive the change after the project's review requirements are satisfied.
