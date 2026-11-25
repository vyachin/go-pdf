# Tasks

## 1. Trigger configuration

- [x] 1.1 Limit the `push` trigger to `main` and keep `pull_request` unfiltered in `.github/workflows/ci.yml`; verify the parsed `on:` block lists exactly those two events and that the job names are unchanged
- [x] 1.2 Verify the change on the open pull request: pushing a commit to its branch starts exactly one run, and no run is attributed to the feature-branch `push` event
- [x] 1.3 Verify statically that the `on:` block can match only a push to `main` and a pull request: `push` is branch-filtered, and no `tags`, `schedule` or `workflow_dispatch` key exists

## Workflow follow-up

- After the merge, confirm the merge commit on `main` started exactly one run and that no run is attributed to the feature-branch `push` event.
