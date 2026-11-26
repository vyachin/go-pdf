# Proposal

## Why

`pdfgen` cannot export the internal types, so `Options`, `Limits` and
`Diagnostic` are mirrored by hand and converted by `toEngine` (on both
`Options` and `Limits`), `fromEngineLimits` and `fromDiags`. A field added on one side is
silently dropped: `toEngine` compiles without it, `fromEngineLimits` leaves it
zero, and the existing default test names every field explicitly, so it only
checks what someone remembered to list. The drift is already visible in godoc:
`engine.Limits.MaxTableCells` says "the number of cells in one table" while the
code and the public field bound the grid area (columns times rows), and
`engine.Options.RenderPNG` still names a CLI debug flag that does not exist.

## What Changes

- Add a field-set test that compares each public type with its internal
  counterpart by name, type and count, so adding or dropping a field on either
  side fails (RV5-52).
- Add a round-trip test that sets every public field and proves it reaches the
  internal value, and every internal field and proves the public result carries
  it; replace the hand-written default comparison with a reflective one.
- Correct the mirrored godoc: `MaxTableCells` describes the grid area and
  `RenderPNG` describes the debug preview without the non-existent CLI flag; a
  source-level test pins the statements (RV5-52).

No behaviour, output or diagnostic changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-quality`: the public option surface becomes a checked mirror of the
  internal one, with a test that fails on a field added to only one side.

## Impact

- `pdfgen/pdfgen.go` and `internal/engine/engine.go` (godoc strings only).
- New tests in `pdfgen` (reflection, round trip) and an extension of the
  documentation test that already compares limits.
- `pdfgen/pdfgen_test.go::TestDefaultLimitsMatchesConversion` is replaced by a
  reflective assertion with the same behaviour.
- No exported field, type, default, diagnostic or byte of output changes.
