# Spec Delta

## ADDED Requirements

### Requirement: A GPOS FeatureList offset is bounds-checked (RV5-01)

Lookup selection SHALL read the `FeatureList` header and every feature record only
inside the GPOS table. An offset that does not fit the table MUST be treated as an
unusable feature list: the font SHALL load without kerning, and parsing MUST NOT
panic or interpret bytes outside the table.

#### Scenario: FeatureList past the table when the table is not last

- **WHEN** a font declares a valid `ScriptList` and a `FeatureList` offset beyond
  the table length, with file bytes after the table
- **THEN** the font loads without error
- **AND** no lookup is enabled from bytes outside the table
- **AND** kerning is not applied

#### Scenario: FeatureList past the table when the table ends the file

- **WHEN** the same malformed font has its GPOS table at the end of the file, so
  the table capacity ends with the file
- **THEN** the font loads or fails with a font error
- **AND** parsing does not panic

#### Scenario: Feature record past the table

- **WHEN** the `FeatureList` header fits but a feature record it points at does not
- **THEN** that feature contributes no lookups
- **AND** parsing does not read outside the table

#### Scenario: A well-formed FeatureList still selects lookups

- **WHEN** a font declares a valid `DFLT` or `latn` script whose `kern` feature
  points at a pair adjustment
- **THEN** that lookup is enabled and kerning is applied
- **AND** the existing kerning and GPOS tests stay green
