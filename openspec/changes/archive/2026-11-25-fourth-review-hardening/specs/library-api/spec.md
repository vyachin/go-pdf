# Spec Delta

## ADDED Requirements

### Requirement: A repeated measurement is built once per width (RV4-03)

Layout SHALL build a box's lines at most once per content width, whether the box is
measured for a break decision or laid out, and every line built for a measurement
MUST be charged to the work guard. A document MUST NOT be able to spend quadratic
time in repeated measurement of the same subtree.

#### Scenario: Nested avoid boxes stay linear

- **WHEN** a document nests 240 `break-inside: avoid` blocks with 300 words each
- **THEN** the conversion builds each block's lines once per width
- **AND** with a small `MaxOperations` the call returns `resource/limit` without
  spending quadratic time
- **AND** the same document under the default limits produces the same pages as a
  document with the same text but no nesting

### Requirement: Text shaping is charged per rune (RV4-04)

Text shaping SHALL charge the work guard once per rune it normalizes, maps and
measures, and the charge MUST cover the per-rune glyph and state storage. A
document at `MaxInputBytes` MUST fail with `resource/limit` before allocating memory
out of proportion to the budget.

#### Scenario: A single huge word is bounded

- **WHEN** a document contains a 2 MB word and `MaxOperations` is 100000
- **THEN** the call returns `resource/limit`
- **AND** the memory allocated before the stop is proportional to the budget, not
  to the input
- **AND** a document that fits the budget keeps its byte-identical output

### Requirement: PDF serialisation is charged by bytes (RV4-06)

PDF serialisation SHALL charge the guard in proportion to the bytes it writes, the
glyphs and CIDs it serialises, and the pixels it decodes, so `MaxOperations` bounds
writer cost and not only the number of operations on the page.

#### Scenario: A large output needs a large budget

- **WHEN** a document produces a PDF of several megabytes from a small input (for
  example 9000 distinct CIDs)
- **THEN** the recorded spend grows with the output size
- **AND** a small `MaxOperations` stops the conversion with `resource/limit` before
  the output is assembled

### Requirement: Validate reports everything Convert can decide before serialisation (RV4-09)

`Validate` SHALL run the same preparation and layout as `Convert`, including the
validation of document metadata. The package documentation MUST name PDF
serialisation, including its work budget, as the only stage `Validate` does not
cover. A document that `Validate` accepts MUST NOT fail `Convert` for a reason
decided before serialisation.

#### Scenario: Metadata is validated by both entry points

- **WHEN** a caller passes a title that is not valid UTF-8
- **THEN** `Convert` and `Validate` both report `options/title`
- **AND** `Validate` returns the same diagnostic list as `Convert`

#### Scenario: The uncovered stage is documented

- **WHEN** a document is accepted by `Validate` and fails in `Convert` only because
  serialisation exceeds the work budget
- **THEN** the error is a `resource/limit` from serialisation
- **AND** the documented scope of `Validate` names that stage

### Requirement: An invalid option still validates inline rules (RV4-28)

When an option is invalid, preparation MUST still validate the declarations the
document carries inline, including the rules of a `<style>` element, and report
them together with the option diagnostics in one pass. A linked asset MUST NOT be
opened for a request that cannot proceed.

#### Scenario: A style element and a bad page size

- **WHEN** `PageSize: "banana"` is combined with `<style>p{display:flex}</style>`
- **THEN** the report contains `options/page-size` and `css/unsupported-value`
- **AND** no linked stylesheet or font is opened

### Requirement: A missing font names its declaration (RV4-37)

A `font/unavailable` diagnostic SHALL carry the file name of the document and the
position of the `src` declaration that named the missing file, so the report points
at the line the author can fix.

#### Scenario: A missing font file

- **WHEN** a document declares `@font-face { src: url(missing.ttf) }`
- **THEN** the diagnostic is `font/unavailable`
- **AND** it carries the document name and the line and column of the `src`
  declaration

### Requirement: The charset check accepts the empty name (RV4-59)

`CharsetSupported` SHALL report true for the empty string, because the empty
`Options.Charset` is a valid request that means "detect". The documentation MUST
state that a caller checking only named encodings writes
`name != "" && CharsetSupported(name)`.

#### Scenario: The empty name is valid

- **WHEN** a caller checks `CharsetSupported("")`
- **THEN** the result is true
- **AND** `CharsetSupported("shift_jis")` stays false

### Requirement: A budget failure stops dependent diagnostics (RV4-62)

Once the work guard reports exhaustion or cancellation, later stages MUST NOT add
diagnostics that depend on the work that was skipped, such as a missing font
reported for a face the exhausted budget never tried to resolve. The report MUST
contain the budget or context cause and no misleading follow-on error.

#### Scenario: An exhausted font budget does not report a missing family

- **WHEN** a document's font work exhausts `MaxOperations` before the fallback
  family is resolved
- **THEN** the report contains `resource/limit`
- **AND** it does not also contain `text/font` for that family

### Requirement: One pipeline serves both entry points (IMP-02)

`Convert` and `Validate` SHALL share one implementation of preparation, layout and
metadata checks, with only the output stage differing. A behaviour added to a shared
stage MUST reach both entry points without a second code path or a duplicated test.

#### Scenario: A new preparation check (IMP-02)

- **WHEN** a check is added to preparation
- **THEN** both entry points report it
- **AND** the suite does not need two fixtures for the same check

### Requirement: Every budgeted stage states a cost and has an invariant test (IMP-03)

For every stage the work guard covers, the code SHALL document what one unit means,
and the suite MUST contain an invariant test: for an adversarial fixture at
`MaxOperations = N`, time and allocations stay within a constant multiple of `N`. A
stage that cannot state its cost MUST NOT be described as covered by the budget.

#### Scenario: A new expensive stage (IMP-03)

- **WHEN** a stage that grows with the input is added
- **THEN** it charges the guard and the invariant test covers it
- **AND** a stage that allocates per unit without a charge fails that test

