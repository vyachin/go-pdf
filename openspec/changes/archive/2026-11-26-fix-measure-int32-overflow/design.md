# Design

## Context

`document-layout` states that every layout length is computed in 64 bits and
stored in a 32-bit field only after a range check, and that a negative box
dimension never reaches the PDF. `paginate.go` follows that rule for the content
box and for `usedWidth`, and provides `satAdd`/`satSub`/`clampInt32` helpers. But
`measure` sums with plain `+`:

```go
own := clampInt32(...)                 // may be MaxInt32
if st.Height.Kind == css.MP && st.Height.Value > 0 {
    return own + st.Height.Value      // wraps to a negative height
}
var h int32
for _, ln := range b.lines {
    h += ln.Height                    // unguarded accumulation
}
```

The negative result is used for page-fit decisions and row heights, and it makes
`countFitting` and the row loop treat content as invisible. `paintLineAt`,
`decorationOps` and the row-paint sums have the same shape.

## Goals / Non-Goals

**Goals:**

- A measured height is never negative, and measuring the same box twice agrees.
- Every geometry addition and subtraction in measurement, placement and painting
  saturates instead of wrapping.
- A box that cannot fit reports `layout/overflow` once and keeps as much content
  as the page can hold.
- Normal documents produce byte-identical output.

**Non-Goals:**

- Supporting heights beyond `math.MaxInt32` millipoints (about 2,147,483 pt).
- Changing the box model, row-height algorithm or overflow thresholds.
- Fixing `computeColumns`, which belongs to `fix-table-layout-defects` (RV5-16)
  and touches the same file.

## Decisions

**Accumulate in `int64`, clamp once at the return.** Every branch of `measure`
computes its sum in `int64` and returns `clampInt32(sum)`. `clampInt32` already
maps an out-of-range value onto `[math.MinInt32, math.MaxInt32]`; the return path
additionally raises a negative result to zero, because a negative outer height is
never meaningful (the content box is already clamped to non-negative).

**Reuse the existing saturation helpers at every remaining `+`.** `satAdd`,
`satSub` and `satAddChecked` are already the project's rule for placement; using
them in `paintLineAt`, `decorationOps`, `paintRowAt` and `afterBlock` removes the
last plain arithmetic on the geometry path without a new abstraction.

**Diagnose once, keep the content that fits.** `measure` does not own the warning;
painting does. The huge line-height case is decided where the cell is painted:
`rowHeights` asks `measure`, and `paintStaticBlock` skips every line that starts at
or below the page limit (the branch currently has no diagnostic). That branch calls
the existing `warnOverflow` latch (`overflowWarned`), which is set the first time
the box exceeds the page and checked before every later warning, so one box
produces one `layout/overflow`.

**A clamped height is treated as "too tall", not as a valid height.** When the
`int64` sum exceeds `math.MaxInt32`, the measured height is `MaxInt32`, which is
larger than any page; the existing page-fit branch then moves or clips the content
and reports overflow. This preserves "no negative dimension" and keeps the
decision monotone.

## Risks / Trade-offs

- **A pathological box now clips instead of vanishing** → the visible outcome is
  one `layout/overflow` and partial content; that is the documented contract.
- **`satAdd` changes a value that was previously wrapping** → only inputs beyond
  the `int32` range differ; a test asserts ordinary boxes measure the same.
- **A single latch could hide a second, unrelated overflow of the same box** →
  accepted: the requirement is one diagnostic per box, and the message names the
  first cause.
- **Row heights built from a saturated height are enormous** → the page-fit
  decision treats them as "does not fit" and the content is clipped with the
  diagnostic, not painted off-page.

## Migration Plan

Internal arithmetic change: no public API, no stored data, no output change for
documents inside the `int32` geometry range. Release note under `Fixed`. The
`fix-table-layout-defects` change owns the table column sum and lands separately.
