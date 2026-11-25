# Spec Delta

## ADDED Requirements

### Requirement: Reproducibility documentation names every input

Every consumer-facing byte-reproducibility guarantee SHALL state or directly
reference its prerequisites: identical engine build, HTML bytes, options, resolved
stylesheet/image/font bytes and resolution choices. It MUST recommend explicit
fonts and immutable resources for reproducible deployments and explain that the
fixed default creation date removes clock dependence. System-font fallback MUST
remain documented as environment-dependent convenience.

#### Scenario: A reader evaluates the README guarantee

- **WHEN** the reader encounters the deterministic-output guarantee
- **THEN** its complete-input prerequisites are visible or directly linked without requiring contributor documentation

#### Scenario: A deployment requires repeatable documents

- **WHEN** a caller follows the font and resource guidance
- **THEN** the guidance uses explicitly selected font bytes and stable stylesheet/image resources rather than assuming system fallback is identical everywhere
