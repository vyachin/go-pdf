# Tasks

## 1. Positions and codes

- [x] 1.1 Fill `File` (and `Line`/`Column` when known) in `fromDiags` and at the diagnostic sites for images, fonts and limits — verify: the three scenarios report the document name.
- [x] 1.2 Rename or document the `text/font` family-resolution code — verify: `docs/profile.md` and the code list agree.

## 2. Options

- [x] 2.1 Parse and validate `PageSize`, `Margin`, `Date` and `Charset` at the top of `prepare` and report all failures — verify: bad page size plus bad CSS yields both diagnostics.
- [x] 2.2 Reject a year outside 0..9999 with `options/date` — verify: pdfcpu strict accepts a PDF with a valid date and the invalid date never reaches the writer.

## 3. Warnings and lists

- [x] 3.1 Warn only for anchors consumed by `target-counter`; skip `href="#"` and attach a position — verify: the placeholder-link document is accepted by `Strict`.
- [x] 3.2 Filter warnings out of `Diagnostics.Err()` and `Error.Diagnostics` — verify: the error+warning document exposes one error entry and both warnings in the result.

## 4. Attributes

- [x] 4.1 Validate attributes against the documented allow-list — verify: `data-x` and `onclick` are reported.
- [x] 4.2 Map `hidden` to `display: none` — verify: the hidden subtree is absent from the extracted text.
- [x] 4.3 Update `docs/profile.md` with the attribute list and the `href` entry — verify: every allowed attribute in the code appears in the document.
