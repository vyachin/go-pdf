# Proposal

## Why

The review found four P1 defects and a set of P2 defects that the existing suite did
not catch: the only adjacent-combinator regression is descendant-only, the golden and
fuzz layers never feed a deeply nested `@media`, a many-spelling image or a crafted
GPOS table, and `TODO.md` states that there are no open tasks. A fix without a
permanent fixture will regress silently, and a suite that asserts wall-clock time is
forbidden by `code-quality`.

## What Changes

- Add an adversarial fixture set: one per confirmed P1/P2, each asserting a
  diagnostic code or a byte-identical result under a fixed operation budget.
- Run the fixtures in the unit and race jobs; a crash or a non-terminating case is a
  failure, detected by the work guard, not by a performance threshold.
- Add a subprocess helper for cases where the failure mode is a fatal runtime error.
- Record the review pass and its fixtures in `TODO.md`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `golden-tests`: an adversarial corpus asserts bounded outcomes.
- `code-quality`: the hostile-template fixtures are part of the required gate.

## Impact

- **Code**: `internal/engine`, `internal/css`, `internal/font`, `internal/layout`
  test files; a shared test helper for budget assertions.
- **CI**: no new runner; the fixtures run in the existing unit and race jobs.
- **Docs**: `TODO.md`, `CONTRIBUTING.md` and the test-layer section of the docs.
