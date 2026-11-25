# Tasks

## 1. Implementation and verification

- [x] 1.1 Update the README, package documentation and font guide with the same reproducibility prerequisites. Verification: all three consumer documents state or directly link the same complete-input prerequisites.
- [x] 1.2 Align the output requirement and preserve fixed-date and document-id guarantees. Verification: the delta retains fixed-date and deterministic-id scenarios while naming asset bytes.
- [x] 1.3 Check repeated conversion with an explicit bundled font and immutable assets; demonstrate that replacing an image or stylesheet changes the complete input. Verification: existing determinism tests pass for identical immutable inputs and a controlled resource mutation changes the complete input.
- [x] 1.4 Run existing determinism/public API tests and documentation checks plus strict OpenSpec validation; no runtime change or golden regeneration is required. Verification: existing tests and strict validation pass with documentation/spec changes only.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate document-reproducible-inputs --strict` and review the diff for unintended changes outside this proposal.
