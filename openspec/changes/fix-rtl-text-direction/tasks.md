# Tasks

## 1. Bidi ordering and base direction

- [ ] 1.1 In `internal/text`, add a bidi pass that takes the logical line text and a base direction and returns the visual run order, using `golang.org/x/text/unicode/bidi`. Verify with unit tests: a pure Hebrew line returns the reversed run order, a pure Latin line returns the input order, and a line with only neutrals returns the base direction's order.
- [ ] 1.2 Resolve the effective base direction from the nearest `dir` attribute (`ltr`/`rtl`/`auto`) in the box builder, falling back to the first strong character. Verify with fixtures: `<p dir="rtl">abc</p>`, `<p dir="auto">שלום</p>`, `<p dir="ltr">שלום</p>` and a nested `dir` that shadows its parent.
- [ ] 1.3 Apply the visual run order in `internal/layout` when a line is placed, keeping shaping and kerning logical, and mirror the same placement for page margin boxes. Verify with a Hebrew paragraph: glyph x-positions in the content stream decrease for the first logical word, and `pdftotext` returns the logical sequence.
- [ ] 1.4 Verify mixed direction: `abc שלום 123` keeps Latin runs left-to-right, reverses only the Hebrew run, keeps digits in left-to-right order, and loses no glyph. Verify justification still spreads spaces on an RTL line. Verify a Latin-only document is byte-identical to the parent commit.

## 2. Unsupported joining scripts

- [ ] 2.1 Add the `text/unsupported-script` code to `internal/diag` (registry, meaning, catalogue) and a script detector for Arabic, Syriac, N'Ko and Indic ranges. Verify the registry test (`TestEveryEmittedCodeIsRegistered`, `TestEveryCodeHasAMeaning`, `TestRegisteredCodesAreUnique`) passes and no literal code is needed outside the constant.
- [ ] 2.2 Report an error at the owning element position when a run needs joining, and keep Hebrew, Cyrillic, Greek and Latin accepted. Verify through both entry points: `Convert` returns the code, `Validate` returns the same code, and no PDF is produced; a Hebrew document converts cleanly.
- [ ] 2.3 Register an adversarial regression fixture (Arabic paragraph with a small `MaxOperations`) that asserts the diagnostic code and a bounded outcome, and add it to `internal/engine/adversarial_test.go`. Verify `go test ./internal/engine -run TestAdversarial` passes and `TestAdversarialManifest` accepts the new fixture.

## 3. Documentation and release note

- [ ] 3.1 Add a script-support section to `docs/profile.md`: bidi reordering and `dir` supported; Hebrew and other non-joining RTL scripts supported; joining scripts refused with `text/unsupported-script`; mirrored punctuation not substituted; hyphenation and CJK unchanged. Verify the wording matches the detector table.
- [ ] 3.2 Add a documentation test that compares the profile's script list with the detector the engine uses, in the style of the existing profile-document tests. Verify changing one side alone fails the test.
- [ ] 3.3 Add an `Unreleased` / `### Fixed` entry to `CHANGELOG.md` stating that right-to-left runs are reordered and that joining scripts are refused with the named diagnostic. Verify the entry names no removed API.
- [ ] 3.4 Run `gofmt -l`, `go vet ./...`, `go test -shuffle=on ./...`, `go test -race ./internal/text ./internal/layout ./internal/engine` and `openspec validate fix-rtl-text-direction --strict`. Verify all pass and the Latin render snapshots are unchanged.

## Workflow follow-up

- Sync the delta into `openspec/specs/text-shaping/spec.md` and
  `openspec/specs/library-docs/spec.md`, then archive the change after review.
