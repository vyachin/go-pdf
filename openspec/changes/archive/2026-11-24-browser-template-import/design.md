# Design

## Context

`certificate.html` is a browser document. The strict profile rejects it with 46
diagnostics: flexbox, absolute positioning, CSS variables, `rem`, screen media
queries, `data:` URIs, `box-sizing` and cosmetic properties. The renderer must
stay small and deterministic; a browser engine is out of scope.

The chosen direction is lowering (the "browser import"): the converter rewrites
the document in memory to the profile, then runs the existing validation,
layout and PDF writer. The alternative, natively implementing flexbox and
absolute positioning in the layout engine, would take several changes and would
erode the closed-allow-list contract.

## Pipeline

```
strict validation ── ok ──> layout ──> PDF
        │
      errors
        │  ImportBrowser
        v
browser.Import: media → variables → units → DOM lowering → property cleanup
        │
        v
strict validation ── ok ──> layout ──> PDF (with browser/* warnings)
```

The import runs before the cascade that layout uses. It mutates the parsed
`html.Document` and the collected `css.Stylesheet` values in place; there is no
HTML serialization round trip. A valid template never reaches the import, so
existing documents and goldens are unaffected.

## Lowering rules

### Values

Media rules that cannot apply to print are removed. Custom properties declared
on the document element are substituted for `var()`; `rem` resolves against the
root font size, `vw`/`vh`/`vmin`/`vmax` against the page area, and a whole-value
`min()`, `max()` or `clamp()` becomes a single length. `background: <colour>`
becomes `background-color`.

### Flex column

A flex container with `flex-direction: column` becomes a block. Every element
child is wrapped in a `div` that carries the child's vertical margins as
`padding-top`/`padding-bottom` and has `text-align: center` when
`align-items: center`. The child's own margins are zeroed. This preserves the
flex spacing without relying on margin collapsing, which behaves differently in
flex and block layout. A row, wrapping, `flex-grow` or a non-`flex-start`
justification is rejected with `browser/unsupported-flex`.

### Absolute cover image

An absolutely positioned `<img>` is lowered only when it covers its containing
block (`inset: 0` or all four offsets zero), has `object-fit: cover`, and the
positioned ancestor has an explicit height. The image is removed and the
ancestor receives `background-image` and `background-size: cover`. The
background is painted behind the content, which reproduces `z-index: -1`
without a stacking-context model. Anything else positioned absolutely is
rejected with `browser/unsupported-position`.

### Box sizing

The renderer uses the content-box model. For an element with
`box-sizing: border-box` and an explicit width or height, the padding and border
are subtracted from the content size, so the border box keeps its declared size.
The CSS property itself is then dropped. This keeps a full-page certificate at
exactly 297 mm even with its bottom padding.

### Text and leftovers

`text-transform` is applied to the text nodes and the property is dropped.
Properties that only describe the screen and have no profile meaning
(`box-shadow`, `isolation`, `overflow`, `z-index`, `aspect-ratio`, `min-height`,
`print-color-adjust`, `color-scheme`) are dropped. Every drop is a
`browser/dropped-property` warning.

## Profile and engine additions

- `main` is a block element in the profile and the UA stylesheet.
- Physical margin and padding longhands are first-class properties.
- The adjacent sibling combinator `+` and the `:root` pseudo-class are
  supported selectors; `:root` matches the document element.
- `background-image` accepts base64 `data:image/png` and `data:image/jpeg`.
- The image loader decodes the same `data:` forms for `<img src>` and caches
  them like file images, so a repeated URI is embedded once.
- The system fallback loads the first available regular face plus the
  conventional bold/italic sibling files. This is the only way a template
  without `@font-face` can print a 700 title bold without bundling a font.

## Decisions

- **Strict contract preserved.** The import is not a relaxed validator: the
  rewritten document must pass the same checks. The CLI enables it for
  `convert`; `-strict` and `validate` keep the original diagnostics.
- **Warnings, not silence.** Every approximation names the property and its
  source position; unsupported constructs are errors.
- **Bounded subset.** Only the constructs the certificate family needs are
  lowered. Remote URLs stay rejected (no network, determinism) and SVG stays
  rejected (no vector renderer); both are documented with the rasterize-first
  workflow.
- **No native flex or positioning.** A general implementation would be a large
  layout change with pagination consequences; the lowering is adequate for the
  single-page certificate and reversible if native support arrives later.

## Risks

- A lowered flex layout is only equivalent for a single-line, top-aligned
  column with centred items; a row or wrapped layout fails loudly instead of
  producing a wrong page.
- System fallback fonts vary per machine. Documents that need byte-identical
  output must declare `@font-face`; the golden fixtures do.
- The CLI default changes behaviour for invalid templates. `-strict` is the
  documented escape hatch and `validate` is unchanged.
