# Spec Delta

## MODIFIED Requirements

### Requirement: Static analysis gate

The repository SHALL keep a committed linter configuration and run it over all
packages on every push to `main` and on every pull request. The gate MUST fail
when the linter reports an issue.

#### Scenario: New lint issue

- **WHEN** a change introduces a finding reported by the configured linter
- **THEN** the lint job fails and the finding names the file, position and check

#### Scenario: Clean tree

- **WHEN** the linter reports no findings for the whole module
- **THEN** the lint job succeeds

### Requirement: Separate CI quality job

Continuous integration SHALL run formatting and linting in a job separate from
the test job, on every push to `main` and on every pull request. The test job
MUST NOT install or invoke the linter.

#### Scenario: Parallel feedback

- **WHEN** a push to `main` or a pull request triggers the workflow
- **THEN** the lint job runs independently of the test job and reports its own
  result

#### Scenario: Failing quality check

- **WHEN** the lint job fails on a pull request
- **THEN** the pull request shows a failing check for code quality

## ADDED Requirements

### Requirement: One workflow run per change

A change covered by a pull request SHALL be verified by that pull request's run
alone: a push to a branch other than `main` MUST NOT start a workflow run of its
own. A commit that lands on `main` MUST start exactly one run.

#### Scenario: A push to a feature branch

- **WHEN** a commit is pushed to a branch that has an open pull request
- **THEN** only the pull request run starts, and no `push` run appears for that
  commit

#### Scenario: A commit on the integration branch

- **WHEN** a commit lands on `main`, whether pushed directly or as a merged pull
  request
- **THEN** exactly one run starts for that commit

#### Scenario: A pull request against another base

- **WHEN** a pull request targets a branch other than `main`
- **THEN** its run still starts, because every pull request is verified

### Requirement: Trigger configuration is the only schedule

The workflow SHALL NOT start on a tag, a schedule or any event other than the
push to `main` and the pull request events, so that the set of runs matches the
set of changes under review or just landed.

#### Scenario: A tag is pushed

- **WHEN** a release tag is pushed to the repository
- **THEN** the workflow does not start; the commit it points at was already
  verified on `main`

#### Scenario: The configuration is read

- **WHEN** a maintainer reads the `on:` block of `.github/workflows/ci.yml`
- **THEN** it lists exactly the `push` restricted to `main` and the
  `pull_request` event
