# Tasks

## 1. Prescan

- [ ] 1.1 Replace `declaredCharset`/`charsetLabel` with a scanner that skips comments, doctypes, processing instructions and non-`meta` tags inside the first 1024 bytes. Verify with unit tests for `<!-- -->`, `<!DOCTYPE html>`, `<?xml?>` and an unterminated comment.
- [ ] 1.2 Parse `<meta>` attributes (quoted and unquoted values, `>` inside a value) and accept only a `charset` attribute or `http-equiv`/`content` pair. Verify a description attribute containing the word `charset` yields no declaration.
- [ ] 1.3 Keep offsets and columns in the original bytes and start the window after a leading BOM. Verify the reported `<meta>` offset and column match the author's file, and that the existing case-folding test stays green.

## 2. Detection order and tests

- [ ] 2.1 Keep the order caller option, BOM, declaration, UTF-8, and make an ignored declaration leave detection to the next rule. Verify a BOM plus a windows-1251 meta declaration still decodes as UTF-8 with no conflict diagnostic.
- [ ] 2.2 Add engine-level tests: commented meta plus windows-1251 bytes reports the first invalid byte, not `utf-8`; a genuine `<meta charset="windows-1252">` reports `html/encoding` naming windows-1252; a genuine `http-equiv` declaration decodes windows-1251. Verify each fails on the parent commit where applicable.
- [ ] 2.3 Add the crafted comment and attribute shapes to `internal/charset` fuzz seeds or the existing parser fuzz corpus, and run the target for 20 s. Verify no panic and no allocation proportional to the input.

## 3. Documentation

- [ ] 3.1 Update the "Input encodings" section of `docs/profile.md`: only a real `<meta charset>` or `http-equiv` declaration in the first 1024 bytes counts, and markup in comments or attribute values is ignored. Verify the text matches the scanner's tests.
- [ ] 3.2 Add a `CHANGELOG.md` `Unreleased / Fixed` entry stating that a commented or attribute-value mention of `charset` no longer influences encoding detection. Verify no encoding is added to or removed from the supported set.
