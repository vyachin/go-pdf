# Design

## Context

See `proposal.md` — Why. The five cases share one property: validation and rendering
disagree about a value the profile says it supports. `line-height` parses into a
length kind that the positivity check only applies to one kind; border colours are
initialised to a concrete black instead of the CSS-wide `currentColor`; inline
declarations carry no stylesheet, so the cascade's position lookup has nothing to
report; page margin boxes are parsed by the generic at-rule path without knowing
their parent; and the parser counts a problem when it records it and again when a
nested block is merged.

## Goals / Non-Goals

**Goals:**

- Every value the profile accepts is either rendered with the declared semantics or
  rejected with a code and a position.
- The initial border colour follows the element's `color`, and `currentColor` is a
  first-class value.
- A diagnostic always names the position it can know.
- A margin box paints only where the profile defines it.
- The problem cap counts problems, not parser events.

**Non-Goals:**

- Supporting further CSS colour or length features beyond `currentColor`.
- Changing the profiled set of margin-box names or their content model.
- Replacing the problem cap with a different diagnostic model.
- Changing any existing diagnostic code.

## Decisions

**Validate `line-height` after unit resolution.** The check moves to the resolved
length, so percent, `em` and absolute forms share one rule. Alternative considered:
add a second check for `Percent` only. Rejected because `-2em` would still pass
through the `Em` kind, which is the same hole one branch over.

**Resolve `currentColor` at style resolution, not at parse time.** A border keeps a
sentinel until the cascade has the element's final `color`, so declaration order in
the same rule cannot decide the answer. Alternative considered: substitute `color`
while parsing `border`. Rejected because `border:1px solid` may precede `color` in
the same rule, and a shorthand has no access to the cascade.

**Record the owning element's position on inline declarations.** The declaration set
carries the position of the element (or of its `style` attribute) into validation, so
the cascade's position lookup never falls back to zero for an inline problem.
Alternative considered: synthesise a one-declaration stylesheet with a fake file
name. Rejected because it would pollute stylesheet identity and the reported file.

**Pass the parent at-rule context to margin-box parsing.** The parser knows whether
it is inside `@page`, so an out-of-place margin box becomes `css/unsupported-at-rule`
at parse time with the rule's own position. Alternative considered: reject it later
in profile validation by walking the rule tree. Rejected because a walker has to
reconstruct nesting and loses the position the parser already has.

**Count problems in one place.** A single append helper increments the counter, and
merging a nested block only appends the already-counted problems. Alternative
considered: cap only in the merge step. Rejected because local problems still pass
through two counters and the off-by-N stays.

## Risks / Trade-offs

- **Borders without an explicit colour change appearance** → correct per CSS; the
  profile document and the golden snapshots that contain such borders must be
  regenerated in the same review, which the tasks make explicit.
- **Two new rejection paths can fail previously accepted templates** → both are
  malformed per CSS; the diagnostics name the declaration and the position.
- **An inline position is the element's, not the declaration's** → the attribute has
  a position, but a per-declaration column inside it is not tracked; naming the
  element is strictly better than `:0:0`, and the design keeps the door open to a
  finer offset later.
- **The cap fix changes the number of diagnostics for existing malformed sheets** →
  tests that asserted a truncated count must move to the true count; the cap
  scenario keeps the bounded behaviour.

## Migration Plan

Not applicable beyond template changes: replace a negative `line-height`, give a
border an explicit colour if the new default is unwanted, and move a margin box
inside `@page`. `docs/profile.md` is updated in the same change and the changelog
entry names the two new rejections and the border default.
