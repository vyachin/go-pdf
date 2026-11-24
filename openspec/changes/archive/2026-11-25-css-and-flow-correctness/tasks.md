# Tasks

## 1. Media rules at any nesting (CSS-01)

- [x] 1.1 Add one traversal that flattens `@media print` / `@media all` (including nested blocks) and feed the cascade with it — verification: `@media print { p { display: none } }` hides the paragraph
- [x] 1.2 Use the same traversal for `@page` resolution and `@font-face` collection — verification: `@page` and `@font-face` inside `@media print` take effect

## 2. Content-box origin for children (LAY-03)

- [x] 2.1 Pass an explicit x origin and content width from a parent to its children — verification: `padding-left: 20pt` moves text by 20 pt; nested padding accumulates
- [x] 2.2 Clamp a fixed child width to the available content width — verification: over-wide child equals the parent content width

## 3. Bottom insets in the flow (LAY-04)

- [x] 3.1 Advance the flow by bottom padding and border when a block completes — verification: padded `div` followed by a paragraph leaves at least 24 pt between baselines
- [x] 3.2 Keep blocks without bottom insets unchanged — verification: existing coordinate tests pass

## 4. Preformatted text (CSS-03)

- [x] 4.1 Emit line feeds and preserved spaces as separate items in `text.Split` for `white-space: pre` — verification: `A` LF `B` renders two lines without a glyph error
- [x] 4.2 Keep preserved space runs and treat tabs as spaces — verification: three spaces keep their width; CRLF produces one break

## 5. Text indent (CSS-04)

- [x] 5.1 Apply `line.Indent` in `paintLineAt` and stop resetting it in `anonymousStyle` — verification: `text-indent: 20pt` shifts only the first line; indent works inside an anonymous wrapper

## 6. Unitless line height (CSS-05)

- [x] 6.1 Keep a line-height factor on the style and resolve it against the element's own font size — verification: parent `10pt/2`, child `20pt` gives 40 pt between lines
- [x] 6.2 Keep absolute lengths absolute — verification: parent `line-height: 30pt` with a larger child font stays 30 pt

## 7. Acceptance

- [x] 7.1 Run `go vet ./...`, `go test ./...`, the mpDF corpus and `openspec validate css-and-flow-correctness` — verification: everything green
- [x] 7.2 Re-check the six reviewed reproductions by hand — verification: each produces the documented outcome
