# document-layout Specification

## Purpose

Defines how the document tree becomes positioned boxes: block and inline layout, the box model, tables, lists and images. This is the layer that decides where each element lands on the page.

## Requirements

### Requirement: Building the box tree

The engine SHALL build a box tree from the document tree in which every block and line is given a type: block, inline, inline-block, list item, table row or table cell. Mixed content, where inline elements sit next to block elements, SHALL be wrapped in anonymous block boxes.

#### Scenario: Text and a block in one container
- **WHEN** a `div` contains text and a nested `p`
- **THEN** the text and the `p` land in separate anonymous block boxes instead of being mixed into one line

#### Scenario: A switched-off element
- **WHEN** an element has `display: none`
- **THEN** it generates no box and takes no space, including its descendants

### Requirement: The box model and collapsing margins

The engine SHALL compute width and height with the `content-box` model, where `width` excludes `padding` and `border`. The vertical margins of adjacent blocks SHALL collapse: the resulting margin equals the larger of the two. `box-sizing: border-box` SHALL be rejected as an unsupported value.

#### Scenario: Collapsing margins
- **WHEN** one block has `margin-bottom: 10mm` and the next has `margin-top: 6mm`
- **THEN** the distance between them is 10 mm

#### Scenario: Width with border and padding
- **WHEN** a block has `width: 100mm`, `padding: 5mm` and `border: 1mm`
- **THEN** its outer width is 112 mm

#### Scenario: Unsupported model
- **WHEN** the template contains `box-sizing: border-box`
- **THEN** validation fails with the unsupported value named

### Requirement: Inline layout and baseline alignment

The engine SHALL form line boxes and align inline elements on the baseline according to `vertical-align` (`baseline`, `top`, `middle`, `bottom`) and `line-height`. Replaced elements inside a line SHALL influence the line height according to their dimensions.

#### Scenario: An image in a line of text
- **WHEN** a line contains an image 20 mm high and some text
- **THEN** the line box is at least 20 mm high and the text is aligned on the baseline

#### Scenario: An explicit line height
- **WHEN** a paragraph has `line-height: 18pt` and `font-size: 12pt`
- **THEN** the baselines of adjacent lines are 18 pt apart

### Requirement: Tables with fixed layout

The engine SHALL support only `table-layout: fixed`, in which column widths are determined by the first row of cells and by `width` attributes. The value `table-layout: auto` SHALL be rejected. The engine SHALL support `colspan` and `rowspan` within a single page.

#### Scenario: Widths from the first row
- **WHEN** the first row of a table declares cells of 30 mm and 60 mm
- **THEN** every following row uses the same column widths

#### Scenario: Automatic layout
- **WHEN** the template contains `table-layout: auto`
- **THEN** validation fails, stating that only fixed layout is supported

#### Scenario: Spanning cells
- **WHEN** a cell is declared with `colspan="2"`
- **THEN** it occupies the combined width of two columns and the neighbouring cells shift

### Requirement: Table borders

The engine SHALL support `border-collapse: collapse` and `separate`. With collapsing, adjacent borders SHALL merge into a single line; with separation, each cell SHALL keep its own.

#### Scenario: Collapsed borders
- **WHEN** a table uses `border-collapse: collapse` and 1 pt borders
- **THEN** one 1 pt line is drawn between adjacent cells instead of two

#### Scenario: Separated borders
- **WHEN** a table uses `border-collapse: separate`
- **THEN** each cell keeps its own border

### Requirement: Lists

The engine SHALL render markers for `list-style-type: disc` and numbering for `decimal`, aligning the marker outside the list item's text block. Nested lists SHALL receive an additional indent, and the `start` attribute SHALL set the initial number.

#### Scenario: A numbered list with start
- **WHEN** an `ol` is declared with `start="5"` and contains two items
- **THEN** the items are numbered 5 and 6

#### Scenario: A nested list
- **WHEN** a list is nested inside an item of another list
- **THEN** its markers and text are indented to the right relative to the parent level

### Requirement: Images

The engine SHALL support PNG and JPEG images, determine their intrinsic dimensions and scale them to the given `width` and `height`, preserving the aspect ratio when only one dimension is given. An unsupported format SHALL produce an error naming the format and the path.

#### Scenario: Scaling by width
- **WHEN** a 200x100 px image is given with `width: 50mm`
- **THEN** the height is computed proportionally and is 25 mm

#### Scenario: Unsupported format
- **WHEN** the document references a GIF image
- **THEN** rendering fails with the format and the file path named

### Requirement: Collapsing whitespace

With `white-space: normal` the engine SHALL collapse sequences of spaces, tabs and newlines into a single space and remove spaces at the start and end of a line. With `white-space: pre` all spaces and newlines SHALL be preserved, and a line SHALL be clipped at the edge of the area rather than wrapped.

#### Scenario: Newlines in the source
- **WHEN** the source HTML contains text broken by newlines inside a paragraph
- **THEN** the output is one paragraph with single spaces

### Requirement: Rowspan shifts following cells

The layout SHALL place every table cell in the grid column it occupies. A cell in a following row MUST start after columns taken by cells that continue from earlier rows, so its horizontal position aligns with the cells of the same column above it.

#### Scenario: A cell under a spanning cell
- **WHEN** the first row has a `rowspan="2"` cell followed by another cell, and the second row has one cell
- **THEN** the second row's cell starts in the column of the first row's second cell
- **AND** it does not sit beneath the spanning cell

#### Scenario: Colspan combined with rowspan
- **WHEN** a spanning cell covers two columns and two rows
- **THEN** cells in the next two rows start after those two columns

#### Scenario: Table without spans
- **WHEN** a table has no `colspan` or `rowspan`
- **THEN** cells keep their sequential positions and widths
