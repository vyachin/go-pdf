# Proposal

## Why

`scanIdent` returns `next == start` when an identifier ends with a lone backslash,
because the loop treats `\` as an escape only when a byte follows it and otherwise
breaks without consuming anything. `lexer.next` then returns an empty identifier
token and the offset never moves. The consequence is unbounded work on a two-byte
input: a style attribute ending in `\` makes `Convert` and `Validate` hang forever,
because declaration parsing tokenizes without a guard, and a `<style>` element
ending in `\` spends the whole `MaxOperations` budget one empty token at a time
while appending roughly a gigabyte of tokens. Both violate the package's bounded
work and no-hang contract, and both are reachable from a template alone.

## What Changes

- Enforce a progress invariant in the tokenizer: every step either consumes at
  least one byte or stops the parse with a `css/syntax` problem. A trailing lone
  backslash is reported instead of looping.
- Thread the conversion's work guard and cancellation through declaration parsing,
  so parsing an inline `style` attribute charges tokens and stops with
  `resource/limit` instead of spinning.
- Add regression tests at the tokenizer, declaration-parser and public-conversion
  levels, with the public cases registered in the adversarial corpus.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: the profile gains an explicit requirement that tokenization always
  makes progress and that declaration parsing is charged and cancellable, so no
  input can hang or spend work without a bound.

## Impact

- `internal/css/token.go` (`scanIdent`, `lexer.next`).
- `internal/css/parse.go` (`ParseDeclarations`, tokenizer call sites).
- `internal/css/css_test.go`, `internal/css/harden_test.go`.
- `testdata/adversarial/` fixtures and `internal/engine/adversarial_test.go`.
- No public API or output format change. Previously hanging input now fails with
  `css/syntax` or `resource/limit`; well-formed CSS is unaffected.
