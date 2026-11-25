# WeasyPrint corpus

The corpus is six documents written for [WeasyPrint](https://weasyprint.org/), a
browser-grade HTML/CSS-to-PDF engine, vendored unmodified together with the PDF
that upstream produced for each of them. They are the closest thing to real
input this library has: full documents with their own CSS, fonts and images,
rather than fragments written for the tests.

`internal/weasycompat` runs them through the engine and enforces, per case, the
state the manifest declares. The corpus sits outside the two snapshot layers of
[docs/golden.md](golden.md), as the mPDF corpus does: its documents are too large
to freeze as document XML or page rasters, and the interesting comparison is
against another engine, not against ourselves.

## What a case is

```
testdata/weasyprint/
  invoice/ letter/ poster/ report/ ticket/   byte-identical upstream directories
  samples.json                               source pin, file hashes, six cases
  oracle/<case>.json                         page count and tokens of their PDF
  golden/<case>.txt                          our page text, once a case converts
  licenses/                                  font licences + the font mapping
  NOTICE.md                                  provenance, licences, exclusion of book
```

`poster/` serves two cases: upstream's `printall.sh` renders `poster.pdf` with
`poster.css` and `flyer.pdf` with `flyer.css` from one `poster.html`, so the
manifest names the extra stylesheet of a case and the harness injects that
`<link>` into an in-memory copy. The document is never edited on disk.

The `book` sample is deliberately absent: it is about 27 MB, its text and images
belong to Madcats, and it is the furthest from this profile. `NOTICE.md` says so,
so the corpus is not mistaken for the complete upstream collection.

## The three states

| State | What the suite asserts |
|---|---|
| `blocked` | conversion returns an error, at least one error diagnostic exists, its first code equals `blocked_by`, nothing panicked, and a second run produces identical diagnostics |
| `measure` | conversion succeeds and our page text equals `golden/<case>.txt`; the page count and token difference from the oracle is logged, not enforced |
| `accepted` | everything `measure` asserts, plus the same page count and the same token multiset as the oracle |

The state is enforced against reality, not trusted: a blocked case that starts
converting fails with "promote it to measure", and an accepted case that stops
converting fails with its diagnostics. Promotion is a reviewable edit of
`samples.json` — the diff says what improved and when.

## What is compared, and what is not

Page text is compared byte for byte, with a form feed between pages, so the
comparison also sees a page that disappeared. The reference contributes two
facts: its page count and its token multiset. A token is a maximal run of
Unicode letters or digits after NFKC, with spaces folded and invisible characters
(soft hyphen, zero-width space and joiner, word joiner, byte-order mark) removed;
case is kept and duplicates are counted. Punctuation, order and line breaking are
excluded on purpose: two engines place a line differently without either being
wrong, while dropped, duplicated or invented words and a lost `text-transform`
are real defects the multiset still catches.

There is no pixel comparison. Two engines with the same features still break
lines and hyphenate differently, so equality is unattainable and a tolerance
would be an arbitrary number that hides real regressions.

## Commands

| Command | Effect | Needs |
|---|---|---|
| `go test ./internal/weasycompat -v` | runs the corpus and prints each case, its state and its blocker | nothing |
| `go test ./internal/weasycompat -run TestCorpus -update` | rewrites the page-text goldens of the cases this run processed | nothing |
| `go test ./internal/weasycompat -run TestUpdateOracle -update-oracle` | regenerates `oracle/*.json` from the reference PDFs | `pdftotext` |
| `go test ./internal/weasycompat -run TestUpdateHashes -update-hashes` | recomputes the file hashes in `samples.json` | nothing |

The normal test reads the committed oracle and needs no external tool and no
network, the same way the rest of the suite runs. Only `-update-oracle` executes
`pdftotext`.

## The oracle

`oracle/<case>.json` records the source commit, the SHA-256 of the reference PDF,
the extractor that produced the text, the page count, the extracted text per
page, and the normalised token multiset. Both hashes are checked on every run:
replacing a reference PDF without regenerating its oracle fails with the
regeneration command, so a stale reference cannot pass silently.

The committed oracles were produced with **`pdftotext` 26.11.0** (Homebrew, macOS).
CI never runs `pdftotext`: the poppler of the CI image (24.02 on `ubuntu-24.04`)
is there for the render goldens only, and the oracle is committed. Regenerating
the oracle with another poppler changes spacing and possibly reading order, so it
is a deliberate commit whose text diff is reviewed like any other golden.

`pdftotext` writes a form feed after every page, including the last one; the
generator drops exactly one trailing empty part, which is why `report` — whose
second page is genuinely blank — records eight pages, one of them empty.

## Current gap

Every case is blocked, because the profile does not yet have the features these
documents need. The blocker is the first error code, and the suite fails if it
changes:

| Case | Reference pages | Tokens | First blocker | What blocks it |
|---|---|---|---|---|
| `invoice` | 1 | 76 | `html/unsupported-element` | `<aside>` is outside the profile; a flex row would be the next rejection |
| `letter` | 1 | 95 | `html/unsupported-element` | `<header>`, then a web form |
| `poster` | 1 | 90 | `html/unsupported-element` | sectioning elements (`<section>`, `<article>`) |
| `flyer` | 1 | 62 | `html/unsupported-element` | the same document as `poster` |
| `ticket` | 1 | 34 | `html/unsupported-element` | `<section>` is outside the profile; a flex row would be the next rejection |
| `report` | 8 | 820 | `html/unsupported-element` | `<article>` is outside the profile; flex, columns, floats and named pages follow |

A green run therefore means "each case is in the state the manifest claims", not
"the samples render". Making a sample convert is feature work in a separate
change; this corpus is the instrument that shows the distance and gates the
result.

## Promoting a case

1. Convert the blocker into a feature in the profile (a separate change).
2. Edit `samples.json`: set `"state": "measure"` and clear `blocked_by`.
3. Run `go test ./internal/weasycompat -run TestCorpus -update` to write the
   golden, and read the logged `pages N/M, tokens N/M, missing …, extra …` line.
4. Review the golden and the difference. Only when the difference is understood
   does the case move to `"accepted"`, where it is enforced.

The manifest lint fails a blocked case without a `blocked_by`, a converting case
that still names one, an unknown state, a case whose directory, HTML or
stylesheet is missing, a vendored directory without a case, and a file that
`samples.json` does not list.

## Refreshing the upstream documents

Replacing the samples with another upstream commit is one reviewed commit:

```bash
# 1. Replace the directories with the new upstream tarball, byte for byte.
# 2. Recompute the hashes and review the samples.json diff.
go test ./internal/weasycompat -run TestUpdateHashes -update-hashes
# 3. Regenerate the oracle, then review pages and tokens per case.
go test ./internal/weasycompat -run TestUpdateOracle -update-oracle
# 4. Regenerate the goldens of the cases that convert.
go test ./internal/weasycompat -run TestCorpus -update
# 5. Update the commit in samples.json, NOTICE.md and this document.
```

A new pin can change which features block a case, so the gap table above is
regenerated from the verbose run rather than edited from memory.

## Size and licences

The corpus adds 7.95 MB to `testdata` (measured over the 66 committed files), of
which 2.12 MB are the reference PDFs, 44 KB the licence texts and 20 KB the
oracle; the six vendored directories alone are 7.86 MB. `testdata` ships inside the module zip, so this
is a real cost for every consumer of the library; it is accepted because the
documents are the only external fidelity target, and because `book` (27 MB) is
excluded.

The documents are public domain, apart from the fonts, which keep their own
licences: `licenses/index.json` maps every bundled font file to its licence text,
and the test fails on a font with no entry, an entry with no file, or a font
covered twice. Coming Soon is the one font not under the SIL Open Font License —
it is Apache-2.0 — and its licence is committed accordingly. `NOTICE.md` records
the upstream commit, the licence statement and the exclusion of `book`.
