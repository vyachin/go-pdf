# Proposal

## Why

`rowspan` and `colspan` are parsed with `strconv.Atoi` and stored as unbounded `int` values on the box. On a 64-bit platform a span such as `4294967296` parses successfully, is converted to `int32` as `0` during layout, and `rowHeight` divides by it: `runtime error: integer divide by zero`. Validation accepts the input, so supplied HTML can terminate the process.

## What Changes

- Spans SHALL be validated at parse time and bounded to a small maximum; a value outside `1..maxTableSpan` MUST produce an error diagnostic naming the attribute and its position.
- Layout SHALL clamp every span it uses, so a malformed box can never reach unchecked arithmetic or an unbounded grid allocation.

Out of scope: the remaining review findings.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `document-layout`: adds the span bounds requirement.

## Impact

- **Code**: `internal/layout/box.go` (span parsing and diagnostics), `internal/layout/table.go` (grid and arithmetic clamps).
- **Behaviour**: documents with an out-of-range span are rejected with a diagnostic instead of crashing.
