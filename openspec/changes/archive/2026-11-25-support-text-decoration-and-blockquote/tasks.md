# Tasks

## 1. Implementation and verification

- [x] 1.1 Accept `u`, `s`, `del`, `ins` and `blockquote` in the profile and add their engine-stylesheet rules. Verification: validation accepts each element and the default rules produce underline, strike-through and quote indentation.
- [x] 1.2 Parse `text-decoration` (`none`, `underline`, `line-through`, both keywords), inherit it to descendants and reject every other value. Verification: cascade tests prove acceptance, inheritance and rejection with the value named.
- [x] 1.3 Paint one rectangle per decorated run below the baseline for `underline` and across the x-height for `line-through`, in the run colour. Verification: layout operations contain the expected rectangle geometry for both decorations.
- [x] 1.4 Add a snapshot fixture, run both snapshot layers, `TestProfileCoverage`, the engine and layout suites, the shuffled suite and the race check. Verification: the fixture is valid without warnings, snapshots are reviewed, and every named suite passes.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate support-text-decoration-and-blockquote --strict` and review the diff for unintended changes outside this proposal.
