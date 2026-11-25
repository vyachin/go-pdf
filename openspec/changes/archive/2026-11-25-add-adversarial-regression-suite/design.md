# Design

## Context

`code-quality` forbids failing a test on elapsed time, and the existing suite already
omits wall-clock budgets. The P1 failures are a fatal stack overflow and a hang that
the work guard does not stop. The fixtures must therefore assert diagnostics and exit
codes, not durations.

## Goals / Non-Goals

**Goals:** every confirmed defect has a permanent, fast fixture; a crash is a test
failure; the gate cannot skip the corpus.

**Non-Goals:** measuring performance; replacing the existing golden or fuzz layers.

## Decisions

- A fixture is a small HTML/CSS/font input plus the expected diagnostic code and the
  budget that must trigger it. The helper runs `Convert` with a small
  `MaxOperations` and fails if the code is absent.
- Fatal-error cases run in a subprocess with a modest context deadline. The deadline
  is a liveness guard for a bug, not a pass criterion: a timeout means the guard did
  not stop the work, and the assertion is the diagnostic.
- The corpus has a manifest test: every finding id maps to a fixture file, and a
  missing file fails (the same shape as the profile coverage test).
- CI adds no job; the fixtures live in the packages that own the behaviour and run in
  the unit and race jobs.

## Risks / Trade-offs

- Subprocess tests are slower than unit tests → keep the inputs small (a few hundred
  kilobytes) and run them only in the engine package.
- A fixture that asserts a diagnostic may pin an implementation detail → assert the
  documented code, not the message.
