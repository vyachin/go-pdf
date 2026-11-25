# Tasks

## 1. Implementation and verification

- [x] 1.1 Introduce a test seam for deterministic fallback candidates and reproduce an aggregate cap of one byte. Verification: the injected candidate reproduces `text/font` under a one-byte cap before the fix.
- [x] 1.2 Propagate per-file, aggregate, limited-read and parser limit causes plus cancellation through fallback selection. Verification: each terminal refusal retains its typed cause through face selection.
- [x] 1.3 Test genuine absence versus terminal refusal, including a bold/italic sibling past the total, with no dependent `text/font`. Verification: only `resource/limit` or the context error is returned for terminal failures, while a missing candidate can fall through.
- [x] 1.4 Run font, engine and public API tests and the race suite; document `resource/limit` consistently. Verification: font/engine/public and race tests pass with deterministic candidates and the documented code.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate propagate-fallback-font-limits --strict` and review the diff for unintended changes outside this proposal.
