# Tasks

## 1. Implementation and verification

- [x] 1.1 Add a regression for a 40,000-item shallow list plus cases for `start`, nested lists and non-list siblings. Verification: the new cases reproduce prefix rescanning before the fix and preserve existing list labels.
- [x] 1.2 Replace repeated sibling scans with a single parent traversal and account for its work through `budget.Guard`. Verification: the traversal counter and `Guard.Used()` grow linearly in a focused layout test.
- [x] 1.3 Assert linear traversal/guard spend with a deterministic counter and exercise exhaustion and cancellation without wall-clock thresholds. Verification: budget exhaustion returns `resource/limit` and cancellation returns the context error.
- [x] 1.4 Register a bounded adversarial fixture and run layout, engine and public API tests, followed by the shuffled suite and race check. Verification: `TestAdversarialManifest`, owning-package tests and the shuffled/race suites pass.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate bound-list-numbering --strict` and review the diff for unintended changes outside this proposal.
