# Tasks

## 1. Implementation and verification

- [x] 1.1 Add repeated large-image coverage for inline, background and repeated-page placements, counting hash invocations and bytes. Verification: an instrumented hash counter exposes one full image scan per old placement.
- [x] 1.2 Cache each digest once and pass shared image handles through `toPDFOps` without rescanning encoded bytes. Verification: 1,000 placements scan the unique image bytes once and embed one object.
- [x] 1.3 Charge unique hashing work before each bounded chunk and test resource exhaustion and context cancellation. Verification: small budgets fail before the next hash chunk and cancellation returns its context cause.
- [x] 1.4 Assert one image object and unchanged PDF bytes/order, register the adversarial case and run engine/PDF tests plus the shuffled suite and race check. Verification: PDF bytes/resources are stable for within-budget fixtures and all owning tests pass.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate cache-image-digests --strict` and review the diff for unintended changes outside this proposal.
