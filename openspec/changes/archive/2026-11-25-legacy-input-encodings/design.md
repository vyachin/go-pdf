# Design

## Context

Every byte of a document enters the engine at one point: `engine.prepare` calls
`html.ParseWithOptions(src, ...)`, which runs the position scan and the standard
tree construction over the same `src`. `html.Document` stores `src` and the line
starts of `src`, and `Document.Position(offset)` turns an offset into a line and
a column. Every diagnostic, every element and every text node uses that offset.

Inline `<style>` is parsed with `Base = textNode.Start` and
`Locate = doc.Position`, so CSS offsets are absolute document offsets. An
external stylesheet is parsed from its own bytes with its own line index and its
own `Position`. `fsx` already resolves and confines file reads through
`os.Root`.

Two failures define the problem (both reproduced through the reference command):

- a windows-1251 document without a declaration fails with
  `text/glyph` for U+FFFD — the tokenizer turned every high byte into the
  replacement character and the font had no glyph for it;
- a UTF-8 document with a byte order mark fails with `text/glyph` for U+FEFF —
  the tokenizer made the mark the first element of the body.

`golang.org/x/net` and `golang.org/x/text` are already module requirements;
`x/text` is currently reached only from the test harness. `x/net/html/charset`
implements the HTML encoding determination and the WHATWG label table. The
project rule added by `go-core-first` says its implementation wins over a
private one.

## Goals / Non-Goals

**Goals:**

- Decode a legacy single-byte document before it is parsed, in one place that
  both `Convert` and `Validate` pass through.
- Keep diagnostics anchored to the original file after decoding.
- Apply the same encoding to external stylesheets.
- Give the caller an explicit override for documents that declare nothing.

**Non-Goals:**

- Multibyte encodings (UTF-16, Shift-JIS, GBK). They break token boundaries and
  byte ranges, and no document in scope needs them.
- Statistical encoding detection. The engine never guesses.
- windows-1252 in automatic detection (see decision 3).
- Legacy encoding for asset paths: file names in `href` and `src` stay UTF-8.
- Moving the HTML parser, the position scan or the CSS parser.

## Decisions

### 1. Detection comes from the Go project's charset package

`charset.DetermineEncoding` handles the BOM, `<meta charset>`, the
`http-equiv` form, comments, the 1024-byte window and the fallback; `charset.Lookup`
handles every WHATWG alias, so `cp1251`, `windows-1251` and `x-cp1251` all work
without a table of our own.

*Why not a private prescan*: it is exactly the code that drifts from the
standard, and the project rule now forbids it. *Why not `charset.NewReader`*:
it decodes into a stream and hides the correspondence between source bytes and
decoded bytes, which is what the position mapper and the undefined-byte check
need. *Cost*: the package links its encoding tables in unconditionally
(`htmlindex` documents this), measured at about 830 KB on a probe binary.

### 2. Precedence: caller, then the document's own bytes, then an error

```
Options.Charset set      -> Lookup + allowlist, no detection
otherwise                -> DetermineEncoding (BOM, declaration, UTF-8, fallback)
   utf-8                 -> validate; strip a leading UTF-8 BOM
   1251 | koi8-r | ibm866 -> decode
   anything else         -> html/encoding
```

`DetermineEncoding` already implements the HTML order — BOM, then the
declaration, then a UTF-8 check, then the windows-1252 fallback — so the engine
does not reorder it. A declaration is therefore honoured even when the bytes
happen to be valid UTF-8: the document says what it is, and a second rule
("valid UTF-8 beats the declaration") would be a hidden exception that every
user would have to learn.

Two details the standard leaves to the caller:

- pure ASCII is reported as the windows-1252 fallback, and ASCII is identical in
  every supported encoding, so an ASCII document is accepted as UTF-8;
- the same fallback is what a non-ASCII document that is not valid UTF-8 gets,
  and it is indistinguishable from a declaration of windows-1252; both are
  rejected (decision 3).

*Alternative*: a valid-UTF-8 fast path before detection — rejected: it silently
overrides the document's own declaration, and it puts a second rule in front of
the Go implementation.

### 3. A closed set, without windows-1252

The set is UTF-8, windows-1251, KOI8-R and IBM866. All but UTF-8 are
single-byte, ASCII-compatible and total enough to decode byte by byte.

windows-1252 is deliberately absent. `DetermineEncoding` returns
`windows-1252` both for a document that declares it and for a document that
declares nothing and is not valid UTF-8 (`certain=false` in both cases). The two
are indistinguishable through that API. Accepting the result would silently read
an undeclared windows-1251 document as windows-1252 — Latin mojibake in a
product that promises no silent substitution. So an unsupported or undetectable
document fails with `html/encoding` and a message that names the supported set
and the caller override. A caller cannot name windows-1252 either: the
asymmetry "declared is rejected, explicit is accepted" would be harder to
explain than one closed set. A document that really is windows-1252 has to be
converted, which the error says.

