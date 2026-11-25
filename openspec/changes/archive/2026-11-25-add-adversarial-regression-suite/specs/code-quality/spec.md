# Spec Delta

## ADDED Requirements

### Requirement: Hostile-template fixtures run in the required gate

The unit and race jobs SHALL run the adversarial fixtures. The gate MUST fail when a
fixture is missing, skipped or ends without its expected diagnostic. The fixtures MUST
use the work budget and context cancellation as their pass criterion, consistent with
the ban on elapsed-time thresholds.

#### Scenario: A missing fixture fails the gate

- **WHEN** a fixture listed by the adversarial corpus is absent
- **THEN** the coverage test fails and names it

#### Scenario: A skipped fixture fails the gate

- **WHEN** a fixture skips because a tool is missing
- **THEN** the gate fails, as with the existing golden job

#### Scenario: The budget, not the clock

- **WHEN** a fixture runs on a slow machine
- **THEN** it still passes or fails on its diagnostic, not on elapsed time
