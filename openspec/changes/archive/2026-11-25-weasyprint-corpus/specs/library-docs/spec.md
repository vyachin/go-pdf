# Spec Delta

## MODIFIED Requirements

### Requirement: Contributor material has its own entry point

Architecture, test layers, golden snapshots, the mPDF corpus, the WeasyPrint
samples corpus, formatting, linting, performance measurement and the release
procedure SHALL be documented under `CONTRIBUTING.md`, directly or through links,
and MUST NOT be required reading for a consumer.

#### Scenario: A maintainer finds every workflow

- **WHEN** a maintainer opens `CONTRIBUTING.md`
- **THEN** it links to each workflow from a single page

#### Scenario: The README does not duplicate it

- **WHEN** contributor instructions change
- **THEN** only `CONTRIBUTING.md` and the documents it links need updating
