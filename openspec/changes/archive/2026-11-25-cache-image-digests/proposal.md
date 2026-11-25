# Proposal

## Why

`toPDFOps` computes SHA-256 over the complete encoded image for every placement before checking the deduplication map. A 3.76 MB JPEG placed 1,000 times therefore hashes about 3.76 GB outside the work guard even though only one image object is embedded.

Review reference: **R02** (P1), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Compute and retain one content digest per loaded image identity for the conversion.
- Reuse the digest and PDF image handle across placements and pagination passes.
- Charge hashing by bytes before work starts and check cancellation between bounded chunks.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `pdf-output`: Extend repeated-image reuse to content hashing and PDF operation conversion.

## Impact

`internal/engine/images.go` already caches an `ImageData` by canonical file path or data-URI content. `internal/engine/convert.go:toPDFOps` discards that identity and calls `sha256.Sum256(img.Data)` per operation. `pdf-output` requires one decode and object, but not one hash.

The later `centralize-conversion-assets` proposal must preserve this invariant. RV4-06 in `fourth-review-hardening` charges writer bytes; this finding concerns the earlier `toPDFOps` hashing stage and is independently actionable.

No new public Go identifiers or external dependencies are required.
