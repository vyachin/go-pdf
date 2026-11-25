# Tasks

## 1. Bounded matcher

- [x] 1.1 Replace the `+` fallback in `Selector.Match` with segment matching: split at every adjacent combinator, match each descendant/child segment with the ancestor-chain dynamic program, then hop to the previous element — verify: the extended differential test and `go test ./internal/css/` are green; with segmentation disabled the deep adjacent test hangs.
- [x] 1.2 Extend `TestLongSelectorMatchesLikeTheNaiveMatcher` to selectors where `+`, `>` and the descendant combinator appear in every position, over siblings, ids, classes, `:root` and a pseudo-element — verify: every element's result equals the recursive matcher.
- [x] 1.3 Add a regression that counts compound comparisons and asserts a bound linear in compounds × depth — verify: the test fails when the recursive path is restored.
- [x] 1.4 Add a benchmark for the adjacent selector on a wide and a deep document — verify: `go test ./internal/css -bench Selector -benchmem` runs.

## 2. Charge the stages

- [x] 2.1 Give `Cascade` an optional `*budget.Guard`, charge `len(compounds)` per evaluated rule, and pass the engine's guard into it; browser import and docdump keep the nil guard — verify: a small budget stops matching, a nil guard matches every element unchanged.
- [x] 2.2 Give `imageLoader` the guard and context; charge decoded pixels before `png.Decode` — verify: a many-image document with `MaxOperations: 1000` returns `resource/limit` before decoding.
- [x] 2.3 Charge GPOS parsing per lookup, subtable and scanned coverage range — verify: the crafted amplifier font returns `resource/limit` under a small budget.
- [x] 2.4 Charge `insertOps` by operations copied and `Node.Path` by path segments — verify: a page of decorated fragments and an element with many classes each stop at the budget.
- [x] 2.5 Check `ctx` in the parse, match, decode and serialise loops at least every 512 units — verify: a cancelled context returns `context.Canceled` during each stage's test.

## 3. Tests and documentation

- [x] 3.1 Add one regression per charged stage (cascade, image, font, writer, path) that asserts the diagnostic code, not elapsed time — verify: each test fails when its charge is removed.
- [x] 3.2 Cover cancellation during matching at engine level — verify: a cancelled context with a rule-heavy document returns the context error, not a document diagnostic.
- [x] 3.3 Extend the `Limits.MaxOperations` doc comment and `docs/profile.md` to list every stage it covers — verify: `go doc` and the rendered doc both name the stages.
- [x] 3.4 Add an Unreleased/Fixed entry to `CHANGELOG.md` — verify: the entry matches the behaviour pinned by the tests.
