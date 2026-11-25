# Spec Delta

## ADDED Requirements

### Requirement: The creation date is validated in UTC (RV4-23)

The date check SHALL validate the year of the UTC instant that is written into
`/CreationDate`, not only the local year. A date whose UTC form falls outside
0000–9999 MUST fail with `options/date`.

#### Scenario: A local date near the year 10000

- **WHEN** `Options.Date` is 9999-12-31 23:30 in a zone twelve hours behind UTC
- **THEN** the call reports `options/date`
- **AND** no PDF with a five-digit year is produced

### Requirement: PNG colour type and bit depth are preserved (RV4-24)

A PNG SHALL be embedded with its source colour type and bit depth, including
`DeviceGray` and 16-bit samples, and MUST NOT lose pixel data. An alpha channel
remains a soft mask.

#### Scenario: A 16-bit grayscale PNG

- **WHEN** a document embeds a 16-bit grayscale PNG
- **THEN** the image stream keeps 16-bit samples in a gray colour space
- **AND** its low-order bits are not discarded

#### Scenario: A one-bit line drawing

- **WHEN** a document embeds a one-bit grayscale PNG
- **THEN** the embedded stream is not expanded to 24-bit RGB

### Requirement: TJ arrays stay within the compatibility limit (RV4-51)

Contiguous glyphs SHALL share one string, and a `TJ` array MUST NOT exceed 8,191
elements. A longer run SHALL be split across several operators without changing the
glyph positions.

#### Scenario: A twenty-thousand-character token

- **WHEN** a document contains one 20000-character token
- **THEN** the content stream uses several `TJ` operators of at most 8,191 elements
- **AND** the extracted text and the rendered positions are unchanged

### Requirement: W arrays are split into groups (RV4-52)

The `/W` array SHALL use multiple `startCID [widths]` groups so no array exceeds the
compatibility limit, for a font with thousands of distinct widths.

#### Scenario: A document with thousands of CIDs

- **WHEN** a document uses a font subset with 9000 distinct CIDs
- **THEN** `/W` is emitted as several groups
- **AND** no single width array exceeds the limit

### Requirement: A zero-area image is not painted (RV4-53)

An image operation whose width or height is not positive SHALL be omitted, so the
content stream never contains a non-invertible current transformation matrix.

#### Scenario: An image whose height rounds to zero

- **WHEN** a document declares an image wider than the page and a height that
  resolves to zero
- **THEN** the page does not paint the image
- **AND** the content stream contains no `cm` with a zero scale

### Requirement: BaseFont names stay within 127 bytes (RV4-54)

The subset tag and the family name written to `/BaseFont` and `/FontName` SHALL be
capped so the resulting name stays within the 127-byte architectural limit, while
different subsets keep distinct tags.

#### Scenario: A font with a very long family name

- **WHEN** an embedded font declares a 200-character family name
- **THEN** `/BaseFont` and `/FontName` are at most 127 bytes
- **AND** two different subsets of that font still have different tags

### Requirement: A PNG is decoded once and through typed fast paths (RV4-66)

The writer SHALL decode a PNG's pixels once per conversion, or reuse the decode the
loader already performed, and MUST use the typed fast paths for the common colour
models instead of reading every pixel through `At()`. The work MUST be charged
according to the budget requirements.

#### Scenario: A repeated PNG

- **WHEN** a document embeds the same PNG on several pages
- **THEN** its pixels are decoded once
- **AND** the conversion does not spend a second full decode in the writer

