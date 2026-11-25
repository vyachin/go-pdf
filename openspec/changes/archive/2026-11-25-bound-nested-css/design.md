# Design

## Context

`parseRules` and `parseAtRule` are mutually recursive, and `RulesForPrint` and
`validateRules` recurse again over the same tree. A cap in one place would leave the
others exposed. The parser already produces `css.Problem` values, which validation
turns into `css/syntax`; the new failure needs its own `resource/limit` code.

## Goals / Non-Goals

**Goals:** a fixed, documented depth limit that applies to parsing and every later
walk; a diagnostic instead of a fatal error.

**Non-Goals:** making the parser fully iterative; changing the accepted grammar for
stylesheets within the limit.

## Decisions

- Cap at 64 nested at-rules: deep enough for real templates, small enough that the
  recursive walks are safe (64 frames, not 500,000).
- Carry a depth counter on the parser and return a `resource/limit` problem when it
  exceeds the cap, so the error has a source offset.
- Make the two walkers iterative with an explicit stack; a second cap would duplicate
  the rule and could drift.
- Test the crash in a subprocess because `runtime.throw` cannot be recovered in-process;
  assert exit 0 and a diagnostic.

## Risks / Trade-offs

- A legitimate generated stylesheet deeper than 64 would now fail → document the limit
  and name it in the diagnostic.
- Iterative walkers change traversal order if written carelessly → keep document order
  and add a snapshot test for a nested `@media` fixture.
