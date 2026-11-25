# Design

## Context

`.github/workflows/ci.yml` has two jobs. `test` runs, in order: checkout, setup-go, install poppler, install `pdfcpu@v0.16.4`, add `$(go env GOPATH)/bin` to `PATH`, `go vet`, the reference-command import check, `go test ./...` with `PDF_REQUIRE_VALIDATOR=1` and `PDF_REQUIRE_RENDER=1`, `go test -race ./...` with the same, the explicit golden step (which re-runs the engine snapshot tests and fails on `--- SKIP`), an artifact upload on failure, five fuzz sessions totalling 65 seconds, and a build from a temporary module. `lint` is separate and runs `gofmt` and the pinned `golangci-lint` on `ubuntu-latest`.

The `test` job is the required check in branch protection; `lint` is expected to be required too. The runner image for rendering is pinned to `ubuntu-24.04` because it defines the poppler and therefore the golden pixels. `setup-go` deliberately disables its cache.

Every step after the installs is independent: the race detector does not read the golden output, the fuzz sessions do not read the race result, and the external build shares nothing with either. Their wall-clock costs add only because one job runs them in sequence.

See `proposal.md` for motivation.

## Goals / Non-Goals

**Goals:**

- A pull request waits for the slowest concern, not for the sum.
- Every check that gates a merge today still gates it, with the same strictness.
- One required check remains, so branch protection does not grow with the job count.

**Non-Goals:**

- A nightly fuzz schedule, coverage reporting, a build matrix or self-hosted runners.
- Enabling the Go module cache.
- Changing triggers, concurrency or the pin set.

## Decisions

### 1. Five test jobs and a gate

| Job | Content | Tools | Timeout |
|---|---|---|---|
| `unit` | `go vet`, the reference-command import check, the external-module build, `go test ./...` with the validator | Go, `pdfcpu` | 15 |
| `race` | `go test -race ./...` with the validator | Go, `pdfcpu` | 20 |
| `golden` | the explicit document, render and profile snapshot step, artifacts on failure | Go, poppler, `pdfcpu` | 20 |
| `fuzz` | the five fuzz sessions | Go | 15 |
| `lint` | `gofmt` and the pinned `golangci-lint`, unchanged | Go, linter | 10 |
| `gate` | `needs` every job, checks their results | none | 5 |

The boundaries follow tool dependencies, not steps. Poppler belongs only to `golden`; `pdfcpu` belongs to the three jobs that set `PDF_REQUIRE_VALIDATOR=1`; `fuzz` needs nothing. One job per step would multiply setup for no wall-clock gain, and one job per package would duplicate compilation.

`unit` keeps vet, the import check and the external build instead of a separate `build` job: those steps cost seconds, and `unit` is not the critical path, so separating them would add a setup without shortening the run.

### 2. The render tests are required only in `golden`

`unit` and `race` drop `PDF_REQUIRE_RENDER=1`. Without poppler installed, the render tests skip there; the document and profile snapshots still run. `golden` keeps `PDF_REQUIRE_RENDER=1`, the `--- SKIP` grep and the explicit `--- PASS` greps, so the strictness that exists today moves to the job that owns it rather than disappearing.

*Why not install poppler everywhere*: the package list and the `apt-get update` cost the most of any setup step, and only one job reads a raster.

### 3. `pdfcpu` is installed per job that validates

The install takes tens of seconds and cannot be shared without an artifact barrier that would make `unit`, `race` and `golden` wait for a `tools` job. In parallel jobs the duplicate installs cost runner-minutes, not wall clock, and the pinned version stays in one place per job.

### 4. The gate is the only required check

```yaml
gate:
  needs: [unit, race, golden, fuzz, lint]
  if: always()
  runs-on: ubuntu-24.04
  steps:
    - run: |
        for result in ${{ join(needs.*.result, ' ') }}; do
          [ "$result" = success ] || exit 1
        done
```

A gate rather than five required checks: branch protection is a repository setting, and one check keeps it stable when jobs are added later. `if: always()` makes the gate run even when a dependency failed; the result loop treats `skipped`, `cancelled` and `failure` alike, so a suite that never ran cannot pass.

### 5. Timeouts and images

Each job gets a timeout with headroom over its expected cost, so a hung job fails in minutes rather than at the twenty-minute limit. `golden` stays on the pinned `ubuntu-24.04`; `unit`, `race` and `fuzz` use the same image for one less variable; `lint` keeps `ubuntu-latest` because it never rasterizes.

### 6. The Go module cache stays off

`setup-go` keeps `cache: false`. The module graph is two small modules, and six jobs restoring and racing to save the same cache would add a failure mode for a few seconds each. The comment is updated to say this, replacing the old one-dependency wording. If a later measurement shows module download dominating a job, a cache is a separate change.

### 7. What is measured

The task list records the wall clock of a representative run before and after. The design does not promise a number: the critical path becomes `max(race, golden, fuzz)` instead of their sum plus setup, and the first runs show which of the three dominates.

## Risks / Trade-offs

- **Branch protection still requires `test`** → the check disappears when the job is renamed. Update the setting in the same change and record the gate name in `CONTRIBUTING.md`.
- **A skipped job passes** → the result loop treats anything but `success` as failure; a task forces a skip to prove it.
- **Render tests silently skip in `unit`** → intended, and `golden` owns them with the `--- SKIP` grep; the task verifies the grep still fails on a missing rasterizer.
- **Timeout too tight** → headroom in every value; adjust after the first runs.
- **More runner-minutes** → accepted: the split trades total minutes for wall clock, which is what a pull request waits on.
- **`fuzz` becomes the critical path** → it is a fixed 65 seconds today; if it grows, longer sessions move to a nightly workflow, which is already named as out of scope here.
- **Artifact name collisions** → only `golden` uploads, and only on failure.

## Migration Plan

The workflow change and the branch-protection update land together; until the setting names `gate`, pull requests wait for a check that no longer reports. Reverting restores the single `test` job and the old required name.
