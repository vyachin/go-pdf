# Design

## Context

Family resolution has three steps: a declared `@font-face` family, `Options.FallbackFont`,
then the first existing path of `DefaultFallbackFonts` (Arial, Times New Roman, DejaVu
Sans, Liberation Serif). The third step makes the default output host-dependent, and
`DefaultFallbackFonts` is an exported slice that tests already mutate. The deterministic
guarantee therefore holds only when the caller selects a font, which the README documents
as a caveat rather than fixing.

## Goals / Non-Goals

**Goals:**

- A default conversion never reads a host font and is identical on every host.
- Host fonts remain available, but only when the caller names them explicitly.
- No mutable package-level state influences font selection.
- The reference command keeps working on a developer machine without making the library
  default non-reproducible.
- An actionable diagnostic when no font is available.

**Non-Goals:**

- Embedding a default font in the package. That changes typography for every existing
  document and adds 1.5–3 MB to every binary; it can be a later opt-in.
- Changing `@font-face` matching, weight/slant selection or font parsing.
- Changing `Options.FallbackFont`'s meaning; it stays a single explicit path.

## Decisions

**Fail instead of embedding a fallback.** An embedded font would make the default
deterministic but silently change the typography of documents that currently render with
Arial or DejaVu, and it would grow every consumer's binary. A `font/unavailable` error
names the requested families and the two options, so the migration is explicit. Alternative:
embed behind a build tag; deferred, not rejected.

**Two explicit options: `FallbackFont` first, then `FallbackFonts`.** `FallbackFont` is the
existing single path and keeps compiling; `FallbackFonts` is an ordered list for callers
who want a family plus siblings or a platform list. When both are set, `FallbackFont` is
tried first, then each list entry in order; the first file that parses becomes the family
and its conventional bold/italic siblings are discovered next to it, as today. Alternative:
drop `FallbackFont`; rejected because it removes a working option for no gain.

**`SystemFallbackFonts()` returns a copy; the list is a function-local constant.** No
package-level slice remains, so no caller or parallel test can change another conversion's
typography. The function documents that its result is host-dependent and belongs to an
explicit opt-in. Alternative: keep an unexported slice and a public accessor; the accessor
must still copy, so the constant is simpler.

**The CLI opts in, the library does not.** `cmd/pdfgen` is an example, not the versioned
surface; it passes `SystemFallbackFonts()` so a developer's one-file conversion still works.
The README example passes an explicit font, showing the reproducible path.

**The diagnostic names the fix.** `font/unavailable` for a missing family lists the
requested families and says to declare `@font-face` or set `FallbackFont`/`FallbackFonts`.
A document that previously rendered now fails with a message that names both remedies.

## Behavioral verification

In addition to host independence, preserve the existing unambiguous face-cache
contract (`text-shaping`, RV4-43) through request-order tests. The exact lists
`["a b"]` and `["a", "b"]` must be exercised through both layout and registry
caches: layout currently formats them with `%v`, while the registry uses `%q`.
Use explicit bundled fonts and an independent expected face for each request.
Test both orders, warm caches and a fixed matrix of weights, slants and sizes.
Compare results per request or labelled text, rather than expecting identical
PDFs from documents whose text order differs. This verifies the existing matching
policy; it does not introduce a new font-selection policy.

## Risks / Trade-offs

- **A breaking change for every caller that relied on system fonts.** That is the point:
  those callers had non-reproducible output. The release is a major version with a
  migration section, and the error names the option to set.
- **The CLI and the library default diverge.** Intentional and documented: the command is
  a development example; the library default is the guarantee.
- **Tests that used an empty `FallbackFont` now fail.** They are updated to pass the
  bundled `testdata/fonts/DejaVuSans.ttf`, which also makes them independent of the host.
- **A caller who wants the old behaviour must opt in.** One line:
  `FallbackFonts: pdfgen.SystemFallbackFonts()`; docs/fonts.md shows it next to the
  reproducibility warning.

## Migration Plan

Release as the next major version (v3.0.0). `CHANGELOG.md` gains a `Breaking` entry and a
migration section: callers that converted without a font must set `FallbackFont`,
`FallbackFonts` or an `@font-face`; callers that want the old platform search set
`FallbackFonts: pdfgen.SystemFallbackFonts()` and accept host-dependent output. `docs/fonts.md`
and the README replace the caveat with the resolution order and the deterministic rule. The
reference command opts in and its tests use the bundled font where reproducibility is
asserted. No stored data or diagnostic codes change; `font/unavailable` already exists.
