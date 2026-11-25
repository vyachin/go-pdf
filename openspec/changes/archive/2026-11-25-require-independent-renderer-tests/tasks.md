# Tasks

## 1. Implementation and verification

- [x] 1.1 Update the golden job to run all current independent-renderer tests with required tools alongside both snapshot layers. Verification: each current `TestIndependentRenderer` parent test reports a run and pass with required tools.
- [x] 1.2 Make the job fail when an expected test is missing, skipped or fails; verify the failure path with a controlled missing-tool run. Verification: a controlled missing tool or empty/omitted test selection makes the job logic fail.
- [x] 1.3 Update the CI ownership table in `CONTRIBUTING.md` without duplicating the existing renderer-pin work. Verification: the ownership table matches the actual job selection and existing pin work.
- [x] 1.4 Run the selected tests with Poppler, inspect workflow selection against `go test -list`, and run strict OpenSpec validation. Verification: selected tests pass with Poppler and the workflow selection matches the enumerated test names.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate require-independent-renderer-tests --strict` and review the diff for unintended changes outside this proposal.
