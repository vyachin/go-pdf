# Spec Delta

## ADDED Requirements

### Requirement: Table columns stay inside the content box (RV4-15)

A table's column widths SHALL be clamped or scaled to the available content width.
A table whose declared widths cannot fit MUST report `layout/overflow` once, and a
later cell MUST NOT be placed outside the page without a diagnostic.

#### Scenario: A cell wider than the page

- **WHEN** a document contains
  `<table><tr><td style="width:1000pt">WIDE</td><td>x</td></tr></table>` on A4
- **THEN** the columns fit the content box or the conversion reports
  `layout/overflow`
- **AND** the second cell is not silently placed beyond the page edge

### Requirement: An unbreakable token that cannot fit is reported (RV4-16)

When the first word on a line is wider than the content box and the word offers no
break opportunity, layout SHALL report `layout/overflow`. The text MUST NOT be
painted past the page edge and clipped without a diagnostic.

#### Scenario: A long word

- **WHEN** a document contains a thousand-character word with no spaces
- **THEN** the conversion reports `layout/overflow` for the block
- **AND** it either breaks the word or states that the content is clipped

#### Scenario: A CJK paragraph

- **WHEN** a document contains a paragraph of CJK characters with no spaces
- **THEN** it is broken between characters or reported as overflowing
- **AND** characters are not silently lost at the page edge

### Requirement: The image format comes from the content (RV4-21)

The image format SHALL be detected from the file's magic bytes, not from its
extension. A PNG named with a `.jpg` extension or with no extension MUST load as a
PNG, and a format mismatch diagnostic MUST name the detected format.

#### Scenario: A PNG with the wrong extension

- **WHEN** a valid PNG is referenced as `logo.jpg` or `logo`
- **THEN** it loads and embeds as a PNG
- **AND** a genuinely unsupported format is rejected by its detected type, not its
  name

### Requirement: An asset reference is resolved as a URL path (RV4-22)

A relative image, stylesheet or font reference SHALL be resolved as a URL path: the
query and the fragment are removed, the path is percent-decoded, ASCII whitespace is
trimmed, and a scheme-relative `//host` reference is treated as remote. A normal
reference copied from a web template MUST resolve to the local file it names.

#### Scenario: A cache-busting query

- **WHEN** a template references `logo.png?v=2`, `logo.png#top`,
  `logo%20x.png` or `" logo.png "`
- **THEN** the engine reads the file `logo.png` or `logo x.png`
- **AND** the diagnostic, when the file is missing, names the local path

### Requirement: Layout geometry saturates (RV4-32)

Every geometry addition and subtraction SHALL saturate at the `int32` bounds, and
the content box MUST be clamped to a non-negative width and height. A page margin
larger than the page MUST NOT produce a wrapped width or coordinates.

#### Scenario: A margin larger than the page

- **WHEN** a caller sets a margin of 2,000,000 pt on an A4 page
- **THEN** the content box is clamped to a non-negative size
- **AND** no painted coordinate wraps to the opposite side of the page

### Requirement: Image area is computed in 64 bits (RV4-33)

The pixel area of an image SHALL be computed in 64-bit arithmetic on every platform
before it is compared with `MaxImagePixels` or an aggregate limit, including on a
32-bit build.

#### Scenario: A very large declared image on 32 bits

- **WHEN** an image header declares a width and height whose product exceeds the
  32-bit integer range
- **THEN** the area is compared with the limit in 64 bits
- **AND** the image is rejected with `resource/limit`

### Requirement: A failed image load does not consume the aggregate budget (RV4-34)

The aggregate image totals SHALL count an image only after it decodes successfully,
or refund it on failure. A corrupt image MUST NOT make a later valid image fail the
aggregate limit.

#### Scenario: A corrupt image before a good one

- **WHEN** a document references a corrupt image and then a valid image, and the
  two fit the aggregate limit together
- **THEN** the corrupt image reports `img/load`
- **AND** the valid image still loads

### Requirement: A truncated JPEG is rejected at load time (RV4-35)

JPEG data SHALL be checked for completeness, or fully decoded within the pixel
budget, before it is embedded. A truncated JPEG MUST fail with `img/load` in both
entry points, exactly as a truncated PNG does.

#### Scenario: A JPEG cut short

- **WHEN** a valid JPEG is truncated before its end-of-image marker
- **THEN** `Convert` and `Validate` both report `img/load`
- **AND** no PDF embeds the truncated data

### Requirement: One asset resolver serves images, stylesheets and fonts (IMP-05)

A single resolver SHALL strip query and fragment, percent-decode, trim ASCII
whitespace, classify schemes and sniff the image format, and images, stylesheets and
fonts MUST all use it. A new scheme or format SHALL be added in one place.

#### Scenario: A new reference form (IMP-05)

- **WHEN** a scheme, an encoding or an image format is supported
- **THEN** all three asset kinds behave the same
- **AND** no asset kind keeps its own path parsing

