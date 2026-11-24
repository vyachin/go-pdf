# Proposal

## Why

The suite has no corpus-level assertion on either what the engine understood
from a document or how the finished page looks. Text goldens compare a
whitespace-free glyph stream, so a change in line breaking, column widths, font
size or colour passes as long as the characters and the page count survive. The
only visual check renders a page and asserts that it is neither blank nor mostly
covered.

The README already promises both missing layers - "Layout snapshots check
geometry, not pixels" and "Visual regressions use the engine's own raster backend
and `raster.Diff`" - but neither test exists and `raster.Diff` has no caller. A
layout regression today is caught only by a targeted unit test, or not at all.

## What Changes

- Add a **document snapshot**: the prepared document model - elements with their
  computed styles, `@page` setup, margin boxes, `@font-face` declarations,
  metadata and diagnostics - serialized as deterministic XML and committed as a
  golden. It is an internal package; no public API and no CLI surface.
- Add **render snapshots**: every page of the produced PDF rasterized with
  poppler at a fixed resolution and compared pixel by pixel with a committed
  PNG. `raster.Diff` gains its first caller.
- Add fixtures that exercise every supported element and property, plus an
  invariant test that fails when a profile entry has no fixture.
- Add maintenance rules for both layers: `-update` regeneration, detection of a
  missing or stale golden, and failure artifacts (actual, expected and diff
  image; changed-pixel count and bounding box).
- Update CI: pin the runner image, keep poppler required, upload the failure
  artifacts.
- Correct the README claims and document both layers, the XML format, the
  renderer version and the update procedure.

## Capabilities

### New Capabilities

- `golden-tests`: canonical document snapshots and page render snapshots, their
  update and staleness rules, coverage of the CSS profile by the fixture corpus,
  and failure reporting.

### Modified Capabilities

None. The change adds test infrastructure; the product behaviour of the
converter is unchanged.

## Impact

- **Code**: new `internal/docdump` package (XML serializer for the prepared
  model); new tests in `internal/engine`; one new caller of `raster.Diff`. No
  production behaviour change.
- **Test data**: XML goldens for the corpus and the new fixtures; PNG goldens for
  the corpus and the visual fixtures (roughly 0.5-1 MB); a set of small
  profile fixtures and one small test image.
- **CI**: `.github/workflows/ci.yml` pins the runner image and uploads snapshot
  artifacts when the test job fails. Poppler and pdfcpu stay required exactly as
  they are now.
- **Docs**: new `docs/golden.md`; the README testing section stops claiming
  snapshots that do not exist and documents the real commands.
- **Dependencies**: none. The module stays dependency-free and keeps no
  `go.sum`.
