# Proposal

## Why

`implicitClose` searches the whole open-element stack for a tag to close, so a start tag inside a nested table or list closes an ancestor across the new boundary:

- `<table id="outer"><tr><td><table id="inner"><tr>…` — the inner `<tr>` closes the outer `<td>`, removes the inner table from the open stack, and attaches its row to the outer table;
- `<ol><li id="outer">A<ol><li id="inner">B` — the inner `<li>` closes the outer item and lifts the inner list out of it.

The text still appears in the output, so only the structure and geometry reveal the defect.

## What Changes

- Implicit closing SHALL be scoped: a cell, row or row group closes only within its own table; a list item only within its own list; `dt`/`dd` within their `dl`; `option` within its `select`.
- Nested tables and lists SHALL keep their parent relationships, and following text MUST stay in the element that contains it.

Out of scope: the remaining review findings.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: adds the structural requirement for nested tables and lists.

## Impact

- **Code**: `internal/html/parse.go` (`implicitClose`, a scoped close helper).
- **Behaviour**: malformed-but-common nesting now builds the tree a browser builds.
- **Risk**: a start tag that used to close across a boundary no longer does; tests pin both nested structures and the resulting geometry.
