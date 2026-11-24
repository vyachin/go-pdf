# Design

## Context

See proposal.md. Current code:

- `buildTable` computes a cell style from the table style, and `Build` computes `body` with `parent=nil`.
- `placeTable` skips header rows with `row.Header && cursor.row < headerCount && cursor.row > 0`, which also fires on the first page for the second header row.
- `paintRow`, `rowHeight` and `computeColumns` walk cells sequentially from column 0.

## Goals / Non-Goals

**Goals:**

- Cells land in their real grid column.
- Header rows appear exactly once per page.
- Inheritance follows `html` → `body` and `table` → group → `tr` → cell.

**Non-Goals:**

- Split rows across pages, `rowspan` across page boundaries, or table background painting on rows.
- Changing the fixed-layout algorithm: column widths still come from the first row.

## Decisions

### 1. Occupancy grid built before measuring

Before column widths and row heights are computed, the table builds a grid: for every row, each cell takes the first column not occupied by a cell continuing from an earlier row, marks its span for `rowspan` rows, and stores the column on the cell (`Box.col`). Measuring and painting then read `cell.col` instead of a running counter.

*Why a grid and not an offset*: `rowspan`/`colspan` combinations make the offset depend on previous rows; a single pass with an occupancy map expresses that directly and is easy to test.

*Why keep widths from the first row*: the profile supports fixed layout only, so the first row defines columns. A cell with `rowspan` in the first row still contributes its width.

### 2. Header repetition flag instead of an index condition

`placeTable` sets `repeated = true` only when it actually paints the repeated headers for a continuation page; the row loop skips header rows only under that flag. On the first page the flag is false, so every header row is placed normally.

*Why not compare `cursor.row` to `headerCount`*: that is exactly the condition that dropped the second header row.

### 3. Style chain through DOM ancestors

`Build` computes the `html` style first and passes it as the parent of `body`. `buildTable` walks `table` → `thead`/`tbody`/`tfoot` → `tr`, computing a style at each level and passing it to cells. Row groups and rows keep no boxes of their own; only the inherited values matter today.

`ensureHTML` copies the token attributes onto the `html` node, so `<html style="…">`, classes and ids reach the cascade.

*Why not build boxes for groups and rows*: their backgrounds and borders are not in the profile; only inheritance is observable, and adding boxes would change pagination without a requirement.

## Risks / Trade-offs

- **Grid cost**: O(rows × columns) extra work; tables are small and this is cheaper than the bugs.
- **`rowspan` in the first row** affects width distribution: the spanning cell width is split across its span, as before.
- **Style chain changes computed values** for documents that set `html` or `tr` styles; this is the intended correction.
- **`html` attributes** may include `class` used by selectors; the parser test pins that they survive.
