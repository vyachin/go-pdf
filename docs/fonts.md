# Fonts: decision and resolution order

## Decision

The engine **does not ship fonts in the binary**. Fonts are part of the document or of the run configuration.

## Why not `go:embed`

- **Licenses.** PT Serif, PT Sans and Liberation are distributed freely, but their redistribution terms differ, and Times New Roman and Arial cannot be redistributed at all. Embedding a font in the binary is distributing it.
- **Size.** One family in four faces is 1.5–3 MB. A full set for documents (serif, sans-serif, mono) easily adds 8–10 MB to the binary.
- **Reproducibility.** Byte-identical PDFs depend on the exact font version. Keeping the font in the same artifact as the documents is more reliable than depending on the system font.

For an integrator this means: the service that embeds the engine decides for itself how to deliver the font — through `go:embed` in its own binary, through a file in the container or through a volume with the fonts.

## Family resolution order

1. `@font-face` in the document — an exact family match, then the closest by weight and slant.
2. `Options.FallbackFont` — a path to a TrueType file, used for any family that is not declared.
3. System fonts from `DefaultFallbackFonts` — the first available family, together with the conventional bold and italic files next to its regular face (`Arial Bold.ttf`, `DejaVuSans-Bold.ttf`, `LiberationSerif-Bold.ttf`). This is a convenience for development, not for production: the set of system fonts is not controlled.
4. The `font/unavailable` error — if nothing is found.

The pseudo-families `serif`, `sans-serif` and `monospace` without a declared `@font-face` resolve through step 2 or 3.

## File requirements

- **TrueType** only (`.ttf`). CFF/OpenType, WOFF and WOFF2 are rejected during validation: their outlines require a separate interpreter.
- Cyrillic is mandatory: a character without a glyph is the error `text/glyph`, not a silent replacement.
- `@font-face` accepts only a local `url(...)`; `format()` and remote URLs are rejected.

## Recommended set for Russian documents

| Role | Font | License |
|---|---|---|
| Body text | PT Serif | ParaType Free Font License |
| Headings | PT Sans | ParaType Free Font License |
| Office compatibility | Liberation Serif / Sans | SIL OFL |
| Wide coverage | DejaVu Sans | free |

## Verification

```go
// FontDir is the directory relative to which @font-face paths resolve.
opts := pdfgen.Options{FontDir: "fonts"}

// Validate checks that the document references only local TrueType fonts.
diags, err := pdfgen.Validate(ctx, src, opts)

// Convert renders with the same resolution rules.
res, err := pdfgen.Convert(ctx, src, opts)
```

The reference command sets `FontDir` to the input file's directory, so a font
placed beside the document is found without any configuration.
