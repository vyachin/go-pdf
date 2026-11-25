# Proposal

## Why

The fourth review pass (recorded in `TODO.md`, "Fourth pass — November 25, 2026")
found 66 open items across the public contract, the parsers, layout, fonts, the PDF
writer and the documentation, together with twelve improvement proposals. Eight are
release-blocking: three paths let a small input exhaust memory, five stages of the
work budget do not charge the work they do, and a breaking API change is queued for
a minor release. The rest are wrong-output, limit-bypass and documentation defects.
The suite is green because none of them has a fixture or a normative statement: the
code does what the specs do not forbid, and the documents promise what the code does
not do.

This change turns every one of the 66 findings and the twelve improvement proposals
into a normative requirement with a scenario, so that a fix or a standing rule is
testable and reviewable, and then fixes the code and the documents behind it.

## What Changes

- **Memory and work bounds.** Cap and charge the GPOS pair table; cap the problems a
  stylesheet can record; charge text shaping, repeated measurement, CFF outline
  conversion, PDF serialisation and adjacent-sibling matching; observe cancellation
  in CFF conversion. A small input must fail with `resource/limit` instead of
  allocating gigabytes.
- **Contract.** Make `Validate` report everything `Convert` can decide before
  serialisation, including metadata; validate inline `<style>` rules when an option
  is invalid; give a missing font the position of its declaration; let
  `CharsetSupported` accept the empty name; stop adding dependent diagnostics after
  a budget failure.
- **HTML positions and encodings.** Derive a node's source span from its own token
  (foster parenting currently swaps `<style>` content); reject a declared
  windows-1252 even for valid UTF-8 bytes; keep the `<meta>` offset exact; define
  and test the meaning of a diagnostic column.
- **CSS grammar.** Reject CSS-wide keywords in `font-family`; accept `media` on
  `<style>`; reject a pseudo-element outside the final compound; reject or resolve
  a percentage `height`; bound adjacent-sibling matching and stylesheet
  diagnostics; fix nested at-rules, duplicate ids, identifier escapes, media
  lists, swallowed declarations and the undocumented `always` value; resolve
  relative paths inside the confined reader's root.
- **Layout and pagination.** Do not start a page for a box that can never fit;
  fragment a spanning cell across pages; reserve a declared height once; clamp
  table columns; report an unbreakable token that cannot fit; detect image formats
  from content; resolve asset references as URL paths; saturate geometry; compute
  image area in 64 bits; refund a failed image load; reject a truncated JPEG.
- **PDF output.** Validate the creation date in UTC; preserve PNG colour type and
  bit depth; keep `TJ` and `/W` arrays within the compatibility limits; skip a
  zero-area image; cap `/BaseFont`; decode a PNG once through typed fast paths.
- **Documentation and process.** Release the removal of `Options.ImportBrowser` as
  v2.0.0 with a migration section; complete the diagnostic catalogue; remove the
  stale lowering claim; align `docs/profile.md` with the code; document the
  `FontDir` fallback; fix the broken list, fence and removed-flag references; give
  open work one home; refresh the benchmark figures; pin the CI image; make the
  test oracle and the PDF extractor check what they claim; remove dead internal
  API and test-only environment hooks from the production path.
- **Improvements as standing rules.** Declare every diagnostic code in one registry;
  serve both entry points from one pipeline; state each stage's cost and test the
  invariant; record source spans during construction; resolve every asset through
  one resolver; generate or check the code facts a document states; refuse a
  breaking tag as a minor; require an independent writer validator, a coverage
  floor and a validator-enabled gate; share test support code; and benchmark each
  measured hot spot.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the work budget covers five more stages; `Validate` matches
  `Convert` before serialisation; an invalid option still validates inline rules;
  a missing font names its declaration; `CharsetSupported` accepts the empty name;
  a budget failure stops dependent diagnostics; one pipeline serves both entry
  points; every budgeted stage states a cost and has an invariant test.
- `text-shaping`: the GPOS pair table and CFF conversion are bounded and charged;
  a non-breaking space survives in the text layer; composite glyph arguments and
  the depth cap are correct; GPOS lookups are selected by script, feature and flag;
  the face cache is unambiguous; every font load path is charged.
- `html-parsing`: a node's span comes from its own token under foster parenting; a
  declared windows-1252 is rejected for valid UTF-8 bytes; the `<meta>` offset is
  exact; diagnostic columns have one defined meaning; source spans are recorded
  during construction.
- `css-profile`: stylesheet problems are capped and charged; adjacent-sibling
  matching is bounded; the grammar is corrected for CSS-wide keywords, `media` on
  `<style>`, pseudo-element position, percentage heights, nested at-rules,
  duplicate ids, identifier escapes, media lists and swallowed declarations; a
  relative path resolves inside its root; `always` is documented; grammar pieces
  share one implementation and one policy.
- `document-layout`: table columns stay inside the content box; an unbreakable
  token is reported; image formats come from content and references are URL paths;
  geometry saturates; image area is computed in 64 bits; a failed load is refunded;
  a truncated JPEG is rejected; a PNG is decoded once; one resolver serves every
  asset kind.
- `pagination`: a box that can never fit does not start a page; a spanning cell is
  fragmented across pages; a declared height is reserved once.
- `pdf-output`: the creation date is validated in UTC; PNG bit depth is preserved;
  `TJ` and `/W` stay within compatibility limits; a zero-area image is skipped;
  `BaseFont` names are capped.
- `library-docs`: a breaking change is a major version; the diagnostic catalogue
  matches the code; the README and the profile document describe only current
  behaviour; the font directory fallback is documented; formatting renders as
  written; open work has one home; performance figures are current; code facts in
  documents are generated or checked.
- `code-quality`: the workflow, renderer record and documentation name one image;
  a test oracle compares what it claims; the budget's concurrency contract is
  documented; dead internal API is removed; the production path reads no test
  environment variable; shared test helpers have one definition; every diagnostic
  code is declared once; a breaking tag cannot be a minor; the gate has a coverage
  floor and a mandatory validator; each measured hot spot is benchmarked.
- `golden-tests`: the PDF extractor validates the structure it reads and is not the
  only check on the writer; an independent validator checks writer output.

## Impact

- **Code:** `internal/font/{gpos,cff_convert,glyph,subset}`, `internal/text`,
  `internal/layout/{paginate,table,lines,paint}`, `internal/css/{token,parse,
  selector,cascade,props}`, `internal/profile`, `internal/html/{parse,node}`,
  `internal/charset`, `internal/engine/{convert,fonts,images,engine}`,
  `internal/pdf/writer`, `internal/fsx`, `internal/budget`, `internal/pdfdebug`,
  `pdfgen/pdfgen.go`, `cmd/pdfgen/main.go`.
- **Tests:** a fixture under `testdata/adversarial/` for every hostile case, a
  regression test in the owning package for every correctness case, and the
  snapshot update that a byte change requires (font naming, PNG depth, `TJ`).
- **Documents:** `README.md`, `CHANGELOG.md`, `CONTRIBUTING.md`, `TODO.md`,
  `docs/profile.md`, `docs/fonts.md`, `docs/golden.md`, `docs/mpdf-corpus.md`,
  `pdfgen/doc.go` and the main specs.
- **CI:** the pinned image, the renderer record and the validator.
- **Compatibility:** removing `Options.ImportBrowser` is a breaking change released
  as v2.0.0 with migration notes. The remaining changes preserve the API; a few
  change output bytes (font names, 16-bit PNGs, `TJ` packing) and therefore the
  committed snapshots.
