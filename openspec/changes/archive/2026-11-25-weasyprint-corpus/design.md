# Design

## Context

The suite has three layers today: document snapshots, render snapshots and the
mPDF corpus, which contributes short HTML fragments and compares them against
our own text goldens only. The WeasyPrint samples are full documents authored
for a browser-grade paged-media engine, and upstream publishes the PDF that
WeasyPrint produced for each of them. See proposal.md for why they are worth
keeping.

Measured against this profile today, none of the six converts: `poster` and
`flyer` (one HTML, two stylesheets) need `@import`, CSS nesting and absolute
positioning; `invoice` needs flex rows and five sectioning elements; `ticket`
needs custom properties, columns and CFF fonts; `report` (the only multi-page
document, 8 pages) needs all of that plus columns, floats, `string-set` and
named pages; `letter` is a web form and will stay outside the profile.

Constraints that shape the design:

- the test suite runs offline and, apart from `pdfcpu` in CI, without external
  tools;
- `testdata` ships inside the Go module zip, so any growth is paid by every
  consumer of the library;
- the documents are third-party content with their own licensing;
- the repository already has a precedent for an external corpus (`mpdfcompat`)
  and for an external oracle (`pdfcpu`, `pdftoppm`).

## Goals / Non-Goals

**Goals:**

- A corpus whose distance from the profile is visible per case and whose
  behaviour cannot change unnoticed.
- A machine-checkable, offline fidelity signal derived from the WeasyPrint output,
  not from our own output.
- Provenance strong enough that "the documents are unmodified" and "the oracle
  belongs to these PDFs" are checked, not asserted.

**Non-Goals:**

- Making the samples convert. That is feature work in later changes; this change
  only builds the instrument that shows the gap and will gate the result.
- Pixel comparison with the WeasyPrint PDFs.
- Replacing the existing golden layers. The corpus stays outside both, as the
  mPDF corpus does.

## Decisions

### D1. Vendored layout mirrors upstream

`testdata/weasyprint/<sample>/` is a byte-identical copy of the upstream
directory (HTML, CSS, fonts, images, reference PDF), with `samples.json`,
`oracle/`, `golden/`, `licenses/` and `NOTICE.md` beside it. The extra
stylesheet that `printall.sh` passes with `-s` is recorded per case in the
manifest, and the two cases that share `poster/` (`poster`, `flyer`) simply name
different stylesheets.

*Alternative:* normalise everything into one JSON corpus like
`testdata/mpdf/cases.json`. Rejected: fonts, images and stylesheets must stay on
disk for `@font-face` and `url()` resolution, and hashing a whole directory is a
stronger provenance check than embedded strings.

### D2. Six cases from five directories; `book` is absent

The manifest lists `invoice`, `letter`, `poster`, `flyer`, `ticket`, `report`.
`book` is not vendored and not listed: its text and images belong to Madcats, it
is 27 MB, and it is the furthest from the profile. Its exclusion is documented in
`NOTICE.md`, so the corpus is not mistaken for the complete upstream set.

`letter` is kept although form controls are outside the profile and it will stay
rejected: it is a real-world document and a robustness case (form markup must not
panic). If the module size ever becomes a concern, it is the first case to drop.

### D3. Three declared states, enforced against reality

Each case declares `blocked`, `measure` or `accepted`:

| state | asserted | oracle |
|---|---|---|
| `blocked` | conversion fails with diagnostics, first error code equals `blocked_by`, no panic, two runs identical | not used |
| `measure` | conversion succeeds, our page text matches the golden | measured, logged, not enforced |
| `accepted` | `measure` assertions plus page count and token multiset equal to the oracle | enforced |

The test fails when the observation and the state disagree: a blocked case that
converts, an accepted case that stops converting or loses fidelity. Promotion is
an explicit edit of `samples.json` (plus `-update` to create the golden, for
`measure`), so the diff shows what improved.

*Alternative:* an expected-failure list that skips broken cases. Rejected: skips
rot silently, and a fixed case is indistinguishable from a still-broken one.

*Alternative:* promote automatically when conversion succeeds. Rejected: a
red-to-green flip would bypass review of the new golden and of the fidelity
result, which is exactly the moment a human should look.

### D4. What is compared: page count and token multiset, never pixels

Our own output is frozen per page as text. The external oracle contributes two
facts about the WeasyPrint PDF: its page count and the multiset of tokens in each
page's extracted text. An accepted case must match the page count exactly and
the document-wide token multiset exactly; `measure` reports the same numbers
without failing.

A token is a maximal run of Unicode letters or digits after normalisation: NFKC
(so `ﬁ` becomes `fi`), non-breaking spaces folded to a space, soft hyphens and
zero-width characters removed. Punctuation, whitespace and order are excluded on
purpose: they differ between engines for reasons that are not defects (line
breaking, kerning, ligature extraction), while dropped, duplicated or invented
text and a missed `text-transform` are real defects that the multiset still
catches. Our own per-page golden covers ordering on our side.

*Alternative:* pixel comparison. Rejected: two engines place a line differently
even when both implement the same feature, so equality is unattainable and a
tolerance threshold is arbitrary and unfalsifiable.

*Alternative:* a text similarity ratio (Levenshtein or per-page token
intersection). Deferred, not rejected: it needs calibration against a case that
actually converts, and a binary multiset check needs none. Per-page token
gating is the natural tightening once a case reaches `accepted`.

*Alternative:* oracle from the source HTML text instead of the PDF. Rejected: it
misses generated content — page counters, `target-counter` table-of-contents
numbers, `::before` and `content()` — which is precisely what the reference
output proves.

