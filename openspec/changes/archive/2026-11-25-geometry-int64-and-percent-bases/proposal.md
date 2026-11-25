# Proposal

## Why

Layout geometry is int32 in millipoints and several sums are computed without a range
check. A legal 3,000,000×2 PNG (47 KB, 6 MP) makes `int32(cfg.Width) * 750` wrap to
−2,044,967 points and the PDF receives a negative `cm` width; two 2,000,000 pt
paddings paint text at X = −294,967,296 with no diagnostic. Percentages are resolved
against the wrong base: `width: 50%; margin-left: 10%` yields 45 %, `padding: 0 10%`
and `margin-bottom: 10%` use the box's own width, and `text-indent: 10%` uses the
content width. `@page { margin: 1em }` becomes 595 pt on A4, `auto`, `%` and negative
margins are accepted silently, `size: portrait` does nothing when the fallback is
landscape, and `width: 0` is treated as `auto`.

## What Changes

- All geometry SHALL be computed in 64-bit and converted to int32 only after a range
  check; an unrepresentable value MUST be clamped or rejected, never wrap.
- Percentages for `width`, margins, padding and `text-indent` SHALL resolve against
  the containing block width.
- `@page` margins SHALL be non-negative lengths; `em` resolves against the initial
  font size and `auto`, `%` and negative values are rejected.
- `size: portrait` MUST swap a landscape fallback.
- `width: 0` SHALL mean zero.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: percentage resolution uses the containing block for every length
  property.
- `document-layout`: geometry does not overflow and `width: 0` is honoured.
- `pagination`: page margins are validated and portrait flips a landscape fallback.

## Impact

- **Code**: `internal/engine/images.go`, `internal/layout/paginate.go`,
  `internal/layout/background.go`, `internal/layout/lines.go`,
  `internal/layout/margin.go`, `internal/css/page.go`.
- **Output**: extreme but legal documents stop corrupting the page; percentage-based
  layouts shift to the values CSS specifies.
- **Tests**: the wide PNG, the two-padding overflow, the four percentage cases, the
  `@page` margin cases and the portrait fallback become regressions.
