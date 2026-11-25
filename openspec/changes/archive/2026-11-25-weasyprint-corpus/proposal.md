# Proposal

## Why

The suite freezes what the engine understands and how it paints, but it has no
outside documents: `testdata/` holds hand-written invoices and certificates, and
the mPDF corpus contributes short fragments. The WeasyPrint samples are the
closest thing to the real input of this library — full documents authored for a
browser-grade paged-media engine, with reference PDFs produced by WeasyPrint —
and they expose a gap the suite currently hides: of the six samples, none
converts today. Keeping those documents as a maintained corpus turns the gap
into a reviewable map and gives the engine an external fidelity target instead of
only comparisons with itself.

## What Changes

- **Vendor the samples as a fourth corpus.** Six cases from five directories of
  `CourtBouillon/weasyprint-samples` at the pinned commit `6dc675b`
  (`invoice`, `letter`, `poster`, `flyer`, `ticket`, `report`), copied
  unmodified with their CSS, fonts, images and WeasyPrint reference PDFs
  (≈7.6 MB). The `book` sample is excluded and documented, not vendored: its
  text and images belong to Madcats, it is 27 MB, and it is the furthest from
  the profile.
- **A new `internal/weasycompat` harness with explicit sample states.**
  `blocked` (rejected, with the first blocking diagnostic code committed),
  `measure` (converts; our own page text is a golden, the external oracle is
  measured and logged) and `accepted` (the oracle gates as well). The state in
  the manifest must agree with reality; a sample that starts converting fails
  the test until it is promoted deliberately.
- **Per-page text goldens of our own output** under
  `testdata/weasyprint/golden/`, regenerated with `-update`, with stale-golden
  detection.
- **A committed fidelity oracle derived from the WeasyPrint PDFs**: page count
  and normalised text per page, produced once with `pdftotext` and committed, so
  the test runs offline and without poppler.
- **Provenance and licence bookkeeping**: `NOTICE.md` with the upstream commit,
  the exclusion of `book`, hashes of the vendored files, and the OFL licence
  texts of the seven bundled font families.
- **Documentation and spec delta**: a new `weasyprint-corpus` capability,
  `docs/weasyprint-corpus.md`, a link from `CONTRIBUTING.md`, and the
  contributor-material requirement in `library-docs` extended to name the second
  corpus.
- **No public API change and no CI workflow change.** The harness is an
  `internal/` test package, and `go test ./...` already runs it.

## Capabilities

### New Capabilities

- `weasyprint-corpus`: the maintained corpus of WeasyPrint sample documents —
  pinned provenance, unmodified vendoring, the blocked/measure/accepted state
  machine, per-page text goldens, the external page-count and text oracle, the
  exclusion of `book`, and the offline guarantee.

### Modified Capabilities

- `library-docs`: the "Contributor material has its own entry point" requirement
  names the corpora a maintainer must find; it must name this corpus next to the
  mPDF one.

## Impact

- New test package `internal/weasycompat` and new data under
  `testdata/weasyprint/` (≈7.6 MB with the reference PDFs, ≈5.7 MB without).
  `testdata` ships in the module zip, so the growth is deliberate and limited to
  the six cases that carry information.
- `internal/pdfdebug` gains honest page counting; its current page detection
  keys on `Tj`/`TJ` and would drop a page that carries only an image, which
  would shift every per-page golden after it.
- `docs/weasyprint-corpus.md` is new; `CONTRIBUTING.md` and the `library-docs`
  spec gain a pointer.
- No change to `pdfgen`, the CSS profile, the golden snapshot layers, the mPDF
  corpus or the workflows. The oracle generation needs poppler once, at
  maintenance time; the test itself needs no external tool.