### D5. The oracle is committed; poppler is only a maintenance tool

`samples.json` and `oracle/<case>.json` are committed. Each oracle records the
upstream commit, the SHA-256 of the reference PDF, the page count, the raw
extracted text per page, and the identity of the extractor (`pdftotext -v`).
Generation runs under a test flag, `-update-oracle`, which is the only path that
executes `pdftotext`; the normal test reads JSON only. The reference PDF hash is
verified on every run, so replacing a PDF without regenerating its oracle fails.

Generation is local and one-off: the oracle is produced with the poppler a
maintainer installs (`brew install poppler` on macOS), and the exact
`pdftotext -v` line is committed inside each oracle and named in
`docs/weasyprint-corpus.md`. CI never runs `pdftotext`: it reads the committed
JSON, so an image bump cannot silently move the oracle. Regenerating it with
another poppler is a reviewed commit whose text diff is expected and inspected.

*Alternative:* run `pdftotext` during the test when it is available. Rejected:
the suite would silently change coverage per machine, and CI already gates the
mPDF corpus on a missing tool being an error rather than a skip.

*Alternative:* a separate `genoracle` command. Rejected: another surface to
document; the existing suites already treat an external binary as a maintenance
tool (`pdfcpu`, `pdftoppm`).

### D6. The extra stylesheet is injected into the HTML, not added to the API

For `poster` and `flyer`, the harness inserts a `<link rel="stylesheet">` for the
case's stylesheet before `</head>` in an in-memory copy of the HTML. The engine
then resolves it like any other external stylesheet, which is also the path a
real browser template takes.

*Alternative:* add `Options.Stylesheets` to the public package. Rejected: no
product need, and `pdfgen`'s surface is versioned; the harness does not justify
growing it.

### D7. `pdfdebug` gains page counting without changing its existing readers

The current `Pages` keys on content streams that contain `Tj`/`TJ`, so a page
that carries only an image or a background disappears and every later golden
shifts. The change adds `PageCount` and `PageTexts`, which walk the page tree
(`/Kids`) and resolve each page's `/Contents` reference, returning one entry per
page including empty ones. `Pages` and `Text` keep their current behaviour, so
the mPDF and engine text goldens do not move for a reason unrelated to this
change.

*Alternative:* fix `Pages` in place. Rejected: it is used by existing goldens;
a silent regeneration of unrelated goldens would hide a real difference.

### D8. Conversion options match what a browser template gets

Cases are converted with `ImportBrowser: true` (the samples are browser-authored
documents, and this corpus is the natural benchmark of the import layer), with
`BaseDir` and `FontDir` set to the sample directory, the HTML path as `File`, and
no fallback font: a sample that cannot load its declared font must fail with
`font/unavailable` rather than silently render with DejaVu. The mPDF corpus
stays strict; the two corpora exercise opposite sides of the import switch.

### D9. Provenance is checked, not described

`samples.json` records the upstream URL and commit, and a SHA-256 for every
vendored file. The test walks each vendored directory and fails on a changed,
missing or unexpected file. `NOTICE.md` records the licence statement, the
exclusion of `book`, and points at `licenses/`, which carries the OFL text for
each of the seven bundled font families.

*Alternative:* trust the copies. Rejected: an edited sample invalidates the
oracle while every test stays green, which is the failure mode this corpus
exists to prevent.

### D10. Documentation and spec placement

A new `weasyprint-corpus` capability owns the guarantees above.
`docs/weasyprint-corpus.md` is the maintainer reference (states, commands,
refresh procedure, current gap table), linked from `CONTRIBUTING.md`; the
`library-docs` requirement that enumerates contributor material gains this
corpus. The README is untouched: it is a chooser's front page. CI workflows are
untouched because `go test ./...` already runs the package.

## Risks / Trade-offs

- **The token metric may not behave as intended on the first converting case**
  (ligature extraction, hyphenation, `text-transform` application). → The
  `measure` state exists precisely for this: a case is promoted to `accepted`
  only after the logged difference is reviewed and understood. If the metric
  proves unusable, the normalisation changes before any acceptance, not after.
- **The multiset is order-blind.** A paragraph moved to another page is not
  detected by the oracle. → Our per-page text golden catches changes in our own
  ordering; upstream ordering is reviewed from `page_text` when a case reaches
  `measure`.
- **`blocked_by` churn.** Every feature removal changes one line per affected
  case. → Accepted: that one-line diff is the visible progress report, and it is
  what makes a silent change impossible.
- **Upstream drift.** HTML and the committed PDF can diverge if upstream
  regenerates only one of them, and a bump changes the oracle. → The SHA is
  pinned, every file is hashed, and the refresh procedure (replace files,
  regenerate oracle and goldens, review the diff) is documented as one commit.
- **Module size.** ≈7.6 MB, of which 2.1 MB is the reference PDFs. → Deliberate:
  `book` (27 MB) is excluded, and the PDFs are kept because they are the
  oracle's source of truth and the only way to regenerate it without the network.
- **pdftotext version drift.** Regenerating the oracle on another poppler
  changes spacing and possibly reading order. → The extractor identity is
  recorded in each oracle, only tokens are compared, and regeneration is a
  reviewed change.
- **The corpus is green while nothing converts.** → That is the intended
  reading: green means "the declared state is true", and the states are visible
  in the manifest and in the docs table.

## Migration Plan

No production behaviour changes; the corpus is test-only data and code. Rollout
is one change: vendored data, harness, `pdfdebug` additions, docs and specs. A
revert removes the package and `testdata/weasyprint/` with no other effect. The
first promotion of a case (blocked → measure → accepted) is a later, ordinary
change that uses the harness.
