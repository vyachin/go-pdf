# Proposal

## Why

`Options.RenderPNG` is debug functionality inside the semver-stable option
struct: it rasterizes only the first page at a fixed 96 dpi, and neither fact is
in its documentation. A `png.Encode` failure is swallowed, so a caller that
asked for a preview gets `Result.PNG == nil` and a successful conversion with no
diagnostic. The engine also always links the raster backend, which exists for
tests and previews, so a production binary carries a debug dependency it cannot
avoid. The field has to become an explicit, contained debug contract now, and
removal has to be scheduled as a breaking change.

## What Changes

- Document `Options.RenderPNG` as a debug-only preview with its exact contract:
  first page only, 96 dpi, bounded by `Limits.MaxRasterPixels`, and mark it
  deprecated in godoc (RV5-51).
- Report a PNG encoding failure as a `raster/render` error diagnostic instead of
  returning a silently empty preview (RV5-51).
- Keep the deprecation path explicit: the field stays for the rest of 2.x, and
  **BREAKING** its removal is reserved for the next major release, when a debug
  package or a preview entry point can replace it.
- Fix the stale engine godoc that claims a CLI debug flag uses the option.

No conversion behaviour, PDF bytes or existing diagnostic changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the debug preview option gains an explicit contract, a
  diagnosed encoding failure and a deprecation/removal rule.

## Impact

- `pdfgen/pdfgen.go` and `internal/engine/engine.go` (godoc and deprecation),
  `internal/engine/convert.go` (preview helper and error surfacing).
- `internal/engine/convert_test.go` and `pdfgen/pdfgen_test.go`: encode-failure
  test, byte-equality test, no-preview test and a source contract test.
- `CHANGELOG.md` under `Deprecated`.
- No rendering change; documents produced without the option are byte-identical.
