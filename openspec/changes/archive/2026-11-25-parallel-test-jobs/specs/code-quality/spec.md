# Spec Delta

## ADDED Requirements

### Requirement: Test work runs as independent jobs behind one gate

The test work SHALL run as several jobs that start without waiting for each other, each reporting its own check. A gate job MUST depend on all of them, MUST fail when any is not successful — including a skipped one — and MUST be the single check branch protection requires.

#### Scenario: Jobs start together

- **WHEN** a workflow run starts
- **THEN** the unit, race, golden and fuzz jobs start without waiting for one another

#### Scenario: Every job reports its own result

- **WHEN** the run finishes
- **THEN** each job has its own check with its own result, and a failure names the job that failed

#### Scenario: A failure blocks the gate

- **WHEN** one job fails
- **THEN** the gate fails even if every other job succeeded

#### Scenario: A skip blocks the gate

- **WHEN** a job is skipped
- **THEN** the gate fails, because a suite that never ran is not a gate

#### Scenario: One required check

- **WHEN** branch protection is reviewed
- **THEN** it requires the gate, and `CONTRIBUTING.md` names that check

### Requirement: Rendering jobs share the pinned image

A job that rasterizes pages for a golden comparison SHALL run on the pinned runner image with the pinned poppler package, and MUST NOT use a floating image label. Moving either pin MUST regenerate the goldens in the same change.

#### Scenario: The pinned image renders

- **WHEN** the golden job runs
- **THEN** it uses the `ubuntu-24.04` image and the poppler package recorded in the workflow

#### Scenario: No floating label renders

- **WHEN** a workflow job rasterizes pages
- **THEN** its `runs-on` names the pinned image, not `ubuntu-latest`

#### Scenario: A pin moves

- **WHEN** the runner image or the poppler pin moves
- **THEN** the same change regenerates the render goldens

## MODIFIED Requirements

### Requirement: Separate CI quality job

Continuous integration SHALL run formatting and linting in a job separate from the test jobs, on every push to `main` and on every pull request. The test jobs MUST NOT install or invoke the linter.

#### Scenario: Parallel feedback

- **WHEN** a push to `main` or a pull request triggers the workflow
- **THEN** the lint job runs independently of the test jobs and reports its own result

#### Scenario: Failing quality check

- **WHEN** the lint job fails on a pull request
- **THEN** the pull request shows a failing check for code quality
