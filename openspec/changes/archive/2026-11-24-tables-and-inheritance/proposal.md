# Proposal

## Why

Three review findings about tables and inheritance, all confirmed with reproductions:

- `rowspan` cells are painted across rows, but the next row still starts at column 0, so a cell that should sit under the spanned column lands beneath the spanning cell.
- A second `thead` row is dropped on the first page: the skip logic that prevents re-painting repeated headers also fires before any repetition happened.
- Styles are computed from the wrong parent: `body` is computed with no parent, so `html` styles do not inherit; table cells are computed from the table node, skipping `thead`/`tbody`/`tfoot` and `tr`; the `html` element loses its attributes at parse time.

## What Changes

- Table layout SHALL place each cell in the grid column it actually occupies, taking cells that continue from previous rows into account.
- Both `thead` rows SHALL appear exactly once on the first page and be repeated exactly once on every continuation page.
- Style computation SHALL follow the real DOM chain: `html` → `body`, and `table` → row group → `tr` → `td`/`th`. Attributes on `html` MUST survive parsing.

Out of scope: the remaining review findings.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: adds a requirement for rowspan-aware cell placement.
- `pagination`: the repeated header requirement gains a scenario for a multi-row `thead`.
- `css-profile`: adds a requirement that inheritance follows the element chain.

## Impact

- **Code**: `internal/layout/table.go` (grid, header skip, painting), `internal/layout/box.go` (style chain, `html` style), `internal/html/parse.go` (attributes on `html`).
- **Output**: tables with `rowspan` place following cells correctly; documents with multiple `thead` rows keep both; inherited font and colour from `html`/`tr` now apply.
- **Risk**: column widths driven by the first row may interact with spanning; covered by tests for `rowspan`/`colspan` combinations.
