# mPDF corpus

The mPDF tests are PHPUnit tests: they check internal APIs (`Imagick`, `Reflection`, `$mpdf->PageLinks`), and their snapshot tests compare a page raster with a reference PDF. Such checks cannot be ported to Go: they test the PHP implementation and features that our profile does not have.

What carries over is the **documents**. HTML was extracted from the mPDF tests and turned into a compatibility corpus: each document is run through the engine, and what is checked is not a match with mPDF but the correct behaviour of our engine.

## What is checked

| Category | Check |
|---|---|
| Document inside the profile | converts, the PDF is valid (`pdfcpu`), byte-deterministic, the text matches the golden text |
| Document outside the profile | rejected with diagnostics, without a panic and without silently ignoring it |
| Dynamic PHP | skipped with the reason stated |

A panic while processing any document fails the test with a stack trace.

The corpus is outside both snapshot layers described in
[docs/golden.md](golden.md): 149 documents would dominate the repository, so
these documents keep their text goldens and are not part of the document and
render snapshot sets.

## Corpus composition

`testdata/mpdf/cases.json` — 149 entries:

- **81** documents from the issue tests (`tests/Issues/*.php`), extracted from `WriteHTML('...')` and heredocs;
- **14** snapshot documents (`tests/Snapshots/*Test.php`) written as plain HTML between `ob_start(); ?>` and `ob_get_clean()`;
- **54** stub entries: 51 issue tests and 3 snapshot tests where the HTML is assembled by PHP code (variables, loops, `$mpdf->...` calls) and is not extracted statically.

Current run result:

```
36 convert, 59 rejected by the profile, 54 skipped

issue:    converted=36 rejected=45 skipped=51
snapshot: converted=0  rejected=14 skipped=3

rejections: unsupported-property 19, unsupported-element 19,
           img/load 8, unsupported-value 6, text/glyph 6,
           unsupported-page-selector 1
```

## Why the snapshot documents do not convert

This is exactly the map of the gap between mPDF and our profile. The first reason for rejection for each document:

| Document | Blocks |
|---|---|
| `auto-font-snapshot` | `text/glyph`: the Devanagari character `अ` is missing from the test font |
| `backgrounds-and-borders-snapshot` | `background-gradient` |
| `barcode-snapshot` | `position` |
| `basic-html-snapshot` | `float` |
| `dot-tab-snapshot` | `outdent` (an mPDF-specific property) |
| `justify-snapshot` | `<s>` — `text-decoration` is needed |
| `line-height-snapshot` | `background-gradient` |
| `list-style-type-snapshot` | `clear` |
| `paging-css-snapshot` | `<htmlpageheader>` (an mPDF-specific tag) |
| `paging-html-snapshot` | `<pageheader>` (an mPDF-specific tag) |
| `positioned-html-snapshot` | `clear` |
| `rtl-snapshot` | `direction` |
| `table-snapshot` | `<link rel="stylesheet">` — external CSS |
| `watermark-snapshot` | `<blockquote>` |

Some of this is deliberate profile limitations (gradients, `float`, `position`, RTL, external CSS). Some are gaps worth closing: `text-decoration` (for `u`, `s`, `del`, `ins`) and `<blockquote>`.

The port has already uncovered one real gap: the profile had no `<b>` and `<i>`, although they are basic document markup. They were added together with the default rules.

## License and provenance

The HTML fragments are taken from [mpdf/mpdf](https://github.com/mpdf/mpdf/tree/development/tests), license GPL-2.0. They are used as test data; the harness in `internal/mpdfcompat` was written from scratch.

If the project is distributed under a license incompatible with GPL-2.0, the `testdata/mpdf` directory must be excluded from the distribution or the fragments replaced with original equivalents. For details, see `testdata/mpdf/NOTICE.md`.

## How to update the corpus

```bash
git clone --depth 1 https://github.com/mpdf/mpdf /tmp/mpdf
The corpus is committed in `testdata/mpdf/cases.json`; extracting it again is
a one-off task and the importer is no longer part of the repository.

# Rebuild the golden texts after changing the engine
go test ./internal/mpdfcompat -update
```

The importer does not access the network itself: it reads a local copy of the repository. The corpus is committed, so the tests work offline.

## Limitations of the approach

- This is **not a comparison with mPDF**. The text match is checked against our own golden text, not against mPDF output: a full comparison would require rasterization and support for all mPDF features.
- "Converts" means "the profile accepted the document", not "the result matches a browser". For example, `{PAGENO}` in the text stays a literal: it is an mPDF placeholder, not HTML.
- `-short` skips documents larger than 20 KB so that the quick run stays quick.
