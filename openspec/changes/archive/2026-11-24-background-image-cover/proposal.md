# Proposal

## Why

Business documents built with the engine need a cover: a photograph filling the
page with the report title centered on it. The profile supports
`background-color` only, so a full-bleed cover cannot be expressed:
`background-image` is rejected as an unknown property, and an `<img>` cannot sit
behind text. The engine already embeds JPEG without recompression and the golden
suite already freezes rendered pages, so the missing piece is a small, explicit
CSS surface plus painting.

## What Changes

- The profile accepts two new properties on any box: `background-image:
  url(<local path>)` and `background-size: cover`. Every other value — a
  gradient, a `data:` or remote URL, `contain`, `auto`, a length pair — is
  rejected with its position, like any other out-of-profile value.
- Layout paints the background image behind the box's content, scaled to cover
  the box and center-cropped; pixels outside the box are clipped.
- The PDF writer gains an optional clip rectangle for an image operation. JPEG
  stays a `DCTDecode` passthrough.
- The document snapshot records both properties, so a change in the background
  is reviewable in the XML layer without rendering.
- A new visual fixture `testdata/fixtures/visual-hello-world.html`: one A4 page
  with no margins, a photograph covering the page, and "Hello world" at 84 pt
  centered horizontally and vertically. Committed with it: the 4175x6263 source
  JPEG, the document snapshot and the rendered page, the latter produced with
  the pinned poppler 24.02.0.
- Documentation: `docs/profile.md` gains the two properties with their
  semantics; `docs/golden.md` mentions the new fixture in the visual set.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: the box model property list gains `background-image` and
  `background-size`, with the exact accepted values and the rejection rule for
  everything else.
- `document-layout`: a new requirement that a background image is painted behind
  a box's content, covers the box and is clipped to it.

## Impact

- Code: `internal/css` (property specs, value parsing, computed style),
  `internal/docdump` (snapshot attributes), `internal/layout` (painting, cover
  geometry, image loading through the existing budgeted loader), `internal/pdf`
  (clip support for image operations).
- Data: `testdata/fixtures/` gains a 3.6 MB JPEG and an HTML fixture;
  `testdata/golden/xml/` and `testdata/golden/render/` gain one golden each.
- Docs: `docs/profile.md`, `docs/golden.md`.
- No public API change: the library and CLI surface stay as they are; the CSS
  profile grows.
- Repo weight: the fixture adds 3.6 MB to `testdata/`. Accepted: the image is
  embedded without recompression, and its resolution is what makes the cover
  look right in print.
