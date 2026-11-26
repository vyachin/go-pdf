# Design

## Context

See `proposal.md` — Why. `stringify` builds a selector's text from tokens whose
ident and hash text is already decoded, and `ParseSelector` then parses that text
again; a decoded `.` or space is indistinguishable from the punctuation the selector
grammar uses. `url()` goes the other way: the tokenizer keeps the raw argument text,
`trimURLQuotes` only strips quotes, so escapes arrive at the asset resolver intact.

## Goals / Non-Goals

**Goals:**

- An escaped character in a selector names the character and never changes the
  selector's structure.
- `url()` behaves like every other CSS string: escapes decode before the value is
  used.
- No change for selectors and URLs without escapes.

**Non-Goals:**

- General selector grammar work such as namespaces or `:is()`.
- Supporting characters that HTML class tokens cannot contain; the selector model
  still has to represent them, which is what the escape fixes.
- Changing how `content` and `font-family` decode escapes; they already do.

## Decisions

**Escape decoded names when building selector text.** The selector builder escapes
the punctuation that the selector grammar treats as structure (`. # , > + ~ : space`
and the backslash) before concatenating compounds. Alternative considered: keep a
raw spelling next to each token and build the selector from raw text. Rejected
because two spellings per token widen the token struct and every consumer has to
choose one; re-escaping a decoded name is a single, testable function.

**Long-term alternative, not taken now: build the selector from tokens.** Parsing
the token stream with compound boundaries preserved would remove the round trip
entirely, but it is a selector-parser rewrite and this change must stay small
enough to ship with the other RV5 fixes.

**Decode `url()` in the tokenizer.** The URL token becomes a decoded value, exactly
as strings already are, and the unquoted scanner stops at an unescaped `)`. Alternative
considered: decode in `props.go` at the use site. Rejected because `background-image`
and `@font-face src` would need duplicate decoders and would drift from `content`.

**Keep the source spelling for diagnostics only where it already exists.** Positions
come from byte offsets, not from token text, so decoding does not move any
diagnostic.

## Risks / Trade-offs

- **A selector that intentionally used a backslash before a letter** → a backslash
  is an escape in CSS, so such a selector was already decoded and misread; the fix
  makes that decode explicit.
- **A Windows path in `url()` with literal backslashes** → not valid CSS; the author
  must escape the backslash, and the decoded path is what the resolver receives.
- **A document that matched the wrong element now matches the right one (or none)**
  → output change, recorded under `Fixed`; tests pin both the positive and the
  negative case so the direction is explicit.

## Migration Plan

Not applicable: no public API, no stored data and no diagnostic code change. A
template that relied on the misparsed selector or URL is corrected by the fix; the
changelog entry states the behaviour change.
