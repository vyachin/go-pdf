# Spec Delta

## ADDED Requirements

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
