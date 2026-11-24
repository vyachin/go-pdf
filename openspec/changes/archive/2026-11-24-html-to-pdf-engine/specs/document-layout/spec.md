# Spec Delta

## Purpose

Defines how the document tree turns into positioned boxes: block and inline layout, the box model, tables, lists and images. This is the layer that decides where every element ends up on the page.

## ADDED Requirements

### Requirement: Box tree construction

The engine SHALL build a box tree from the document tree in which every block and line gets a type: block, inline, inline-block, list item, table row or table cell. Mixed content, where inline elements sit next to block elements, MUST be wrapped in anonymous block boxes.

#### Scenario: Text and a block in one container
- **WHEN** a `div` contains text and a nested `p`
- **THEN** the text and the `p` land in separate anonymous block boxes instead of mixing on one line

#### Scenario: Suppressed element
- **WHEN** an element has `display: none`
- **THEN** it produces no box and takes up no space, descendants included

### Requirement: Box model and collapsing of outer margins

The engine SHALL compute width and height by the `content-box` model, where `width` does not include `padding` and `border`. Vertical outer margins of adjacent blocks MUST collapse: the resulting margin equals the larger of the two. `box-sizing: border-box` MUST be rejected as an unsupported value.

#### Scenario: Margin collapsing
- **WHEN** one block has `margin-bottom: 10mm` and the next has `margin-top: 6mm`
- **THEN** the distance between them equals 10 mm

#### Scenario: Width with border and padding
- **WHEN** a block has `width: 100mm`, `padding: 5mm` and `border: 1mm`
- **THEN** its outer width equals 112 mm

#### Scenario: Unsupported model
- **WHEN** the template contains `box-sizing: border-box`
- **THEN** validation fails, naming the unsupported value

### Requirement: Inline layout and baseline alignment

The engine SHALL form line boxes and align inline elements along the baseline according to `vertical-align` (`baseline`, `top`, `middle`, `bottom`) and `line-height`. Replaced elements inside a line MUST affect the line height by their own dimensions.

#### Scenario: An image in a line of text
- **WHEN** a line contains an image 20 mm high and text
- **THEN** the line box height is at least 20 mm, and the text is aligned along the baseline

#### Scenario: Explicit line height
- **WHEN** a paragraph sets `line-height: 18pt` and `font-size: 12pt`
- **THEN** the baseline of adjacent lines is 18 pt apart

### Requirement: Tables with fixed layout

The engine SHALL support only `table-layout: fixed`, in which column widths are determined by the first row of cells and by `width` attributes. The value `table-layout: auto` MUST be rejected. The engine SHALL support `colspan` and `rowspan` within a single page.

#### Scenario: Widths from the first row
- **WHEN** the first row of a table sets cells of 30 mm and 60 mm
- **THEN** all subsequent rows use the same column widths

#### Scenario: Automatic layout
- **WHEN** the template contains `table-layout: auto`
- **THEN** validation fails, stating that only fixed layout is supported

#### Scenario: Merging cells
- **WHEN** a cell is declared with `colspan="2"`
- **THEN** it occupies the combined width of two columns, and the adjacent cells shift

### Requirement: Table borders

The engine SHALL support `border-collapse: collapse` and `separate`. When collapsing, adjacent borders MUST merge into one line; when separating, they MUST be kept on each cell.

#### Scenario: Collapsed borders
- **WHEN** a table uses `border-collapse: collapse` and 1 pt borders
- **THEN** one line 1 pt thick is drawn between adjacent cells instead of two

#### Scenario: Separate borders
- **WHEN** a table uses `border-collapse: separate`
- **THEN** each cell keeps its own border

### Requirement: Lists

The engine SHALL render markers for `list-style-type: disc` and numbering for `decimal`, aligning the marker outside the content box of the list item. Nested lists MUST get an additional indent, and the `start` attribute MUST set the initial number.

#### Scenario: A numbered list with start
- **WHEN** an `ol` is declared with `start="5"` and contains two items
- **THEN** the items get the numbers 5 and 6

#### Scenario: A nested list
- **WHEN** a list is nested inside an item of another list
- **THEN** its markers and text are shifted to the right relative to the parent level

### Requirement: Images

The engine SHALL support PNG and JPEG images, determine their intrinsic dimensions and scale them by the given `width` and `height` while preserving proportions when only one dimension is set. An unsupported format MUST lead to an error naming the format and the path.

#### Scenario: Scaling by width
- **WHEN** a 200x100 px image is set with `width: 50mm`
- **THEN** the height is computed proportionally and equals 25 mm

#### Scenario: Unsupported format
- **WHEN** the document references an image in GIF format
- **THEN** rendering fails, naming the format and the file path

### Requirement: Whitespace collapsing

With `white-space: normal` the engine SHALL collapse sequences of spaces, tabs and newlines into one space and remove spaces at the start and end of a line. With `white-space: pre` all spaces and newlines MUST be preserved, and the line MUST be clipped at the edge of the area without wrapping.

#### Scenario: Line breaks in the source
- **WHEN** the source HTML contains text broken by newlines inside a paragraph
- **THEN** the output is one paragraph with single spaces
