# Design

## Context

The guard is created in `engine.prepare` and passed to the parser, stylesheet
collection, layout and the writer. Image loading, font parsing, cascade matching and
`insertOps` run inside those stages without a guard reference. Selector matching has
two matchers: `matchFrom`, a recursive right-to-left walk, and `matchAncestors`, an
ancestor-chain dynamic program that is `O(compounds × depth)`. Dispatch sends every
selector that uses `+` to the recursive walk, which restores combinatorial cost.

## Goals / Non-Goals

**Goals:** every supported selector matches in bounded time; every unbounded stage has
a guard reference and a cancellation check; the charge is proportional to work; a
failure is reported once.

**Non-Goals:** changing the default `MaxOperations`; adding a second budget; changing
the accepted selector syntax or `MaxDOMDepth`.

## Decisions

- **D1. Segment the selector at `+`.** Split at every adjacent combinator into
  descendant/child segments, match the rightmost segment with the existing DP, then hop
  to `previousElement` and continue leftward. Cost is
  `O(compounds × depth + adjacent hops × siblings)`; alternatives (memoising
  `matchFrom`, a document-wide node-set DP, an abort-after-N-steps shortcut) either
  allocate on every call, add shared state, or silently change match results.
- **D2. Charge through a cascade-owned guard.** `Cascade` gains an optional
  `*budget.Guard`; `cascadeEntries` charges `len(compounds)` per evaluated rule and
  stops when the guard errors. Passing the guard to `Compute` would change every call
  site; a package-level callback would be mutable global state.
- **D3. Charge the remaining stages.** One unit per rule-element test, per
  lookup/subtable/coverage range scanned, per 64 KiB of decoded pixels and per operation
  copied by `insertOps`, through the same guard.
- **D4. Preserve results with a differential test.** Extend
  `TestLongSelectorMatchesLikeTheNaiveMatcher` to every position of `+`, `>` and the
  descendant combinator, over a document with siblings, ids, classes, `:root` and a
  pseudo-element.
- **D5. Pin cost with a step counter, not a clock.** Count compound comparisons in a
  test and assert a bound linear in `compounds × depth`.

## Risks / Trade-offs

- `previousElement` scans siblings on every hop → benchmark a wide document; cache the
  pointer on the node if the scan dominates.
- Per-rule charging overstates cheap rules → charge per compound and test that an
  unmatched-rule-heavy document stops with `resource/limit` instead of running.
- Charging the cascade makes rule-heavy documents fail earlier → document the counts in
  `Limits` and cover a realistic invoice.
- Double counting shrinks the effective budget → charge only the stage that performs the
  work and pin `Used()` for a small fixture.
