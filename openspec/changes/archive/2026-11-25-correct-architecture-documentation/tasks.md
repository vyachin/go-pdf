# Tasks

## 1. Implementation and verification

- [x] 1.1 Correct the paths and diagram in `CONTRIBUTING.md`, including the public API and engine. Verification: every diagram path resolves to a tracked package and the arrows match orchestration.
- [x] 1.2 Document actual collaborator ownership and the integer-storage versus scaling/rounding distinction. Verification: the prose matches `text.Scale` and PDF descriptor rounding as well as actual collaborator ownership.
- [x] 1.3 Cross-check every named package against tracked directories and the relevant imports; review the rendered Markdown. Verification: the rendered diagram and package links are navigable and no proposed refactor is described as delivered.
- [x] 1.4 Run strict OpenSpec validation; do not change runtime code or regenerate PDF snapshots for this documentation-only implementation. Verification: the strict change validation passes and the implementation diff contains documentation only.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate correct-architecture-documentation --strict` and review the diff for unintended changes outside this proposal.
