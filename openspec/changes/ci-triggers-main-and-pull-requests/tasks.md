# Tasks

## 1. Trigger configuration

- [ ] 1.1 Limit the `push` trigger to `main` and keep `pull_request` unfiltered in `.github/workflows/ci.yml`; verify the parsed `on:` block lists exactly those two events and that the job names are unchanged
- [ ] 1.2 Verify the change on the open pull request: pushing a commit to its branch starts exactly one run, and no run is attributed to the feature-branch `push` event
- [ ] 1.3 Verify a commit on `main` after the merge starts exactly one run and that tags start none
