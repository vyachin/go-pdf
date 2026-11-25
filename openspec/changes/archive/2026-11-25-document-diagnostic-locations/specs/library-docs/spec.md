# Spec Delta

## ADDED Requirements

### Requirement: Diagnostic documentation states location availability

The diagnostic reference SHALL distinguish known source positions, option errors
and genuinely aggregate/output failures. It MUST explain that zero line or column
means unknown, that an element path is supplied only when an owning element is
known, and that linked stylesheet diagnostics use the stylesheet file. It MUST NOT
promise complete source coordinates for every error, or excuse missing coordinates
when the originating source location is available.

#### Scenario: A caller handles an aggregate diagnostic

- **WHEN** the caller reads a document-named diagnostic with zero line and column
- **THEN** the documentation explains those fields as unknown and shows how to format the diagnostic without invented coordinates

#### Scenario: A source declaration fails

- **WHEN** a stylesheet declaration has a known source position
- **THEN** the reference promises its actual source file and position rather than the generic aggregate fallback
