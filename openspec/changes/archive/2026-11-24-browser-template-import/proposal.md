# Proposal

## Why

Real browser-oriented documents, such as the gift-certificate template exported
from the certificate service, use CSS the profile deliberately rejects:
flexbox, absolute positioning, `rem` units, CSS variables, screen media queries
and `data:` URI images. Requiring every author to rewrite such a document by
hand defeats the converter's purpose. At the same time, widening the renderer
into a browser engine would break the deterministic, checkable profile that the
rest of the project depends on.

## What Changes

- Add an opt-in **browser import** layer that lowers a bounded set of browser
  constructs to the profile before validation, on the original document.
- Keep the strict profile unchanged: the import runs only after the strict
  validation failed, and only when the caller enables it. `pdfgen convert`
  enables it by default; `pdfgen convert -strict` disables it.
- Extend the profile with the constructs the lowering needs: `main`, physical
  `margin-*` and `padding-*` longhands, the adjacent sibling selector `+`,
  the `:root` pseudo-class, and base64 PNG/JPEG `data:` URI images for `<img>`
  and `background-image`.
- Extend the system font fallback with the bold and italic sibling files of the
  first available family, so a `font-weight: 700` title is not printed regular.
- Add the certificate as an acceptance fixture and a render golden from its
  profile-clean equivalent.
- **BREAKING** for the CLI: `pdfgen convert` now retries a rejected document
  through the browser import instead of failing immediately. `-strict` keeps
  the old behaviour, and `pdfgen validate` never imports.

## Capabilities

### New Capabilities
- `browser-import`: the opt-in lowering of a bounded browser CSS subset to the
  deterministic profile, with a warning for every transformation and an error
  for every construct that cannot be lowered.

### Modified Capabilities
- `css-profile`: `main` becomes a supported element; physical box longhands,
  `+`, `:root` and `data:` URI background images become part of the profile.
- `pdf-output`: `<img src>` accepts base64 PNG/JPEG `data:` URIs.

## Impact

- New package `internal/browser`.
- `engine.Options.ImportBrowser` and `pdfgen.Options.ImportBrowser`.
- `internal/css`: selectors, longhand properties, background sources.
- `internal/engine`: image loader, font fallback, preparation pipeline.
- `cmd/pdfgen`: `convert` default and `-strict` semantics.
- `testdata/browser`, `testdata/fixtures`, `testdata/fonts`, render and document
  goldens.
- Documentation: `README.md`, `docs/profile.md`, `docs/fonts.md`, new
  `docs/browser-import.md`.
