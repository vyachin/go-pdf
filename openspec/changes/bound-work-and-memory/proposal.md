# Proposal

## Why

The work budget counts tokens, boxes and operations, but not the bytes each stage
materialises, so a document that is cheap in units can allocate hundreds of megabytes
before the guard notices. Measured through the public API with the default limits:
7 MiB of CSS made of newlines allocates 322 MiB total / 74 MiB heap and reports no
diagnostic, while the same size of `x` allocates 22 MiB, because `lineStarts` stores
one `int` per newline and nothing charges it. 60 unique 1 MiB stylesheets convert
successfully with 73 MiB heap because linked stylesheets have a per-file limit but no
aggregate one. 8 MiB of plain text gives 1.3 GiB total allocation / 401 MiB heap with
`err == nil`, because shaping charges one unit per rune and line painting then builds a
second glyph slice. `MaxOperations` is documented to bound memory and does not.

## What Changes

- Make the cost model byte-aware: charge before a line index, decoded text or glyph
  copy is allocated, and pin the maximum bytes one unit may buy in
  `TestAdversarialSpendIsProportional`.
- Bound the line index: cap the stored starts or build them lazily, charge each stored
  entry, and return `resource/limit` for a stylesheet made of newlines instead of
  allocating hundreds of MiB.
- Add `Limits.MaxTotalStylesheetBytes` (default 16 MiB) beside the existing aggregate
  image and font limits, and charge linked stylesheet reads by size through the asset
  registry.
- Stop copying the shaped glyph slice when no glyph carries an offset, allocate an
  exact-size slice when a copy is required, and charge one unit per copied glyph.
- Extend the adversarial fixtures and the cost-model tests; document the new limit in
  `docs/profile.md` and in `pdfgen.Limits`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: the cost-model requirement gains a byte-proportional rule and its
  invariant test.
- `document-layout`: text placement gains a glyph-reuse and per-glyph charge rule.
- `library-api`: the bounded-work guarantee gains a bounded line index and an aggregate
  stylesheet limit in `Limits`.

## Impact

- `internal/css/parse.go`, `internal/html/parse.go` (`lineStarts`),
  `internal/engine/engine.go`, `internal/engine/convert.go`,
  `internal/profile/styles.go`, `internal/assets/assets.go`,
  `internal/layout/paint.go`, `pdfgen/pdfgen.go`.
- Public API: additive field `Limits.MaxTotalStylesheetBytes`; no code or output change
  for documents inside the new default.
- `pdfgen.DefaultLimits`, `docs/profile.md` limits table, `CHANGELOG.md`.
- Tests: `internal/engine/budget_stages_test.go`, the adversarial corpus, layout and
  stylesheet regression tests.
