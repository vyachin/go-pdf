# Spec Delta

## ADDED Requirements

### Requirement: Elapsed time never fails the suite

The automated test suite MUST NOT fail because a wall-clock measurement exceeded a threshold; timings depend on the machine, the race detector and the runner load. Performance SHALL be reported by benchmarks and by the opt-in `PDF_PERF=1` report, which MUST NOT assert a threshold. Resource limits that count work — the `internal/budget` guard — and correctness checks SHALL remain.

#### Scenario: A slower machine

- **WHEN** the suite runs on a slower or loaded machine
- **THEN** no test fails because an elapsed-time threshold was exceeded

#### Scenario: Performance stays visible

- **WHEN** a contributor asks for the current numbers
- **THEN** the benchmarks and the `PDF_PERF=1` report log timings and allocations without failing on them

#### Scenario: Resource limits still apply

- **WHEN** a caller's operation budget is exhausted
- **THEN** the engine still reports the limit through its diagnostics and stops the work

#### Scenario: Correctness checks still fail

- **WHEN** a compared font table, a decoded position or a generated page diverges
- **THEN** the test fails as before, regardless of how long it took
