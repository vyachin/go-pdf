# Proposal

## Why

List numbering repeatedly scans every preceding sibling. The review reproduced about 800 million sibling visits for 40,000 empty list items under the default work budget, so a small template can spend quadratic CPU time without a resource diagnostic.

Review reference: **R01** (P1), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Assign list indexes in one ordered traversal per list parent.
- Charge list traversal to the conversion guard and observe cancellation.
- Preserve numbering, nested-list scope and PDF bytes for ordinary lists; register the pathological case.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: Make list indexing linear and explicitly covered by the work budget.

## Impact

`internal/layout/box.go` calls `liIndex` from `buildElement`; `liIndex` walks `n.Parent.Children` from the beginning for each item. The same calculation runs for unordered lists even though they usually draw bullets. The existing `Lists` requirement covers visible markers but does not constrain this repeated scan.

No overlapping active change was found for this finding.

The public Go API and dependency allowlist remain unchanged. Planning is the only work performed by this change creation; implementation tasks are unchecked.
