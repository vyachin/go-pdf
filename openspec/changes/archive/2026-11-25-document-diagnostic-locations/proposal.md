# Proposal

## Why

The profile promises a file, line, column and element path for every error, while the public `Diagnostic` contract permits zero line/column for unknown positions. The main API spec also contradicts itself by requiring positions only when available and then allowing omissions only for options.

Review reference: **D02** (documentation), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Define source-attributable, option and genuinely aggregate/output diagnostic location rules.
- Correct the profile and godoc to explain unknown zero positions and file placeholders.
- Retain the stronger requirement to preserve known layout provenance; do not use documentation to excuse discarded locations.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: Resolve the contradictory unknown-location wording without dropping known source positions.
- `library-docs`: Document diagnostic location availability and examples accurately.

## Impact

`docs/profile.md` says every error has all location fields; `pdfgen/pdfgen.go` documents zero line/column as unknown. `library-api` currently says both "when a source position exists" and "Only options/* ... MAY lack a position". A serialization budget failure does not necessarily identify one source node.

RV4-27 concerns generated-content documentation and position consistency. This proposal adds an explicit unknown-position contract and does not replace `preserve-layout-source-spans`.

No new public Go identifiers or external dependencies are required.
