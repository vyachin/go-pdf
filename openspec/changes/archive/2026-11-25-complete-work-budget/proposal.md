# Proposal

## Why

`Limits.MaxOperations` is documented as bounding "the total work of a conversion", but
the shared guard is only charged for tokens, layout boxes, placed lines and writing
operations. Selector matching is the most expensive uncovered stage: `Selector.Match`
sends every selector with three or more compounds and an adjacent-sibling `+` to the
recursive matcher, which re-walks the ancestor chain at every descendant step, so a
2.3 KB document runs for 92 seconds and a 420-byte one for seconds at
`MaxOperations: 2000`. PNG decoding, GPOS parsing, the per-fragment `insertOps` copies
and `Node.Path` construction are equally uncounted: a crafted 131 KB font spends 25
seconds in `parseGPOS`, and 10k rules × 10k elements take 3.2 seconds. None of these
stages observes context cancellation, so the README guarantee that a document over a
limit fails before the expensive work does not hold for them.

## What Changes

- A supported selector SHALL be matched in time bounded by its compound count and the
  tree depth, including selectors that use `+`: the selector is split at each adjacent
  combinator, each segment is matched right-to-left by the ancestor-chain dynamic
  program, and `+` is a deterministic hop to the previous element.
- Every stage whose cost grows with the document SHALL charge the shared work guard and
  observe context cancellation: cascade and selector matching, font parsing, image
  decoding, text shaping, layout and PDF serialisation.
- An exhausted budget MUST surface as a `resource/limit` diagnostic from whichever
  stage hit it, and a cancelled context MUST win over a document diagnostic.
- A regression test SHALL pin the bounded matcher cost with a step counter rather than a
  wall-clock threshold, as `code-quality` requires.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: matching a supported selector is bounded and accounted in the
  operation budget; cancellation during matching is honoured.
- `library-api`: the work budget covers every stage, not only tokenising, layout and
  writing; cancellation is observed inside those stages.

## Impact

- **Code**: `internal/css/selector.go` (segmented matcher, removal of the
  `hasAdjacent` fallback), `internal/css/cascade.go` (guard plumbing),
  `internal/engine/images.go`, `internal/font/gpos.go`, `internal/font/font.go`,
  `internal/pdf/writer.go`, `internal/html/node.go`, `internal/budget`.
- **Output**: documents that previously took minutes now fail with `resource/limit`
  under the default budget; within-budget output is unchanged.
- **Tests**: the extended differential matcher test, one regression per charged stage
  under a small `MaxOperations`, a cancellation test, and benchmarks; no elapsed-time
  assertions (see `code-quality`).
