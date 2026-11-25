# Tasks

## 1. Implementation and verification

- [x] 1.1 Add a body/header/footer regression using `сло́во` at the same font and size and assert equal relative mark coordinates. Verification: body and margin glyph coordinates expose the discarded mark offsets before the fix.
- [x] 1.2 Extract the offset-aware shaped-run emitter and use it for body, margin boxes and markers. Verification: equal face/size runs use the same relative offset-aware glyph placements in each caller.
- [x] 1.3 Test nonzero vertical offsets, stacked marks, kerning, alignment and missing mark attachment. Verification: the focused shaping and painting regressions pass for both X and Y offsets.
- [x] 1.4 Verify the copyable combining sequence with independent extraction, review affected rasters and run layout, text, engine and PDF tests. Verification: independent extracted text preserves combining sequences and owning tests/snapshots pass.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate unify-shaped-text-painting --strict` and review the diff for unintended changes outside this proposal.
