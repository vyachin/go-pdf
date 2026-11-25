# Tasks

## 1. Implementation and verification

- [x] 1.1 Add multiline body tests for missing glyph and missing face asserting file, nonzero line/column and owning element path. Verification: the missing-glyph/face tests assert the actual source file, nonzero coordinates and owning path.
- [x] 1.2 Carry provenance through inline pieces, lines, anonymous wrappers and generated content; use it in font/glyph/overflow diagnostics. Verification: diagnostics from body/generated/overflow paths preserve the supplied provenance.
- [x] 1.3 Test entities, NFC composition, nested spans, UTF-8 and a supported legacy encoding without fabricated character precision. Verification: the reported locations map to original encoded bytes or the known owning element.
- [x] 1.4 Preserve identical diagnostic lists between public entry points before serialization, then run layout, charset, engine and public API tests. Verification: pre-serialization diagnostic lists match and all named package suites pass.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate preserve-layout-source-spans --strict` and review the diff for unintended changes outside this proposal.
