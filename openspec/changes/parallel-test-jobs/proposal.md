# Proposal

## Why

The `test` job is one sequential chain: vet, the unit suite, the race detector, both golden layers, five fuzz sessions and a build from another module. None of these depends on another, yet a pull request waits for their sum. The race detector, the pixel goldens and the fuzz sessions are the long poles, and they would finish much earlier if they ran side by side. The linter already has its own job for the same reason.

## What Changes

- The test work SHALL split into independent jobs: a unit job (vet, the reference-command import check, the external-module build and the suite with the validator), a race job, a golden job (poppler, both snapshot layers and the failure artifacts), a fuzz job, and the existing lint job.
- Poppler SHALL be installed only in the golden job. `pdfcpu` SHALL be installed in the jobs that require the validator. The render tests SHALL be required only in the golden job, so the unit job does not pay for a rasterizer it does not need.
- A gate job SHALL depend on every job, fail when any is not successful — including a skipped one — and SHALL be the single check branch protection requires.
- The golden job SHALL keep the pinned `ubuntu-24.04` image and poppler; no job that rasterizes pages may use a floating image label.
- `CONTRIBUTING.md` SHALL describe the job map, the gate and the required check.

Not included: a nightly fuzz schedule, coverage reporting, a build matrix, self-hosted runners, and enabling the Go module cache — considered and left off because the module download is two small modules and parallel jobs would race to save the same cache.

## Capabilities

### Modified Capabilities

- `code-quality`: requirements for independent test jobs behind one gate and for a shared rendering image, and the quality-job wording follows the plural.

## Impact

- **CI**: `.github/workflows/ci.yml` restructured; branch protection moves from `test` to the gate — a repository setting.
- **Time**: the pull-request wall clock becomes the slowest single concern instead of the sum; total runner-minutes rise slightly from repeated checkout and setup.
- **Documentation**: `CONTRIBUTING.md`.
- **Product code**: none.
