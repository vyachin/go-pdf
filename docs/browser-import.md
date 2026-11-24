# Browser import

Documents exported from design tools, and certificate-like templates, often use
CSS the profile deliberately rejects: flexbox, absolute positioning, `rem`
units, CSS variables, screen media queries and `data:` URI images. The browser
import is a compatibility layer that **lowers** those documents to the profile
before validation. It is not a browser and it does not relax the profile: the
rewritten document must pass the same validator, and every approximation is
reported.

The strict profile contract is unchanged. The import only runs when the strict
validation already failed and the caller opted in.

## Turning it on

| Interface | Behaviour |
|---|---|
| `pdfgen convert` | on by default; a valid profile template never reaches the import |
| `pdfgen convert -strict` | off; the original profile diagnostics are reported |
| `pdfgen validate` | always off; it checks the template as written |
| `pdfgen.Options.ImportBrowser` | explicit switch for library users, off by default |
| `engine.Options.ImportBrowser` | same switch inside the engine |

```bash
pdfgen convert certificate.html          # imports and writes certificate.pdf
pdfgen convert -strict certificate.html  # profile diagnostics, no PDF
```

## What is lowered

| Browser construct | Lowering |
|---|---|
| `display: flex` with `flex-direction: column` | block layout; every flex item is wrapped in a block that carries its vertical margin as padding, so the items keep the flex spacing and do not collapse |
| `align-items: center` | `text-align: center` on the container and on the item wrappers |
| `position: absolute` image with `inset: 0` (or all offsets zero) and `object-fit: cover` | the image is removed and becomes `background-image` + `background-size: cover` of the positioned ancestor |
| `rem` | `px` at the root font size (`html { font-size }`, 16px when absent) |
| `vw`, `vh`, `vmin`, `vmax` | `px` against the printed page area |
| whole-value `min()`, `max()`, `clamp()` | a single length against the page area |
| `var(--name[, fallback])` | the custom property registered on the document element |
| `@media` that cannot apply to print | dropped |
| `background: <colour>` | `background-color` |
| `text-transform: uppercase`, `lowercase`, `capitalize` | applied to the text |
| `box-sizing: border-box` with an explicit width or height | padding and border are subtracted from the content size, so the border box keeps its declared size |
| cosmetic properties (`box-shadow`, `isolation`, `overflow`, `z-index`, `aspect-ratio`, `min-height`, `print-color-adjust`, `color-scheme`) | dropped |

`<main>` is part of the profile, and `:root` and the adjacent sibling selector
`+` are supported selectors, so documents that use them do not need lowering.

## Diagnostics

Every transformation is a warning on the declaration that caused it:
`browser/flex`, `browser/background`, `browser/dropped-property`,
`browser/box-sizing`, `browser/text-transform`, `browser/media`. The CLI prints
them next to the input path, so a reviewer can see exactly what changed.

Constructs that cannot be lowered are errors and stop the conversion:

| Code | Meaning |
|---|---|
| `browser/unsupported-flex` | a flex direction other than `column`, or `justify-content` other than `flex-start` |
| `browser/unsupported-position` | absolute positioning on anything except a box-covering `<img>`, or a missing positioned ancestor / explicit ancestor height |
| `browser/unsupported-text-transform` | a `text-transform` value other than the three listed above |

## Limits

- Only a **single-line flex column** is lowered. Rows, wrapping, `flex-grow`,
  `justify-content: center` and gaps are rejected rather than approximated.
- Only a **full-box cover image** is lowered from absolute positioning. Text or
  boxes positioned absolutely must be rewritten by hand.
- A **percentage margin** on a flex item cannot be resolved during lowering.
- `data:` URI images must be base64 PNG or JPEG. SVG is not decoded; rasterize
  it to PNG first. Remote URLs are still rejected: the engine never touches the
  network.
- The font stack is not resolved file by file. A document without `@font-face`
  uses `Options.FallbackFont`, the system font from `DefaultFallbackFonts` (with
  its bold and italic siblings), or fails with `font/unavailable`. Ship the
  fonts and declare `@font-face` when byte-identical output matters.

## Fonts and the system family

The system fallback now loads the regular file and looks for the conventional
bold and italic siblings next to it: `Arial Bold.ttf`, `DejaVuSans-Bold.ttf`,
`LiberationSerif-Bold.ttf`. This keeps a 700 title bold on a machine where the
template declares no `@font-face` instead of silently printing it regular.

## Acceptance fixture

The `testdata/browser/certificate.html` fixture is the browser certificate that
the import must accept without edits. `TestBrowserCertificateImport` checks that it
converts to one A4 page with the expected text and images, that the import
reports the flex and background lowerings, that the plain profile still rejects
the file, and that repeated imports are byte-identical. The certificate also has
a render golden, produced from a profile-clean equivalent:
`testdata/fixtures/visual-certificate.html`.
