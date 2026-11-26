# Tasks

## 1. One character-column implementation

- [ ] 1.1 Add `charset.Column(src []byte, lineStart, offset int) int` that counts code points and makes a leading UTF-8 BOM zero-width, and unit-test it for ASCII, Cyrillic, a BOM before text and an offset before the mark. Verify the BOM case returns the same column as the same bytes without it.
- [ ] 1.2 Route `charset.Mapper.Position`, `charset.sourcePosition` and the `Stylesheet.Position` fallback through `Column`, and make `html.Document.Position` use the same helper for the no-mapper path. Verify the four paths agree on one document with Cyrillic text.
- [ ] 1.3 Keep the mapper's incremental cursor and apply the BOM shift once, so a diagnostic after a long Cyrillic word does not rescan from the line start on every call. Verify with the existing position benchmark or a new one that cost stays linear in the source.

## 2. Tests and documentation

- [ ] 2.1 Extend `internal/engine/encoding_test.go` with a UTF-8 document whose missing image follows Cyrillic text, with and without a BOM, and assert column 14 in both. Verify the BOM case fails on the parent commit with column 20.
- [ ] 2.2 Add a linked UTF-8 stylesheet case asserting the character column for `/* Счёт */ .x { display: flex }`, and a charset error after multibyte text. Verify both report the author's column and the legacy single-byte cases keep their numbers.
- [ ] 2.3 State the rule and the BOM exception in `docs/profile.md` under "Input encodings" / "Diagnostics" and in the README sentence about columns; verify the wording matches the test assertion and the same sentence is not duplicated with different phrasing.

## 3. Compatibility of pinned positions

- [ ] 3.1 Update every golden or unit test that pins a byte column for a multibyte source, and list the changed positions in the change review. Verify the full suite, `TestCorpus` and `TestDocumentSnapshots` stay green, and no diagnostic code, message or severity changed.
- [ ] 3.2 Add a `CHANGELOG.md` `Unreleased / Fixed` entry: a UTF-8 BOM no longer shifts a diagnostic column, and linked stylesheets and charset errors count characters. Verify the entry names no API change.
