# Tasks

## 1. Implementation and verification

- [x] 1.1 Add layout/public regressions comparing 10 by 10 and 200 by 200 attributes with equivalent pixel CSS. Verification: rectangle coordinates distinguish 10 by 10 from 200 by 200 and agree with equivalent CSS sizing.
- [x] 1.2 Implement checked dimension hints before the author cascade, preserving explicit CSS `auto` and `!important` precedence. Verification: cascade tests prove CSS values, `auto` and `!important` override the presentation hints.
- [x] 1.3 Test one dimension/aspect ratio, zero, malformed/negative/overflow values, and inline versus block images. Verification: geometry tests prove aspect ratio and invalid/zero dimensions behave as specified.
- [x] 1.4 Update `docs/profile.md` with units and precedence; run CSS, layout and engine tests and review only affected snapshots. Verification: the documented syntax matches the passing CSS/layout/engine tests and affected snapshot review.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate honor-html-image-dimensions --strict` and review the diff for unintended changes outside this proposal.
