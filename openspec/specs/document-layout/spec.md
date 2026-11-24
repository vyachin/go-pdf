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

### Requirement: Background images cover their box

The engine SHALL paint a box's background image before the box's content, scaled so that the image covers the box and centered, with the overflow divided equally between opposite edges. A box without a background image SHALL lay out as before.

#### Scenario: A photograph behind a heading
- **WHEN** a full-page cell has a photograph as its background and a heading as its content
- **THEN** the photograph covers the whole page and the heading is painted over it

#### Scenario: A cropped cover
- **WHEN** a 2:3 image covers an A4 box
- **THEN** the image is scaled to the box width and the vertical overflow is split equally between the top and the bottom

#### Scenario: Text without a background image
- **WHEN** the box declares only `background-color`
- **THEN** no image operation is added to the page

### Requirement: Background images are clipped to their box

The engine SHALL clip a background image to the box it belongs to, so that no pixel outside the box receives the image, and the clip SHALL NOT affect any other operation on the page.

#### Scenario: Overflow beyond the box
- **WHEN** the covered image is wider or taller than the box
- **THEN** the produced page has no image pixel outside the box edges

#### Scenario: Neighbouring content stays unclipped
- **WHEN** a page contains the covered box and other content
- **THEN** that other content is painted in full, unaffected by the background's clip

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

### Requirement: Table spans are bounded

`colspan` and `rowspan` SHALL be validated before layout uses them: a value MUST be an integer in `1..100`, and any other value MUST produce an error diagnostic naming the attribute and the cell. Layout MUST clamp every span it reads, so no span can reach integer conversion, division or grid allocation unchecked.

#### Scenario: Overflowing rowspan
- **WHEN** a cell declares `rowspan="4294967296"`
- **THEN** conversion fails with an `html/invalid-span` diagnostic
- **AND** no panic occurs

#### Scenario: Huge colspan
- **WHEN** a cell declares a `colspan` above the limit
- **THEN** conversion fails with a diagnostic naming the attribute

#### Scenario: Ordinary spans
- **WHEN** a table uses `rowspan="2"` and `colspan="3"`
- **THEN** layout places the cells as before

### Requirement: Children are placed inside the parent content box

A child block SHALL start at the content-box origin of its parent, that is after the parent's left border and padding, and MUST be measured within the parent's content width. The root block starts at the page content-left. A fixed child width MUST be clamped to the available content width.

#### Scenario: Padding on the parent
- **WHEN** a block declares `padding-left: 20pt` and contains a paragraph
- **THEN** the paragraph text starts 20 pt to the right of the page content-left

#### Scenario: Nested padding
- **WHEN** two nested blocks each declare 10 pt of left padding
- **THEN** the inner text starts 20 pt to the right of the page content-left

#### Scenario: Width inside a padded parent
- **WHEN** a child declares a width larger than the parent's content width
- **THEN** the child is clamped to that content width

### Requirement: Bottom padding and border advance the flow

After a block completes, the flow SHALL advance by its bottom padding and border in addition to its bottom margin, so the next block does not overlap them.

#### Scenario: Padded block followed by a paragraph
- **WHEN** a `div` with 10 pt padding is followed by a paragraph with no margin
- **THEN** the distance between their baselines is at least 24 pt

#### Scenario: Block without bottom insets
- **WHEN** a block has no padding, border and margin
- **THEN** the next block starts immediately after its content

### Requirement: Nested tables and lists keep their structure

Implicit end tags SHALL be scoped: a cell, row or row group closes only inside its own table; a list item only inside its own list; `dt`/`dd` only inside their `dl`; `option` only inside their `select`. Text that follows a nested container MUST stay in the element that contains it.

#### Scenario: Nested table
- **WHEN** a table cell contains another table with its own row
- **THEN** the inner row belongs to the inner table
- **AND** the inner table stays inside the outer cell

#### Scenario: Nested list
- **WHEN** a list item contains a nested list with an item
- **THEN** the inner item belongs to the inner list
- **AND** text after the nested list stays in the outer item

#### Scenario: Flat table
- **WHEN** a table has several cells and rows without nesting
- **THEN** the cells keep their positions and the layout is unchanged
