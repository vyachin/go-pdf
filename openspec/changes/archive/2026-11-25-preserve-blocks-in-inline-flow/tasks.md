# Tasks

## 1. Implementation and verification

- [x] 1.1 Add the exact nested block-image regression and a missing-image variant through `Validate` and `Convert`. Verification: the wrapped image is absent before the fix and the missing-source case currently escapes asset validation.
- [x] 1.2 Normalize inline/block boundaries without flattening replaced elements, tables or list items. Verification: layout operations contain a real image/table/list box in the original document order.
- [x] 1.3 Test text before/after the block, inherited style, hidden descendants, anchors, and one nested table/list case. Verification: each regression checks text order, inherited styles, hidden content and anchor ownership.
- [x] 1.4 Check image presence and text order with layout operations and independent extraction/rendering; run layout and engine tests and review affected snapshots. Verification: the independent renderer sees the image and the targeted suites and reviewed snapshots pass.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate preserve-blocks-in-inline-flow --strict` and review the diff for unintended changes outside this proposal.
