# Spec Delta

## ADDED Requirements

### Requirement: The line index is bounded and charged (RV5-12)

The HTML and CSS parsers SHALL NOT store an unbounded line-index entry per input
newline. The index MUST be capped or built lazily, every stored entry SHALL be charged
to the work guard before the slice grows, and a stylesheet made of newlines MUST fail
with `resource/limit` once the cap is reached instead of allocating hundreds of
megabytes. Diagnostics past the cap MUST still report the correct line and column.

#### Scenario: A stylesheet made of newlines

- **WHEN** a linked stylesheet of 7 MiB consists of newlines and `MaxOperations` is
  small
- **THEN** the call returns `resource/limit`
- **AND** the heap allocated before the stop is a small multiple of the stylesheet size

#### Scenario: The same size of ordinary CSS

- **WHEN** a linked stylesheet of 7 MiB consists of `x` characters inside one comment
- **THEN** the conversion succeeds
- **AND** it allocates tens of MiB, not hundreds

#### Scenario: A position past the capped index

- **WHEN** a diagnostic points at a line after the capped number of stored starts
- **THEN** its line and character column are correct
- **AND** the lookup is charged like any other scan

### Requirement: Aggregate stylesheet bytes are bounded (RV5-13)

`Limits` SHALL bound the total encoded bytes of linked stylesheets in one conversion
through `MaxTotalStylesheetBytes`, in addition to `MaxStylesheetBytes` per file. The
default SHALL be 16 MiB (16777216 bytes). A stylesheet that would cross the total MUST
fail with `resource/limit` before it is read, `DefaultLimits` MUST report the value, and
the documentation MUST state it.

#### Scenario: Many stylesheets cross the total

- **WHEN** a document links 60 unique 1 MiB stylesheets and the default limits apply
- **THEN** the call returns `resource/limit` before the seventeenth file is read
- **AND** the heap does not grow with the stylesheets past the limit

#### Scenario: One stylesheet up to the per-file limit

- **WHEN** a document links one 8 MiB stylesheet under the default totals
- **THEN** the conversion succeeds and the PDF is unchanged

#### Scenario: The caller raises the total

- **WHEN** the caller sets `MaxTotalStylesheetBytes` above the document's total
- **THEN** every linked stylesheet is read and the conversion succeeds

#### Scenario: One file linked twice counts once

- **WHEN** a document links the same stylesheet twice
- **THEN** the bytes are counted once against the total
- **AND** both rules keep their place in the cascade
