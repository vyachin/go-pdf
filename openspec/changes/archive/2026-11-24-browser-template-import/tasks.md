# Tasks

## Profile and engine prerequisites

- [x] Add `main` to the supported elements and the user-agent stylesheet.
- [x] Add physical `margin-*` and `padding-*` longhand properties.
- [x] Support the adjacent sibling selector `+` and the `:root` pseudo-class.
- [x] Accept base64 PNG/JPEG `data:` URI images in `<img src>` and `background-image`.
- [x] Decode and cache `data:` URI images in the image loader with the existing budgets.
- [x] Load the bold and italic sibling files of the first available system fallback family.

## Browser import

- [x] Add `internal/browser` with the lowering pipeline: strict validation first, import on errors, revalidation.
- [x] Drop non-print `@media` rules and report `browser/media`.
- [x] Resolve `var()` from document-level custom properties.
- [x] Lower `rem`, `vw`, `vh`, `vmin`, `vmax` and whole-value `min()`/`max()`/`clamp()`.
- [x] Lower `background: <colour>` to `background-color`.
- [x] Lower a flex column to block layout with item wrappers and preserved vertical margins.
- [x] Lower an absolutely positioned cover image to a background.
- [x] Lower `box-sizing: border-box` for explicit dimensions.
- [x] Apply `text-transform` to the text and report it.
- [x] Drop cosmetic and replaced flex properties, reporting each drop.
- [x] Reject flex rows, unsupported alignments, non-image absolute positions and unsupported text transforms.

## Integration

- [x] Add `engine.Options.ImportBrowser` and `pdfgen.Options.ImportBrowser`.
- [x] Enable the import for `pdfgen convert`, disable it for `-strict` and `validate`.
- [x] Keep the original profile diagnostics when the import cannot help.

## Tests and fixtures

- [x] Cover longhands, `data:` URI images, `+` and `:root` in package tests.
- [x] Add the browser certificate fixture and the end-to-end acceptance test.
- [x] Add the CLI test for the default import and `-strict`.
- [x] Add the certificate render golden and the profile-clean visual fixture.
- [x] Add the longhands fixture and its document snapshot.

## Documentation

- [x] Document the lowering, limits and diagnostics in `docs/browser-import.md`.
- [x] Update `docs/profile.md` for `main`, longhands, `+`, `:root` and `data:` URIs.
- [x] Update `docs/fonts.md` for the fallback family variants.
- [x] Update `README.md` for the browser import and the `-strict` behaviour.
