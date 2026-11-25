# Spec Delta

## ADDED Requirements

### Requirement: Block descendants of inline elements retain their boxes

A supported block descendant of an inline element SHALL generate its own box in
normal flow. Inline content before and after it MUST be split into ordered inline
runs; the descendant MUST NOT be replaced by only a line break or its text children.
Replaced images, tables and lists MUST retain their own layout behavior and asset
validation.

#### Scenario: A block image inside a span

- **WHEN** a document contains `<span><img src="logo.png" style="display:block"></span>`
- **THEN** the image is loaded and painted as a block and the PDF contains its image object
- **AND** validation succeeds without a content-loss warning

#### Scenario: Text surrounds a nested block

- **WHEN** an inline wrapper contains text, a block image and more text
- **THEN** the text, image and following text occur in that order in separate flow fragments
- **AND** inherited styles and anchors remain attached to the correct fragment

#### Scenario: A nested block references a missing image

- **WHEN** the image inside that inline wrapper does not exist
- **THEN** both entry points report `img/load` at the image element

#### Scenario: A nested table or list is preserved

- **WHEN** an inline wrapper contains a supported table or list styled as a block
- **THEN** its cells or markers are constructed and rendered rather than flattened away
