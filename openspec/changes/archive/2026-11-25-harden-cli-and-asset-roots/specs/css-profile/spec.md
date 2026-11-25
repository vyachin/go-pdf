# Spec Delta

## ADDED Requirements

### Requirement: A failed asset root stops asset reads

When a configured base or font directory cannot be opened, the conversion SHALL fail
with an `options/asset-root` diagnostic before any asset is read. It MUST NOT continue
with an unbounded root, and a linked path outside the intended directory MUST NOT be
opened.

#### Scenario: A missing base directory

- **WHEN** `BaseDir` names a directory that does not exist and the document links an
  absolute stylesheet outside it
- **THEN** the call returns `options/asset-root`
- **AND** the outside stylesheet is not read or parsed

#### Scenario: A valid base directory

- **WHEN** `BaseDir` exists and the document links a stylesheet inside it
- **THEN** the stylesheet loads and the conversion proceeds
