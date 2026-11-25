# Tasks

## 1. Implementation and verification

- [x] 1.1 Align the main diagnostic-location rule via the delta and update profile/godoc wording and examples. Verification: profile and godoc examples use the same known/unknown-position contract as the delta.
- [x] 1.2 Retain tests for source-attributable missing assets and linked stylesheets, and add a public aggregate/output error example with zero location but a file. Verification: existing source-position tests remain valid and an aggregate failure retains a file with unknown coordinates.
- [x] 1.3 Verify no known source position is discarded under the clarified rule and cross-check the source-provenance change. Verification: the provenance cases keep known locations and codes unchanged.
- [x] 1.4 Run existing diagnostic-contract/public API tests and strict OpenSpec validation; avoid adding string-matching tests for incidental prose. Verification: the diagnostic/public API suites and strict validation pass without brittle prose assertions.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate document-diagnostic-locations --strict` and review the diff for unintended changes outside this proposal.
