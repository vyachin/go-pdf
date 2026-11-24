# Spec Delta

## ADDED Requirements

### Requirement: Media rules apply at any nesting

Rules inside `@media` blocks whose query matches `print` or `all` SHALL apply to the document, at any nesting depth, preserving document order. The same traversal MUST feed the cascade, `@page` resolution and `@font-face` collection; a `@media` with any other query MUST apply nothing.

#### Scenario: Nested print block
- **WHEN** the stylesheet contains `@media print { p { display: none } }`
- **THEN** the paragraph is not rendered

#### Scenario: Nested all block
- **WHEN** the stylesheet contains `@media all { @media print { h1 { color: red } } }`
- **THEN** the `h1` colour is red

#### Scenario: Unsupported media query
- **WHEN** the stylesheet contains `@media screen { p { display: none } }`
- **THEN** validation rejects the query as unsupported

#### Scenario: Page and font rules inside media
- **WHEN** `@page` or `@font-face` is declared inside `@media print`
- **THEN** the page size and the font take effect

### Requirement: White space in preformatted text

With `white-space: pre`, a line feed (LF or CRLF) SHALL start a new line, sequences of spaces MUST be preserved, and no character used for formatting may reach glyph lookup. A tab MUST be treated as a space.

#### Scenario: Two lines
- **WHEN** preformatted text contains `A` and `B` separated by a line feed
- **THEN** two lines are rendered
- **AND** no `text/glyph` error is reported

#### Scenario: Preserved spaces
- **WHEN** preformatted text contains a run of three spaces
- **THEN** the run keeps its width

#### Scenario: Carriage return line feed
- **WHEN** the line ending is CRLF
- **THEN** it produces exactly one line break

### Requirement: Text indent applies to the first line

`text-indent` SHALL shift the first line of its block by the specified length or percentage of the content width. Following lines MUST keep the regular origin, and justification MUST use the reduced width.

#### Scenario: Indented first line
- **WHEN** a paragraph declares `text-indent: 20pt`
- **THEN** its first line starts 20 pt to the right
- **AND** the second line starts at the normal position

#### Scenario: Indent survives an anonymous wrapper
- **WHEN** a block with `text-indent` contains inline content wrapped in an anonymous box
- **THEN** the first line is still indented

### Requirement: Unitless line height is inherited as a factor

A unitless `line-height` SHALL be inherited as a factor and resolved against the font size of the element that uses it. Lengths and percentages MUST keep their absolute semantics.

#### Scenario: Factor follows the child font size
- **WHEN** a parent sets `font-size: 10pt; line-height: 2` and a child sets `font-size: 20pt`
- **THEN** the child's lines are 40 pt apart

#### Scenario: Absolute length stays absolute
- **WHEN** a parent sets `line-height: 30pt` and a child changes its font size
- **THEN** the line height stays 30 pt
