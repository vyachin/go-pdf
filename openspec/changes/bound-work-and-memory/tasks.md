# Tasks

## 0. Behavioral invariant tests before implementation

- [x] 0.1 Add stage-local work counters for line-index growth, decoded-text
  materialization, asset reads and offset-bearing glyph copies. Exercise each
  stage with enough budget and with one unit less than its documented next
  charge. Verify the refused read, expansion or copy never starts, completed
  work grows with the admitted units, and exhaustion stops dependent stages.
  Use instance-local test instrumentation and assert stage deltas rather than
  the conversion's incidental total operation count.
- [x] 0.2 Extend the proportional-spend matrix with increasing hostile input
  sizes at fixed budgets and increasing budgets for the same input. Verify
  admitted work and allocated bytes stay within documented per-stage bounds
  plus a stated fixed overhead, and report the offending stage. Exercise
  `Validate` and `Convert` with shared preparation/layout fixtures, separating
  the writer-only cost. Use counters and allocation bounds for pass/fail;
  elapsed timings remain benchmark/report data only.

## 1. Byte-aware charging and the line index

- [x] 1.1 Add a documented `bytesPerUnit` constant and a `chargeBytes(n int)` helper next to the guard, and use it in the CSS and HTML parsers before the line index grows. Verify with a unit test that `chargeBytes(4096)` costs two units (one plus one) and that a nil guard is a no-op.
- [x] 1.2 Cap `lineStarts` at 65,536 starts, charge each stored entry, and fall back to a charged scan from the last stored start for offsets past the cap. Verify with a test that a 7 MiB newline stylesheet returns `resource/limit` and a position on the 70,000th line is still correct.
- [x] 1.3 Extend `TestAdversarialSpendIsProportional` with the 8 MiB text fixture and assert allocated bytes per unit for line indexing, decoded text and glyph copies; name the failing stage in the message. Verify it fails on the parent commit and passes here.

## 2. Aggregate stylesheet limit

- [x] 2.1 Add `MaxTotalStylesheetBytes` (default 16 MiB) to `engine.Limits`, `pdfgen.Limits`, both mapping functions, `resolved()`, `DefaultLimits()` and the mirror test. Verify that a zero value resolves to 16777216 and that the field round-trips through both packages.
- [x] 2.2 Pass the total as `assets.Request.MaxTotalBytes` when a linked stylesheet is read, and charge `ceil(size/4096)+1` in `Registry.Read` before `readBytes`. Verify with 60 unique 1 MiB stylesheets: the call now returns `resource/limit` before the seventeenth read, and a single 8 MiB sheet still converts.
- [x] 2.3 Document the field in the `pdfgen.Limits` godoc and in the limits table of `docs/profile.md`, and add the `TestDocumentedLimitsMatchTheCode` entry. Verify the documentation test fails when the default changes.

## 3. Layout glyph reuse and fixtures

- [x] 3.1 In `paintShapedRun`, paint `run.Glyphs` directly when no glyph has a non-zero offset, and otherwise copy into an exact-size slice charged per glyph before `make`. Verify with a benchmark that the 8 MiB single-word profile no longer allocates a second slice, and that the text and render goldens are unchanged.
- [x] 3.2 Add an adversarial fixture with a 7 MiB newline stylesheet and another with 60 aggregate-limit stylesheets, register both in `adversarialCases()` with `Want: "resource/limit"`, and run `TestAdversarial` plus `TestAdversarialManifest`. Verify the fixtures fail on the parent commit.
- [x] 3.3 Update `CHANGELOG.md` under `Unreleased / Changed` with the byte-aware budget, the bounded line index and the new limit; state that ordinary documents keep identical output. Verify no removed or renamed API appears in the entry.
