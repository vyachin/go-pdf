# Spec Delta

## ADDED Requirements

### Requirement: An adversarial corpus asserts bounded outcomes

The suite SHALL keep one fixture per confirmed hostile or pathological case. Each
fixture MUST assert a specific diagnostic code or a successful result under a fixed
operation budget; a crash, a skipped fixture or a case that ends only by exhausting the
harness is a failure. The pass criterion MUST be the work guard's diagnostic, not
elapsed time.

#### Scenario: A known bomb is rejected, not run

- **WHEN** the suite runs the crafted GPOS, many-spelling image and nested-media
  fixtures under small budgets
- **THEN** each returns its expected diagnostic
- **AND** no test relies on a wall-clock threshold

#### Scenario: The corpus covers every confirmed defect

- **WHEN** the review's P1 and P2 findings are enumerated
- **THEN** each has a fixture named in the corpus manifest

### Requirement: Fatal runtime errors are caught in a subprocess

A fixture whose failure mode is a fatal runtime error, such as a stack overflow, SHALL
run in a subprocess that asserts a normal exit with a diagnostic. The corpus MUST grow
when a supported construct or a limit is added.

#### Scenario: A process-killing input

- **WHEN** the nested-media fixture is executed in the subprocess helper
- **THEN** the process exits 0 with a diagnostic

#### Scenario: A new limit needs a fixture

- **WHEN** a supported construct or a limit is added to the profile
- **THEN** the coverage check requires a fixture that exercises it
