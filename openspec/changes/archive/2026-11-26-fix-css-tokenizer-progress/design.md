# Design

## Context

See `proposal.md` — Why. `internal/css` has a hand-written tokenizer whose `next`
returns a token and the next offset. `scanIdent` consumes a backslash escape only
when a byte follows it; for a trailing lone `\` it breaks with `i == start`, so the
returned offset equals the input offset. `ParseSheet` passes a guard into
`TokenizeWithGuard`, but `ParseDeclarations` — used for every `style` attribute and
for validation — calls `Tokenize` with no guard, which makes the loop unbounded.

## Goals / Non-Goals

**Goals:**

- Every byte sequence produces a terminating parse with a diagnostic for malformed
  input; no tokenizer or declaration loop can fail to advance.
- Inline declaration parsing is charged to the shared guard and observes
  cancellation like stylesheet parsing already does.
- Keep valid CSS and every existing diagnostic code and position unchanged.

**Non-Goals:**

- General CSS error recovery or a rewrite of the tokenizer state machine.
- Changing how escapes are decoded for well-formed input.
- Capping or restructuring the problem list; that is RV5-25's scope.

## Decisions

**Put the invariant in the lexer, not only in `scanIdent`.** `lexer.next` is the
single place every caller passes through. A defensive check there (if the returned
offset did not advance, consume one byte, emit a `DelimToken` or record a
`css/syntax` problem) guarantees progress for every token kind, not just
identifiers. Alternative considered: patch `scanIdent` alone to consume the
backslash. Rejected because other scanning helpers could acquire the same defect and
the caller would still have no guarantee; the invariant test can then cover the
lexer as a whole.

**Report `css/syntax` for a trailing lone backslash.** CSS defines a backslash at
end of input as an invalid escape; a diagnostic names the construct. Alternative
considered: silently consume it as a delimiter. Rejected because silent recovery is
exactly what the profile forbids, and the author should see the typo.

**Thread the conversion guard into `ParseDeclarations`.** The guard already carries
cancellation and the work limit; passing it keeps one accounting mechanism across
stylesheets and attributes. Alternative considered: rely on the problem cap only.
Rejected: the tokenizer appends tokens before any problem is recorded, so a cap
does not bound the spinning or the allocation, and cancellation would still be
unobserved.

## Risks / Trade-offs

- **A malformed style attribute changes from a hang to an error** → intended; the
  input was never renderable. Documented as a fix, no migration needed.
- **Charging tokens changes the budget spent per attribute** → an attribute that
  previously parsed "for free" now consumes units, which is the documented cost
  model; the default budget is far above any realistic declaration list.
- **An extra guard parameter touches every declaration call site** → a mechanical
  change; the tokenizer invariant test and the existing `style` attribute suite
  catch a missed call site only through the budget test, so the parser test names
  each entry point.

## Migration Plan

Not applicable: internal parser fix, no public API, no stored data and no output
format change. Release note under `Fixed`.
