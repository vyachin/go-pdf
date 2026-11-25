# Tasks

## 1. Roots

- [x] 1.1 Return `options/asset-root` when `fsx.Open` fails and stop collecting stylesheets and fonts — verify: the missing-BaseDir fixture reports only that code and the outside file is not read.
- [x] 1.2 Never call `readConfined` with a nil root — verify: a unit test passes a failed root and gets an error, not an unbounded read.

## 2. Command

- [x] 2.1 Compare the resolved output with the input (`os.SameFile` and case-insensitive handling) and refuse to clobber — verify: `template.pdf` and `-o invoice.html invoice.html` both refuse.
- [x] 2.2 Write through a temporary file in the destination directory and rename — verify: a simulated write failure leaves the previous file byte-identical.
- [x] 2.3 Check the input size with the exported defaults before reading — verify: an oversized file fails without a full read.

## 3. API and docs

- [x] 3.1 Add `pdfgen.DefaultLimits()` and test it against the zero-value defaults — verify: the exported value matches `Limits{}.resolved()`.
- [x] 3.2 Update the CLI usage text and `docs/profile.md` for the new failure modes — verify: the documented exit codes and messages match the tests.
