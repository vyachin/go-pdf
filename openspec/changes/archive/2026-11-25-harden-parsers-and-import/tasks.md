# Tasks

## 1. HTML parsing

- [x] 1.1 Add an iterative depth walk over the tree `html.Parse` returns, before the conversion model, and return a `resource/limit`-wrapped error when the deepest element exceeds `MaxDOMDepth`; charge one unit per visited element — verify: `<p><div></p>` ×20 with `MaxDOMDepth: 8` fails, 300 sibling `<p>` with 256 converts, and a small budget stops the walk.
- [x] 1.2 Replace the token-stack depth check with the sound early run counter (never-implicitly-closed tags, reset on end tags, disabled by table/foreign/unsupported) — verify: the large nested-div document still fails before the tree is built and a table document is not rejected by the early rule.
- [x] 1.3 Cover the false-positive shapes: 300 sibling `<p>`, repeated `body`/`head`/`main`, tables without end tags — verify: each converts or fails with the diagnostic its real content calls for.
- [x] 1.4 Grow the intern table (or return a dedicated limit error) instead of returning 0 — verify: the 70,000-tag flat document parses without a nesting diagnostic.
- [x] 1.5 Wrap x/net tree-construction errors as `resource/limit` with a position — verify: the 600-deep `<b><div></b>` document reports the coded diagnostic.
- [x] 1.6 Strip a UTF-8 BOM when the caller names an encoding — verify: the windows-1251 + BOM fixture does not start with "п»ї".

## 2. CSS values

- [x] 2.1 Decode hex escapes and escaped newlines in the tokenizer — verify: `"\2014"`, `"\41"` and `"A\20 B"` render the intended text.
- [x] 2.2 Make selector-list splitting token-aware — verify: `p[title="("], div` keeps the `div` part.
- [x] 2.3 Accept `rgb()`/hex colours in `border` and paint `transparent` as nothing — verify: both border scenarios pass.

## 3. Browser import

- [x] 3.1 Resolve `em` border widths before subtracting from a border-box dimension — verify: the 500 pt / 1em case imports 476 pt.
- [x] 3.2 Parse longhands when the `margin` shorthand fails and report dropped auto/negative margins — verify: both margin scenarios produce the new warnings.

## 4. Oracle, corpus and docs

- [x] 4.1 Add a differential depth test over implied end tags, implied `tbody`, duplicate `html`/`body`/`head`/`main`, stray end tags, raw-text and foster parenting — verify: the enforced depth equals an independently computed tree depth for every case.
- [x] 4.2 Re-measure `testdata/mpdf/cases.json` and reclassify documents whose accepted/rejected status changes — verify: the corpus test passes and each move is explained.
- [x] 4.3 Add every reproduction as a regression and a fuzz seed; update `docs/profile.md` and `docs/browser-import.md` — verify: each test fails before its fix and the documented behaviour matches the scenarios.
