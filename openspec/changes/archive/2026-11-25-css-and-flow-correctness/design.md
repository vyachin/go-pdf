# Design

## Context

See proposal.md. Current behaviour:

- `NewCascade`, `ParsePageStyle` and `newFontRegistry` iterate `sheet.Rules` at the top level only.
- `placeBlock` sets `b.x = f.left` for every block, ignoring the parent content box.
- `afterBlock` advances the flow by the sibling margin only; `finishFragment` adds bottom padding and border to the painted rectangle.
- `text.Split(s, pre)` returns one `Word` item holding the whole string.
- `paintLineAt` ignores `line.Indent`; `anonymousStyle` zeroes `TextIndent`.
- `line-height: <number>` becomes `EmLen(n)` and is resolved on the parent.

## Decisions

### 1. One flattening traversal

`css.RulesForPrint(sheets []*Stylesheet) []Rule` walks each sheet, descends into `MediaRule` whose query is empty, `all` or `print` (case-insensitive, comma lists split), and returns the remaining rules in document order. Cascade, `ParsePageStyle` and the font registry consume that slice.

*Why a shared function*: three consumers already drift from each other; a single traversal keeps media semantics identical.

*Nested media*: recursion is the same rule applied again, so `@media print { @media print { … } }` works by construction.

### 2. Content-box origin for children

`placeBlock` gains an explicit `x` origin. The root starts at the page content left; a parent passes `b.x + borderLeft + padLeft` and its content width to each child. `b.x` is no longer read from the flow.

*Why not store the origin on the flow*: siblings have different origins; a parameter is the simple, obvious carrier.

*Horizontal margins*: not part of the supported profile (the UA sheet uses padding for lists), so they stay out of scope; the requirement says "supported horizontal insets".

### 3. Bottom insets belong to the flow

`afterBlock` adds `padBottom() + borderBottom()` to `f.y` before recording the sibling margin. A block that overflows the page keeps its existing clipped behaviour.

*Why in `afterBlock`*: it runs exactly once per completed block, which is where the flow should consume the insets; `finishFragment` only draws.

### 4. Preformatted text before shaping

`text.Split(s, pre)` gains `Newline` and preserved `Space` items: `\n` and `\r\n` become `Newline`, tabs become spaces, space runs are emitted one item per space. Line building maps `Newline` to a forced break and keeps preserved spaces without collapsing or dropping them at line start.

### 5. Indent at paint time

`paintLineAt` adds `line.Indent` to its starting `x`; the available width already excludes it, so justification and alignment stay correct. `anonymousStyle` stops resetting `TextIndent` (the property inherits and the anonymous box has no separate value).

### 6. Unitless line height as a factor

`Style` gains `LineHeightFactor`. A number in `line-height` sets the factor and leaves `LineHeight` auto; `LineHeightMP` multiplies the factor by the element's own font size. Percentages and lengths keep their current absolute behaviour.

*Why not keep `EmLen`*: `em` is resolved on the declaring element, while a unitless factor must follow the inheriting element's font size.

## Risks / Trade-offs

- **Placement change touches every block**: covered by the full corpus, page-count checks and the new position tests.
- **Media flattening changes which rules apply** to documents that nested them; that is the correction.
- **Preformatted spaces** may increase line width; shaping already handles the glyphs.
- **Line-height factor** changes inheritance; absolute lengths keep their semantics and a test pins both.
