# Tasks

## 1. Default root and boundary

- [ ] 1.1 Derive the asset root from the configured kind root, then `BaseDir`, then `filepath.Dir(Options.File)`, then the working directory, and open a bounded `fsx.Root` for each case. Verify with a table test that the same relative reference resolves under each of the four configurations.
- [ ] 1.2 Confine relative references to that root, including `../` and a symlink that leaves it. Verify with a test for each asset kind; the escape reports the resource's load diagnostic and no PDF.
- [ ] 1.3 Keep the engine's document name and the root selection separate: `Options.File` stays the diagnostic name while its directory is used only as the default root. Verify that `File: "<stdin>"` still resolves relative assets in the working directory.

## 2. Absolute reference policy

- [ ] 2.1 Add `AllowAbsoluteAssets bool` to `pdfgen.Options` and its engine mirror, defaulting to false, and plumb it into the asset request. Verify a zero `Options` still converts a document with adjacent assets and that the public docs test passes.
- [ ] 2.2 Accept an absolute reference inside a configured root and reject one outside it for images, stylesheets and fonts. Verify the codes: `img/load`, `css/link-path`, `font/unavailable`, and that the rejected file is not embedded.
- [ ] 2.3 Read an absolute reference as written when the opt-in is set and no root is configured. Verify a conversion with an absolute image, stylesheet and font succeeds only with the option set.
- [ ] 2.4 Keep `FallbackFont` explicit: it is read as written regardless of the option. Verify with a test that a document reference and the fallback font follow different rules.

## 3. Documentation and release note

- [ ] 3.1 Update the `BaseDir`/`FontDir` godoc, the README asset guarantee and the family-resolution section of `docs/fonts.md` to state the root precedence, the relative-only rule and the opt-in. Verify each sentence has a matching test in this change.
- [ ] 3.2 Add a `Changed`/breaking entry to `CHANGELOG.md` naming the absolute-reference default and the `File`-directory root, with the migration line. Verify the entry names `AllowAbsoluteAssets` and no removed identifier.

## Workflow follow-up

- Run `openspec validate fix-default-asset-confinement --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/library-api/spec.md` and archive the change after review.
