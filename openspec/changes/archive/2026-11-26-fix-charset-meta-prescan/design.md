# Design

## Context

`declaredCharset` lowercases a copy of the first 1024 bytes, finds `<meta` by substring,
cuts to the next `>` and lets `charsetLabel` return the first literal `charset` in that
slice. It cannot tell markup from text, so a comment or an attribute value decides the
document encoding. Measured: `<!-- <meta charset="utf-8"> -->` plus windows-1251 bytes
reports `unsupported document encoding "utf-8"` at 1:6, and a description attribute
containing "charset is a word" reports `"is"`.

## Goals / Non-Goals

**Goals:**

- Only a real `charset` attribute or `http-equiv="content-type"` `content` declaration
  influences detection.
- Comments, doctypes, processing instructions, non-`meta` tags and other attribute
  values are ignored.
- Offsets and columns in diagnostics stay in the original bytes.
- The 1024-byte window, the precedence order and the windows-1252 rule are unchanged.

**Non-Goals:**

- Reimplementing the full HTML tree construction ahead of decoding.
- Supporting encoding labels outside the closed set.
- Changing how a caller-named `Options.Charset` or a BOM is applied.

## Decisions

**A purpose-built ASCII scanner, not `x/net/html`.** The prescan runs before decoding and
must preserve byte offsets; the tokenizer decodes entities and produces a tree for an
input whose encoding is not yet known. A small scanner that skips `<!--...-->`, `<!...>`,
`<?...>` and then parses `<meta ...>` attributes is easier to bound and to test for
offset fidelity. Alternative: reuse `x/net/html` with a byte reader; rejected because its
error and token position model does not preserve the original offsets for this purpose.

**Parse attributes instead of searching for the word.** After `<meta`, read
space-separated `name=value` pairs with quoted or unquoted values, ASCII-fold the name
and the value where the value is a keyword. `charset` is a declaration only as an
attribute name; `http-equiv` is a declaration only with an ASCII case-insensitive value
of `content-type` and a `content` value containing `charset=`. A `>` inside a quoted value
does not end the tag.

**Ignore a leading BOM and keep the 1024-byte window.** The BOM is removed before this
scan by the existing decoder path; the scanner starts after it and counts the window from
the first byte that can be markup. The window stays 1024 bytes, as the HTML standard and
`docs/profile.md` state.

**One tolerance rule.** An unterminated comment or tag consumes the rest of the window and
yields no declaration; it never panics and never falls back to a substring search. The
document is then decoded by the next rule in the order.

## Risks / Trade-offs

- **A real declaration written with unusual markup is missed.** The fallback is the
  documented "no declaration" path, and the new tests cover the forms the profile accepts;
  the old substring scan's false positives were the larger correctness problem.
- **The scanner is security-sensitive.** It is bounded by the 1024-byte window, has no
  allocation proportional to input, and gets a fuzz seed for comment and attribute shapes.
- **Detection changes for documents that relied on a commented declaration.** Those
  documents were diagnosed with a message about an encoding they did not declare; the new
  behaviour follows the specification.

## Migration Plan

No API change. A legacy document whose only apparent declaration was inside a comment or
an attribute value now fails with the "not valid UTF-8 at the first invalid byte" message
(or decodes as UTF-8 when valid) instead of naming an unrelated encoding. The
`CHANGELOG.md` `Unreleased / Fixed` entry states that only a real meta declaration counts.
