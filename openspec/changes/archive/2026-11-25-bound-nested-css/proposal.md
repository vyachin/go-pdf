# Proposal

## Why

The CSS parser recurses once per nested at-rule (`parseRules` → `parseAtRule` →
`parseRules`) and enforces no depth limit. A 4.5 MB inline stylesheet of `@media{`
repeated 500,000 times — below the default 8 MiB input limit — dies with
`fatal error: stack overflow`; `runtime.throw` cannot be recovered, so a hostile
template kills the host process. `RulesForPrint` and profile validation recurse over
the same structure, so a cap in the tokenizer alone would not be enough.

## What Changes

- The parser SHALL reject at-rule nesting deeper than 64 levels with a
  `resource/limit` diagnostic, before any recursive walk over the rules.
- Every walk over parsed rules SHALL either be iterative or inherit the same cap.
- A subprocess regression SHALL prove that a deeply nested sheet leaves the process
  alive with a diagnostic.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: nested at-rules are depth-limited and cannot crash the process.

## Impact

- **Code**: `internal/css/parse.go` (depth counter, `RulesForPrint`),
  `internal/profile/profile.go` (recursive validation).
- **Output**: deeply nested sheets now fail with `resource/limit`; sheets within the
  cap are unchanged.
- **Tests**: parser-level depth test and a subprocess test that asserts exit code 0
  and a diagnostic, never a fatal error.
