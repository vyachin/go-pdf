# Design

## Context

See `proposal.md` — Why. The pipeline is parse → cascade → document snapshot →
layout (pagination and paint ops) → PDF writer, and the two golden layers freeze
the snapshot and the rendered page (`docs/golden.md`).

Constraints that shape the approach:

- The profile is a closed allow-list. `Style` is a flat struct of computed
  values, `docdump` serialises it, and `TestProfileCoverage` fails when a
  property in `css.PropOrder` is declared by no fixture.
- `ImageOp` paints a decoded image into a `Rect`; the writer emits
  `q W 0 0 H X Y cm /Im Do Q` and embeds JPEG data as `DCTDecode` without
  recompression. There is no clip and no source rectangle.
- Images already go through `imageLoader`: confined to `BaseDir`, byte and pixel
  budgets, decoded once and cached for the conversion.
- Vertical centering exists as `vertical-align: middle` inside a `table-cell`;
  `height` is honoured only as an absolute length (`paginate.go`), so a
  percentage height silently does nothing. The fixture therefore uses `297mm`.
- Render goldens are pixel-exact and tied to poppler 24.02.0 (Ubuntu 24.04),
  recorded in `testdata/golden/render/renderer.txt`.
- The source photograph is a 4175x6263 progressive JPEG, 3.6 MB — within the
  40 MP and 32 MB limits; its committed weight is accepted.

## Goals / Non-Goals

**Goals:**

- A box can carry a local photograph as its background, cover-scaled,
  center-cropped and clipped, painted behind the box's content.
- Both properties are visible in the document snapshot and the profile listing,
  and rejected values fail validation like any other out-of-profile value.
- A one-page cover fixture freezes the feature in both golden layers, with the
  render golden produced by the pinned renderer.

**Non-Goals:**

- `background-position`, `background-repeat`, multiple backgrounds, gradients,
  `contain`, `auto` and explicit sizes — deferred until a document needs them.
- Page-level backgrounds (`@page { background-image }`) and `@page:first`.
- Overlays, opacity or text shadows for contrast; the fixture picks a text
  colour that reads on the photograph.
- Any change to `<img>` sizing or painting.

## Decisions

**1. The background belongs to a box, not to the page.** A normal property
composes with the existing full-page `table-cell` trick and stays standard CSS.
A page-level property would repeat on every page and there is no `@page:first`
to scope it to the cover. Alternative considered: `@page { background-image }` —
rejected for the repetition and the non-standard grammar.

**2. `cover` implies centering; `background-position` is not added.** One
documented semantics — scale to cover the padding box, split the overflow
equally between opposite edges — costs one keyword and no second property.
Alternative: support `background-position` in v1 — rejected; it doubles the
value grammar for a case no fixture exercises.

**3. Clip in the PDF writer instead of cropping pixels.** `ImageOp` gains an
optional clip rectangle and the writer wraps the draw in `q <rect> re W n ... Q`.
This keeps the JPEG a `DCTDecode` passthrough. Alternative: decode, crop and
re-encode the image — rejected, because it would recompress a photograph that
the engine promises to embed as-is, and the writer already has no pixel path for
JPEG.

**4. Paint the background where `background-color` is painted today.** The
image op is inserted at the fragment's start in `decorationOps`, so it stays
behind text and after nothing else. Geometry (scale, centering, clip) is
computed from the fragment rect, which makes a full-page cell and an ordinary
box the same case.

**5. Reuse `imageLoader` for the background.** Same root confinement, same
budgets, same cache and the same diagnostics as `<img>`. A missing file is
reported during layout, before the writer runs.

**6. The snapshot records the declarations, not the geometry.** `docdump` writes
`background-image` (the source string as written) and `background-size` (the
keyword). The resolved scale and crop are layout results and are frozen by the
render golden, not duplicated in XML.

**7. Fixture geometry.** `@page { size: A4; margin: 0 }`, body margin 0, a table
210 mm wide, its single cell 297 mm high with padding 0,
`vertical-align: middle`, `text-align: center`, and an `h1` at 84 pt, weight
700, colour `#1c2b3a`. If row placement treats an exactly page-high row as
overflow, the cell becomes 296.9 mm — 0.28 px at 72 dpi, invisible — because the
fixture must validate without warnings.

**8. Render golden from the pinned toolchain.** Generate the PNG inside
`ubuntu:24.04` with poppler 24.02.0 so `renderer.txt` stays valid and CI's
strict check passes. Alternative: Homebrew poppler on macOS — a different
version, which CI reports as a renderer mismatch.

**9. Commit the original photograph.** The user's 3.6 MB, 4175x6263 file is
copied into `testdata/fixtures/` as the fixture's image. The render golden is
only 595x842 at 72 dpi, but the PDF keeps the print resolution.

## Risks / Trade-offs

- **Progressive JPEG through `DCTDecode`** — `jpegInfo` accepts any SOF, but PDF
  validators and viewers are happiest with baseline. → Verify with
  `pdfcpu validate -strict` and poppler; if rejected, re-encode the same
  4175x6263 pixels as a baseline JPEG (no visible change) and note it in the
  fixture.
- **Exact page-height fit** — a 297 mm row on a 297 mm page may be classified as
  overflow by `placeTable`. → Use 296.9 mm if a warning appears; never commit a
  fixture that validates with a warning.
- **Cover semantics diverge from CSS** (implicit centering) — a future
  `background-position` would refine it. → Document the exact semantics in
  `docs/profile.md` now.
- **Repo weight** — `testdata/` grows by 3.6 MB. → Accepted; the JPEG is not
  decoded for layout (header only) and appears once in the PDF resource table.
- **Contrast depends on the photograph** — the profile has no shadows, scrims or
  opacity. → The fixture uses a dark text colour over the pale sky; a new
  photograph means a new fixture decision, not an engine feature.

## Migration Plan

No migration: both properties are new, so every existing template continues to
validate and render identically. Rollback is a revert; templates using the new
properties then fail validation again, as they did before.
