# Proposal

## Why

When a document declares no `@font-face` family and the caller sets no `FallbackFont`, the
font registry tries four hard-coded system paths and uses the first one that exists: Arial
on macOS, DejaVu Sans or Liberation Serif on Linux. The same HTML, stylesheet and options
therefore produce different glyphs, different PDF bytes and different line breaks on two
hosts, which contradicts the package's deterministic-output guarantee in its default
configuration. `DefaultFallbackFonts` is also an exported mutable slice read on the
conversion path, so any in-module caller or parallel test can change the typography of a
concurrent conversion. The README documents the caveat, but a caveat on the default path
is a broken guarantee.

## What Changes

- **BREAKING:** remove the implicit system-font search. With no matching `@font-face` and no
  caller-selected font, the conversion fails with `font/unavailable`, and no installed host
  font is read.
- Keep `Options.FallbackFont` (one explicit path) and add `Options.FallbackFonts []string`
  as an ordered explicit list. Both are caller-selected bytes, so both are deterministic.
- Replace the exported mutable `DefaultFallbackFonts` slice with
  `pdfgen.SystemFallbackFonts() []string`, which returns a fresh copy of the conventional
  platform paths for callers who choose the non-reproducible behaviour explicitly.
- Update the reference command to opt in with `FallbackFonts: pdfgen.SystemFallbackFonts()`,
  so `pdfgen invoice.html` keeps working on a developer machine while the library default
  is deterministic.
- State the resolution order in `docs/fonts.md` and the README: `@font-face`, then the
  caller's fallback, then `font/unavailable`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the zero-options and determinism requirements change: a zero `Options`
  no longer reads a host font, and a missing font is a documented failure rather than an
  environment-dependent success.

## Impact

- `internal/engine/fonts.go` (`DefaultFallbackFonts`, `loadSystemFamily`, `Face`, `pick`),
  `internal/engine/engine.go`, `pdfgen/pdfgen.go` (`Options`, new export),
  `cmd/pdfgen/main.go`.
- Public API: additive `Options.FallbackFonts` and `SystemFallbackFonts()`; removed
  implicit behaviour. **BREAKING** — the next release is a major version (v3.0.0).
- `docs/fonts.md`, `README.md` (Remove the caveat and state the rule), `CHANGELOG.md`
  migration section.
- Tests: font resolution and fallback-limit tests, `pdfgen` option tests, CLI tests; every
  test that relied on a host font must pass an explicit path.
