# Spec Delta

## ADDED Requirements

### Requirement: Independent renderer checks run in the required CI gate

CI SHALL execute the complete `TestIndependentRenderer` test family in a required
job with `pdftoppm` and `pdftotext` installed and `PDF_REQUIRE_RENDER=1`. The job MUST
retain both snapshot layers and MUST fail when a required independent check is
missing, skipped or unsuccessful. A test selection that runs none of the required
checks MUST NOT pass the gate.

#### Scenario: All independent checks execute

- **WHEN** a pull request runs the quality gate
- **THEN** the content, coordinates/colors and transparency independent-renderer checks each run and pass in the tool-equipped job

#### Scenario: A tool is unavailable

- **WHEN** either required Poppler executable is absent from that job
- **THEN** the check and gate fail rather than skipping the tests

#### Scenario: A required test is omitted

- **WHEN** a filter or rename removes a required independent-renderer test from the selected run
- **THEN** the job detects the missing execution and fails even if `go test` exits successfully
