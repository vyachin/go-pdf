# Tasks

## 0. Behavioral invariant tests before implementation

- [x] 0.1 Strengthen `TestFaceCacheDistinguishesFamilyLists` with exactly
  `font-family: "a b"` and `font-family: a, b`, without the extra fallback family
  that currently avoids the layout-key collision. Use distinct bundled regular
  and bold fonts and test both request orders with cold and warm caches through
  layout and the registry. Verify each request resolves to its expected font,
  not merely that two font objects appear in the PDF; the current layout cache
  must fail this reproducer.
- [x] 0.2 Add a fixed request-permutation matrix spanning family lists, weight,
  italic and size with explicit font inputs. Compare the resolved font and
  metrics for each request signature with its isolated-resolution baseline,
  and inspect the font assigned to labelled text in end-to-end output. Verify
  cache warming cannot change a request's result; compare whole PDF bytes only
  for repeated identical documents, since reordered text changes the document.

## 1. Font resolution

- [x] 1.1 Remove the implicit `DefaultFallbackFonts` lookup from `pick`/`systemFaces`: with no declared family, no `FallbackFont` and no `FallbackFonts`, return a `font/unavailable` error naming the requested families and both options. Verify with `Options{}` and a document that declares no font: no PDF, one `font/unavailable`, and no host font opened (check with a missing syscall trace or an unreadable candidate path).
- [x] 1.2 Add `Options.FallbackFonts []string` and load the first parseable file as the fallback family, discovering bold/italic siblings next to it; keep `FallbackFont` tried first. Verify with two list entries where the first does not exist, and with an explicit single path, that the selected face and its siblings match today's `FallbackFont` behaviour.
- [x] 1.3 Replace the exported `DefaultFallbackFonts` slice with `pdfgen.SystemFallbackFonts()` returning a fresh copy of the conventional paths, and delete the mutable package state. Verify a caller that truncates the returned slice does not change a second call or a concurrent conversion (`-race`).

## 2. Public API, command and tests

- [x] 2.1 Add `Options.FallbackFonts` and `SystemFallbackFonts()` to `pdfgen` with godoc that calls the list host-dependent and opt-in, and map the new field into `engine.Options` and the mirror test. Verify `TestExportedDocs` and the options-mirror test pass.
- [x] 2.2 Make `cmd/pdfgen` pass `FallbackFonts: pdfgen.SystemFallbackFonts()`, and verify its tests still convert a template with no declared font. Verify the CLI help or documentation states that this is an example opt-in, not the library default.
- [x] 2.3 Update every engine, pdfgen and layout test that relied on a host font to pass `testdata/fonts/DejaVuSans.ttf` explicitly, and add a test asserting that a conversion without a font fails with `font/unavailable`. Verify the full suite and `-race` are green on a host with and without the four conventional paths.

## 3. Documentation and release

- [x] 3.1 Rewrite the "Family resolution order" section of `docs/fonts.md`: `@font-face`, then the caller's `FallbackFont`/`FallbackFonts`, then `font/unavailable`; describe `SystemFallbackFonts()` as an explicit, host-dependent opt-in. Verify the document no longer calls system fallback part of the default and the README's determinism section matches it.
- [x] 3.2 Add the `CHANGELOG.md` **BREAKING** entry and migration section for v3.0.0, naming the removed implicit search and the two ways to restore rendering. Verify the entry names `FallbackFont`, `FallbackFonts` and `SystemFallbackFonts()`.
- [x] 3.3 Update the package documentation and the README example to pass an explicit font, and grep the repository for `DefaultFallbackFonts` to confirm no reference remains. Verify `go doc github.com/vyachin/go-pdf/pdfgen` states the resolution order.
