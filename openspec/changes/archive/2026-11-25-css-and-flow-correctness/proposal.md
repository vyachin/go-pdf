# Proposal

## Why

Six confirmed defects that make supported CSS behave differently from the specification:

- rules nested in an applicable `@media` block are parsed and validated but silently dropped by the cascade, page style and font registry;
- every block starts at the page margin, so horizontal padding and border of a parent do not reach its children;
- bottom padding and border are painted but do not advance the flow, so the next block overlaps them;
- `white-space: pre` turns the whole text into one word, so a line feed reaches glyph lookup and valid text is rejected;
- `text-indent` is accepted and measured but never applied to the first line, and the anonymous block wrapper resets it;
- a unitless `line-height` is converted to absolute millipoints on the parent and inherited as a fixed size.

## What Changes

- A single traversal SHALL expose rules that apply to print, descending into `@media` blocks whose query matches `print` or `all`, and the cascade, `@page` resolution and `@font-face` collection MUST all use it.
- Child blocks SHALL be placed at the parent's content-box origin and measured inside its content width.
- A completed block SHALL advance the flow by its bottom padding and border in addition to its margin.
- `white-space: pre` SHALL keep line feeds as line breaks and preserve space runs before shaping.
- `text-indent` SHALL shift the first line of its block and MUST NOT be reset by an anonymous wrapper.
- A unitless `line-height` SHALL be inherited as a factor and resolved against the element's own font size.

Out of scope: the remaining review findings (budgets, cancellation, public API, importer cleanup).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: media nesting, `white-space: pre`, `text-indent`, unitless `line-height`.
- `document-layout`: content-box placement of children and bottom padding/border in the flow.

## Impact

- **Code**: `internal/css/parse.go`, `internal/css/cascade.go`, `internal/css/style.go`, `internal/css/props.go`, `internal/layout/{paginate,paint,lines,box}.go`, `internal/engine/fonts.go`, `internal/text/normalize.go`.
- **Output**: documents using the six features render differently, which is the correction; documents without them are unchanged.
- **Risk**: the origin change touches every block, so page-count and corpus tests are the guard; new tests pin each reviewed reproduction.
