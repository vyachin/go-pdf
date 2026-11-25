# WeasyPrint samples corpus

This directory vendors sample documents published by CourtBouillon for
[WeasyPrint](https://weasyprint.org/), a browser-grade HTML/CSS-to-PDF engine.
The documents are unmodified copies: they are the external input of the
`internal/weasycompat` suite, which converts them with this engine and measures
the result against the reference PDF that upstream produced.

## Source

| | |
|---|---|
| Repository | https://github.com/CourtBouillon/weasyprint-samples |
| Commit | `6dc675b05a5cba59576dfaa8881ea6ccdccb13cc` |
| Retrieved | 2026-11-25, as the tarball of that commit |
| Vendored | `invoice/`, `letter/`, `poster/`, `report/`, `ticket/` (49 files) |
| Verified | `diff -r` against a fresh extraction of the same commit: no file added, edited or dropped |

The SHA-256 of every vendored file is committed in `samples.json`, so a local
edit is a test failure, not a silent divergence from the reference output.

`poster/` serves two cases: upstream's `printall.sh` generates `poster.pdf` with
`poster.css` and `flyer.pdf` with `flyer.css` from the same `poster.html`. Both
PDFs are kept, and the manifest names the stylesheet of each case.

## Licence of the documents

Upstream states the terms of the collection in its `README.rst`:

> The fonts used have their own licence.
>
> The text of the "All Cats Are Beautiful" book is the full property of Madcats
> (https://madcats.fr/).
>
> Everything else from the samples is in the public domain.

Only that public-domain material is vendored. The fonts are third-party work
under their own licences, reproduced in `licenses/` next to the licenses that
cover them; the mapping from every bundled font file to its licence text is in
`licenses/index.json`.

## The `book` sample is excluded

`book/` is **not** part of this corpus: its text and images belong to Madcats,
it is about 27 MB, and it is the furthest sample from this engine's profile. Its
absence is deliberate, so this corpus must not be read as the complete upstream
set. The six cases here are `invoice`, `letter`, `poster`, `flyer`, `ticket` and
`report`.

## The reference PDFs

Each sample ships the PDF that WeasyPrint produced for it upstream; the command
that produced it is upstream's `printall.sh`. Those PDFs are the source of the
committed per-case oracle in `oracle/` (page count and normalised text per
page). They are **not** this engine's output and are never compared pixel by
pixel: this corpus measures our own output page by page and checks fidelity
against the reference text, not against the reference painting.

## Fonts

| Family | Files | Licence |
|---|---|---|
| Source Sans 3 | `invoice/SourceSans3-Regular.ttf`, `invoice/SourceSans3-Bold.ttf` | `licenses/OFL-Source-Sans-3.txt` |
| Pacifico (2011) | `invoice/pacifico.ttf` | `licenses/OFL-Pacifico-2011.txt` |
| Pacifico (2018) | `letter/fonts/Pacifico-Regular.ttf` | `licenses/OFL-Pacifico-2018.txt` |
| Coming Soon | `letter/fonts/ComingSoon-Regular.ttf` | `licenses/Apache-2.0-Coming-Soon.txt` |
| Archivo Black | `poster/archivoblack-regular.ttf` | `licenses/OFL-Archivo-Black.txt` |
| Fira Sans | `report/FiraSans-{Light,LightItalic,Regular,Italic,Bold}.ttf` | `licenses/OFL-Fira-Sans.txt` |
| Barlow Condensed | `ticket/barlowcondensed-{light,regular,bold}.otf` | `licenses/OFL-Barlow-Condensed.txt` |
| Libre Barcode 128 | `ticket/librebarcode128-regular.ttf` | `licenses/OFL-Libre-Barcode-128.txt` |

Coming Soon is the one font in the corpus that is not under the SIL Open Font
License: it is Apache-2.0. Its licence text is committed as-is for that reason.
The other seven licences are the SIL Open Font License 1.1, each with the
copyright notice read from the name table of the bundled font version — the
pinned samples ship older cuts than the current upstream release, so the year
in a licence file can differ from the latest published one without either text
being altered.

## Refreshing the corpus

Replacing the samples with another upstream commit is a reviewed change that
must, in the same commit: replace the directories, regenerate `samples.json`
(hashes and cases), regenerate `oracle/` with `-update-oracle`, regenerate the
per-case goldens, and update this file with the new commit. `docs/weasyprint-corpus.md`
records the procedure and the tools it needs.
