# Spec Delta

## MODIFIED Requirements

### Requirement: The import runs only after the strict profile failed

The engine SHALL validate the document as written first. The browser import MUST run only when that validation produced errors, the caller enabled `Options.ImportBrowser`, and the document is not in strict mode. A document that already passes the profile SHALL be converted without any lowering. The reference command SHALL enable the import for the single conversion it performs, so that a browser-oriented template converts without an extra flag.

#### Scenario: A valid profile template
- **WHEN** a document inside the profile is converted with the import enabled
- **THEN** no lowering runs and the output is identical to a conversion with the import disabled

#### Scenario: A browser document with the import enabled
- **WHEN** a browser document is converted with `Options.ImportBrowser`
- **THEN** the strict errors are replaced by the import report and the rewritten document is validated again

#### Scenario: Strict mode
- **WHEN** the same browser document is converted with `Options.Strict` or validated with `Validate`
- **THEN** the original profile diagnostics are reported and no PDF is returned
