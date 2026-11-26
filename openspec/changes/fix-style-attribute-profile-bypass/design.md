# Design

## Context

See `proposal.md` — Why. `ParseDeclarations` has to feed the cascade from an
attribute that contains no selector and no braces. It reuses the sheet block parser
by appending a synthetic `}` and calling `parseBlockContents`, which both treats a
real `}` as the end of the list and returns nested rules that the caller ignores.
The profile then validates only the declarations the parser returned, so anything
after the brace or inside an accepted at-rule never reaches validation.

## Goals / Non-Goals

**Goals:**

- A declaration list that ends at end of input and reports `{` and `}`.
- At-rules in an attribute are reported with `css/unsupported-at-rule` and are not
  turned into rules or discarded.
- Every declaration in the attribute is validated in the same pass, before and
  after the offending token, so no nameable problem is hidden.
- No change to how stylesheets themselves are parsed.

**Non-Goals:**

- Making a style attribute a general stylesheet container.
- Recovering a selector, a nested rule or an `@page` body from an attribute.
- Changing the severity or the code of existing declaration diagnostics.

## Decisions

**Add an explicit declaration-list mode instead of another sentinel.** The block
parser gets a mode in which end of input, not `RBraceToken`, terminates the list,
and a real brace becomes a problem. Alternative considered: keep the sentinel and
strip braces from the source before tokenizing. Rejected because stripping shifts
every later offset, so diagnostic columns would point at the wrong character, and
it would still silently discard nested rules.

**Report and resume after a brace.** The parser records the problem, skips the
offending token, and keeps parsing to end of input so a second unsupported
declaration is named in the same pass. Alternative considered: stop at the first
brace. Rejected because the profile promises every problem in one pass and the
current behaviour already collects several diagnostics at once.

**An at-rule is unsupported in whole, not descended into.** `@media print { ... }`
in an attribute is reported once; the declarations it wraps are not applied.
Alternative considered: validate the inner declarations and apply them. Rejected
because an attribute has no rule structure to attach them to, and silently
flattening a media rule would be a new semantic rather than a fix.

**Keep the declarations before the token.** They are already parsed and their
positions are valid; discarding them would add a second silent loss and would make
the diagnostic order depend on the parser's recovery point.

## Risks / Trade-offs

- **A template that accidentally contains a brace now fails** → intended; the
  failure names the attribute and the position, and such a template was never
  rendering the content it declared.
- **Resuming after a brace can produce cascading syntax problems** → each problem
  is one entry with its position and the problem cap still bounds the list; the
  parser does not invent declarations, it only reports.
- **The block parser gains a mode flag** → the sheet path must keep the identical
  behaviour; the existing stylesheet suite and the position tests are the guard.

## Migration Plan

Not applicable: no public API or stored data. A template that used the bypass must
move its at-rule into a `<style>` element or remove it. The change is noted under
`Changed` with the diagnostic codes it now reports.
