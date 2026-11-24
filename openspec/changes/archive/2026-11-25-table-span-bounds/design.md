# Design

## Context

See proposal.md. `attrInt`:

```go
i, err := strconv.Atoi(v)
if err != nil || i < 1 { return def }
return i
```

`int` is 64-bit here, so a value above `math.MaxInt32` survives and later narrows to `0`.

## Goals / Non-Goals

**Goals:**

- No panic and no unbounded allocation from a span attribute.
- A clear diagnostic for an invalid span.

**Non-Goals:**

- Supporting huge tables; a table with hundreds of columns is outside the profile.
- Changing how normal spans lay out.

## Decisions

### 1. Bound at the boundary

`maxTableSpan = 100`. `spanAttr(n, name) (int, bool)` reads the attribute, converts with `strconv.Atoi`, and reports `false` when the value is not a positive integer or exceeds the bound. `buildTable` turns `false` into an error diagnostic `html/invalid-span` at the cell and uses `1` so layout can continue and report other problems.

*Why 100*: it bounds the occupancy grid to a small size while covering every realistic invoice or report; larger values are almost always malformed.

*Why error and not clamp*: a silently narrowed span would render a table that does not match the source and hide a broken template.

### 2. Clamp again in layout

Every use of `ColSpan`/`RowSpan` goes through a local `span`/`rowspan` normalisation (`< 1 → 1`, `> maxTableSpan → maxTableSpan`) before arithmetic or map growth. `rowHeight` divides only when `RowSpan > 1`.

*Why twice*: boxes can be constructed by code paths other than `buildTable` (tests, future builders); the layout must not depend on the parser having been careful.

## Risks / Trade-offs

- **A legitimate table wider than 100 columns** is rejected; the diagnostic names the limit, and the constant is one line to change.
- **Existing fixtures** with spans in range are unaffected; the corpus is the guard.
