# Proposal

## Why

`text.AdvanceWidth` sums glyph advances in `int32` without saturation. A 350,000
character word at 12 pt wraps to a negative run width, the layout line's natural
width becomes negative, and the `layout/overflow` warning disappears: 1,000
characters still warn, 350,000 characters are clipped silently. `text.Scale`
narrows a float64 product to `int32` without a range check, so an extreme font
size produces an implementation-defined value that differs between architectures
and can flip the sign of an advance or a metric. Both break the "wrong output is
never silent" and "identical bytes from identical inputs" contracts.

## What Changes

- Accumulate run and line widths in `int64` and clamp explicitly to `MaxInt32`,
  so a width is never negative and the clamp is observable.
- Treat saturation as an overflow: report `layout/overflow` at the owning element
  once, the same diagnostic a word wider than the content box already produces.
  The warning MUST NOT disappear because an `int32` wrapped.
- Compute `text.Scale` and the face metrics in `float64`/`int64`, then clamp to
  `[-MaxInt32, MaxInt32]` explicitly; an extreme `font-size` that requires
  clamping is reported instead of silently producing architecture-dependent
  geometry.
- Keep documents that do not reach the range byte-identical, and verify the
  result on `GOARCH=386` as well as on 64-bit hosts.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: run widths saturate and report overflow; scaling range-checks
  before narrowing so geometry is architecture-independent.

## Impact

- `internal/text/shape.go` (`AdvanceWidth`, `Scale`, `MetricsOf`),
  `internal/layout/lines.go` and `paint.go` (line width accumulation and the
  overflow check), `internal/layout/paginate.go` (the existing notice path).
- Tests in `internal/text`, `internal/layout` and `internal/engine`; a 32-bit
  build check in CI.
- No exported Go API change and no change to valid, in-range documents.
