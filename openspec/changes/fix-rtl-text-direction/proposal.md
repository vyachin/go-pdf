# Proposal

## Why

Hebrew and Arabic text is shaped in logical order and painted left to right, so a
right-to-left sentence appears reversed. With the bundled font, `שלום עולם` renders
as `םלוע םולש`; Arabic is also unjoined. The conversion reports zero diagnostics.
The engine has no bidi pass and no GSUB support, while
`golang.org/x/text/unicode/bidi` is already an allowed dependency. A business
document in Hebrew or Arabic is silently wrong today, which contradicts the
project's rule that text is never silently misplaced.

## What Changes

- Reorder the runs of every line into visual order per UAX#9, using
  `golang.org/x/text/unicode/bidi`. The base direction comes from the nearest
  `dir` attribute (`ltr`, `rtl`, `auto`) or from the first strong character.
  Shaping stays in logical order; only placement is reordered, so kerning inside a
  run is preserved.
- Keep the text layer in logical order, so copied text returns the authored
  sequence rather than the visual one.
- Detect scripts whose shaping needs GSUB joining (Arabic, Syriac, N'Ko, Indic)
  and refuse them with the error diagnostic `text/unsupported-script` instead of
  painting unjoined or reversed text. Hebrew needs reordering but no joining, so
  it becomes supported.
- Document the script capability table in `docs/profile.md`: reordering and `dir`
  supported; joining scripts refused with the new code; mirrored punctuation not
  substituted; hyphenation and CJK line breaking unchanged.
- Give the `dir` attribute, already accepted by the profile, its documented
  meaning; today it is accepted and ignored.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `text-shaping`: visual-order layout for right-to-left runs, base-direction
  selection, and a diagnostic for scripts that require GSUB.
- `library-docs`: the profile document states which scripts and shaping
  capabilities are supported and which are refused.

## Impact

- `internal/text` (bidi ordering and script detection), `internal/layout`
  (visual placement of runs and margin boxes), `internal/html`/`internal/layout`
  (read the effective `dir`), `internal/diag` (`text/unsupported-script`).
- `docs/profile.md`, `CHANGELOG.md`.
- Tests: Hebrew order and mixed-direction fixtures, `dir` precedence, Arabic
  refusal through both entry points, and byte-identical output for Latin-only
  documents. Render snapshot expectations for existing Latin documents do not
  change.
- No exported Go API change. Output for documents containing right-to-left text
  changes because it was wrong before; this is a fix, not a contract change, so
  the release is not marked breaking.
