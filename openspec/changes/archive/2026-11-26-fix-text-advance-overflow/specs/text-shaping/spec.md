# Spec Delta

## ADDED Requirements

### Requirement: Run widths accumulate in 64-bit and saturate (RV5-09)

A shaped run and every line width SHALL be accumulated in `int64` and clamped
explicitly to `MaxInt32`. The resulting width MUST never be negative and MUST be
identical on 32-bit and 64-bit builds. A run whose true width is below the limit
MUST keep its exact width.

#### Scenario: A word wider than the range

- **WHEN** a single word of 350,000 `ж` at 12 pt is shaped
- **THEN** the run width is `MaxInt32`, not a negative number
- **AND** no advance or line width in the output is negative

#### Scenario: The same document on a 32-bit build

- **WHEN** the same fixture is rendered by a `GOARCH=386` build
- **THEN** the run width, line width and diagnostics equal the 64-bit result

#### Scenario: An in-range width is exact

- **WHEN** a word's true width is below `MaxInt32`
- **THEN** the run width equals the exact sum of its advances

### Requirement: A saturated run reports layout/overflow (RV5-09)

When a run or line width saturates, or when the true width exceeds the content
box, the conversion SHALL report one `layout/overflow` warning carrying the
owning element's position. Saturation MUST NOT suppress the warning, and a normal
word MUST NOT produce it.

#### Scenario: The 350,000 character word warns

- **WHEN** the 350,000 character word is laid out
- **THEN** the document reports `layout/overflow` once for the paragraph
- **AND** the PDF is produced with the run clipped, as for any overflowing word

#### Scenario: A short long word still warns

- **WHEN** a 1,000 character word is laid out
- **THEN** it reports the same `layout/overflow` warning it reports today

#### Scenario: A normal word is quiet

- **WHEN** every word fits its content box
- **THEN** no `layout/overflow` warning is reported

### Requirement: text.Scale clamps before narrowing (RV5-09)

`Scale` and the face metrics SHALL compute their product in `float64` or `int64`
and clamp explicitly to `[-MaxInt32, MaxInt32]` before narrowing. An out-of-range
conversion MUST NOT depend on the Go implementation's float-to-integer behaviour,
and a document whose font size requires clamping MUST be reported rather than
rendered with wrapped geometry.

#### Scenario: An out-of-range scale

- **WHEN** `Scale(60000, 2000000000, 1000)` is evaluated on amd64 and on arm64
- **THEN** both return `MaxInt32`
- **AND** neither architecture returns a value of the opposite sign

#### Scenario: An extreme font size is diagnosed

- **WHEN** a document sets a font size whose scaled advance or metric would leave
  the `int32` range
- **THEN** the conversion reports `layout/overflow` at the element
- **AND** the document is not rendered with a wrapped negative advance

#### Scenario: Ordinary sizes are unchanged

- **WHEN** a document uses a font size within the geometry range
- **THEN** its glyph advances and metrics equal the current values
