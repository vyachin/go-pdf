# Proposal

## Why

The contributor architecture diagram names `backend/pdf/`, but the actual writer is `internal/pdf`. It also omits the engine orchestration and states that results never depend on floating point even though shaping and descriptor scaling use explicit floating-point rounding.

Review reference: **D01** (documentation), architecture/code/documentation review of `7c7ac70`.

## What Changes

- Correct the package paths and show `pdfgen` and engine orchestration around the rendering pipeline.
- Describe ownership of the work guard, resource lifetime, layout drawing operations and debug rasterizer.
- State integer millipoint storage accurately and describe the actual scaling/rounding boundary.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-docs`: Require the architecture reference to identify the actual modules and their contracts.

## Impact

`CONTRIBUTING.md` is the architecture entry point. The repository contains `internal/pdf/writer.go`, `internal/engine/convert.go`, `internal/layout`, `internal/text/shape.go` and `internal/backend/raster`; it has no `internal/backend/pdf` package. `text.Scale` and PDF descriptor conversion use floating point.

No overlapping active change was found for this finding.

No new public Go identifiers or external dependencies are required.
