# Spec Delta

## ADDED Requirements

### Requirement: A table's bottom decoration stays on the page (RV5-43)

A table's completed fragment SHALL keep its bottom padding and border inside the
page. When the decoration would cross the page bottom, it MUST be clipped to the
page or reported with `layout/overflow`, and MUST NOT be painted past the MediaBox.

#### Scenario: Bottom padding taller than the page

- **WHEN** a table with `padding-bottom:200pt` and a background is laid out on a
  100 pt page
- **THEN** the background fill ends at or above the page content bottom
- **AND** exactly one `layout/overflow` is reported for the clip

#### Scenario: An ordinary table

- **WHEN** a table's bottom padding and border fit on the page
- **THEN** the decoration is painted unchanged
- **AND** no overflow diagnostic is reported

#### Scenario: A table split across pages

- **WHEN** a table spans two pages
- **THEN** the first fragment's bottom decoration belongs to the first page
- **AND** the last fragment's bottom decoration belongs to the last page

### Requirement: Margin box geometry stays within the page (RV5-44)

Margin box width and positions SHALL be computed with saturating 64-bit arithmetic
and clamped to the page. A page margin larger than the page MUST NOT produce a
negative or wrapped coordinate, and margin box content MUST either be painted
inside the page or reported with a diagnostic naming the box.

#### Scenario: A margin larger than the page

- **WHEN** a 200 pt page declares `margin:10pt 2000000pt` and
  `@bottom-center { content: "FOOT" }`
- **THEN** no painted coordinate for the footer is negative or wrapped
- **AND** the footer is either painted inside the page or reported by a diagnostic
  that names `bottom-center`

#### Scenario: A content box of zero width

- **WHEN** the horizontal margins leave no content width
- **THEN** the margin box reports one diagnostic
- **AND** no coordinate outside the page reaches the content stream

#### Scenario: Ordinary margin boxes

- **WHEN** `@top-left`, `@top-center`, `@top-right` and the bottom boxes are
  declared with normal margins
- **THEN** their positions are unchanged
- **AND** no diagnostic is reported
