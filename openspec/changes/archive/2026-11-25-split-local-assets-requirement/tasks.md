# Tasks

## 1. Verification

- [x] 1.1 Run the sync and compare the two new requirements against the old block scenario by scenario, so no scenario or rule is lost. Verification: every old scenario appears verbatim under one of the two requirements.
- [x] 1.2 Run `openspec validate --specs` and `openspec validate --all --strict`; the long-requirement warning must disappear without a new warning. Verification: strict validation passes for `css-profile` and the totals no longer report it.
- [x] 1.3 Review the diff for anything outside `openspec/specs/css-profile/spec.md`. Verification: no engine, test or documentation change is included.

## 2. Acceptance

- [x] 2.1 Confirm the engine behaviour is unchanged: the profile/asset tests and the engine suite still pass. Verification: `go test ./internal/profile ./internal/engine` passes.
- [x] 2.2 Run `openspec validate split-local-assets-requirement --strict` and archive the change. Verification: the change is valid and archived with both requirements synced.
