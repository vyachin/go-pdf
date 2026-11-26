# Proposal

## Why

`measure` computes a box's outer height in `int32` with plain `+`. Its padding and
border term is already clamped, and a declared `height` may be as large as
2,000,000 pt, so `own + Height.Value` wraps: a `padding:2000000pt;
height:2000000pt` box measures `-147483649`. The negative value then flows into
row heights and page-fit decisions, so content disappears. Verified: a cell with
`line-height:2000000pt` containing `A<br>B` paints only "A", drops "B" and paints
the following block above the cell, with no diagnostic.

`document-layout` already requires every layout length to be computed in 64 bits
and saturated before storage, and requires a page margin larger than the page not to
produce wrapped coordinates. The measurement and painting paths were never
converted; this change closes that gap.

## What Changes

- Compute every sum in `measure` in 64-bit arithmetic and saturate the result
  before storing it, so a measured height is never negative.
- Convert the remaining plain additions in placement and painting:
  `paintLineAt`, `decorationOps`, `paintRowAt` and `afterBlock`.
- Report `layout/overflow` exactly once for a box whose geometry exceeds the page,
  instead of dropping its content silently.
- Add regression tests with huge padding, huge line-height and huge margins; the
  table column sum (`computeColumns`) is owned by `fix-table-layout-defects`
  (RV5-16), so the two changes do not edit the same lines.

Not included: raising the representable geometry range beyond the `int32`
millipoint bounds; oversized values are clamped, not supported.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: the geometry-saturation requirement is extended to
  measurement, line painting and fragment decoration, with a no-negative-height
  and one-diagnostic outcome.

## Impact

- `internal/layout/paginate.go` (`measure`, `afterBlock`),
  `internal/layout/paint.go` (`paintLineAt`, `decorationOps`),
  `internal/layout/table.go` (`paintRowAt` height sums).
- `internal/layout/pagination_test.go`: new regression for a huge line-height
  cell; existing overflow tests cover the page-facing outcome.
- No public API, diagnostic-code, profile or output-format change. Oversized
  documents that already warn now warn once instead of losing content.
- Render goldens are unchanged for documents whose geometry fits `int32`.
