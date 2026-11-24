# Spec Delta

## ADDED Requirements

### Requirement: Independent validation in the test pipeline

The test pipeline SHALL validate produced PDFs with an independent validator located by `exec.LookPath`, with an explicit `PDFCPU` override. When `PDF_REQUIRE_VALIDATOR=1` is set and no validator is found, the tests MUST fail; otherwise the skip MUST be reported. Continuous integration MUST run the pipeline with the validator installed and the variable set.

#### Scenario: Validator available
- **WHEN** the tests run with pdfcpu in `PATH` or `PDFCPU` set
- **THEN** every produced PDF is validated by that tool

#### Scenario: Validator required but missing
- **WHEN** `PDF_REQUIRE_VALIDATOR=1` is set and no validator is found
- **THEN** the test fails with a message naming the variable

#### Scenario: Local run without the tool
- **WHEN** the variable is unset and no validator is found
- **THEN** the test passes and logs that independent validation was skipped