### 4. A per-encoding byte table, built from the Go decoder

When an encoding is first used, a 256-entry table is built once: each source
byte is handed to the Go decoder, and the table stores the UTF-8 bytes of the
resulting rune; a U+FFFD result marks the byte as undefined and is an error at
that byte. Decoding a document is then a table lookup per byte, and the width of
every decoded byte is known, which the position mapper needs.

*Why not decode per byte at run time*: 256 decodes per conversion is wasteful,
and the table is immutable and shared, so the no-mutable-global-state guarantee
is unaffected. *Why not `transform.NewReader`*: it cannot say which source byte
produced a replacement character, and it hides the widths.

### 5. Decode inside the HTML parser, keep positions in source bytes

`html.ParseOptions` gains `Charset`; `ParseWithOptions` resolves the encoding,
decodes, stores both the decoded text and the original bytes on the `Document`,
and runs the scan and the tree construction over the decoded text. The position
scan, the node ranges and the inline `<style>` mapping stay in one coordinate
space — the decoded one.

`Document.Position` translates a decoded offset back to the source before it
returns: the line number is the same (a single-byte encoding never changes a
newline), and the column is found by walking the original line and counting the
decoded widths until the decoded column is reached. The last translated line is
cached, so a document that produces many diagnostics on one line costs one walk.

```
"<p>Привет <b>"        source bytes             decoded text
                         <p> = 3 bytes            <p> = 3 bytes
                         Привет = 6 bytes         Привет = 12 bytes
                         <b> at source col 11     <b> at decoded col 17
                                                       |
                              Document.Position(decoded 16) -> col 11
```

*Alternative*: report decoded columns — rejected, it weakens the "byte range of
the source" requirement for exactly the documents this change introduces.
*Alternative*: scan the original bytes and parse the decoded ones — rejected:
the inline `<style>` `Base` is a document offset and the two coordinate spaces
would no longer agree.

### 6. External stylesheets use the same encoding and the existing hook

`profile` reads the file through `fsx`, decodes it with the document's encoding,
and passes a `Locate` closure over its own mapper to `css.ParseSheet`. `css`
needs no new field: `SheetOptions.Locate` already exists for inline sheets.
A `@charset` rule at offset 0 is consumed by the parser: it is accepted when it
names the same encoding and produces `css/link-encoding` otherwise.

*Why decode with the document encoding*: it is the CSS 2.1 rule for a linked
sheet with no `@charset`, and it keeps one encoding per document. *Why not let
`@charset` override*: a second detector means a second decoder and a second
answer to "which encoding is this document", for a case the profile does not
need.

### 7. Diagnostics

| Code | When | Position |
|---|---|---|
| `options/charset` | the caller named an unknown or unsupported encoding | none |
| `html/encoding` | the document cannot be decoded, or declares an unsupported encoding | the offending byte, or the first `<meta` tag when the declaration is unsupported, or none |
| `css/link-encoding` | a stylesheet cannot be decoded, has an undefined byte, or has a conflicting `@charset` | the offending byte in the stylesheet |

`DetermineEncoding` does not expose the offset of the declaration, and parsing
it ourselves is the code this change removes; pointing at the first `<meta` in
the 1024-byte window is enough to find the line, and the message names the
encoding.

## Risks / Trade-offs

- **About 830 KB of encoding tables in every binary** → accepted: the project
  rule prefers the Go implementation; the library is not size-constrained and
  the tables are the price of correct detection. Recorded in the changelog.
- **windows-1252 documents are rejected** → documented in `docs/profile.md` and
  in the error message, with the supported set and the conversion advice. If it
  ever becomes a real input, the decision to distinguish "declared" from
  "fallback" is a separate change.
- **A stale declaration on a UTF-8 document becomes mojibake** → the
  document's own declaration is authoritative, as in a browser; the caller can
  override it with `Options.Charset` or fix the declaration. Reordering
  detection to repair it was rejected (decision 2).
- **Decoded size can reach three times the input** → bounded by `MaxInputBytes`
  (8 MiB), so at most 24 MiB before parsing; noted where the limit is documented.
- **Position translation on a hostile single-line document** → the one-line
  cache makes the total work linear; a test with thousands of diagnostics on one
  long line guards it.
- **Binary fixtures can be normalized by git** → a `.gitattributes` rule marks
  the new fixtures `-text`, as the weasyprint corpus already is.
- **Existing output must not change for UTF-8** → a UTF-8 document without a
  declaration still resolves to UTF-8 and passes through undecoded; golden tests
  must stay byte identical, and that is the first check.

## Migration Plan

The change is additive and needs no data migration. A document that used to
fail now converts, or fails with a more precise code. Rolling back means
reverting the commit: the new codes disappear and the old failure modes return.
