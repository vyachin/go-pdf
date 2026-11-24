# Proposal

## Why

The repository has no automated code-quality gate. CI runs `go vet` and the test
suite, but nothing verifies formatting or runs a linter, so the source has
already drifted: a full `golangci-lint` run (with its per-message display limits
lifted) reports 72 findings, including dead code, ineffectual assignments and
silently discarded errors. Without a gate this debt grows with every change.

## What Changes

- Add a committed `golangci-lint` v2 configuration with the standard linter set
  (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`) and **no**
  presets or per-path exclusions. Every ignored error is decided in the code.
- Make the current source pass it: check or explicitly discard each error,
  remove dead code, fix ineffectual assignments and the remaining staticcheck
  findings.
- Add a CI job that runs `gofmt` and `golangci-lint` on every push and pull
  request. The existing test job is unchanged.
- Pin the linter version in CI and document the same version locally. The linter
  stays a development tool: no module dependency, no `go.sum`.
- Document formatting and linting commands in the README.

The product behaviour does not change; the source is cleaned up and the quality
gate becomes enforceable.

## Capabilities

### New Capabilities

- `code-quality`: formatting and static analysis of the Go source, the
  suppression policy, and the continuous-integration gate that enforces them.

### Modified Capabilities

None.

## Impact

- **Code**: 22 Go files (7 of them test files). 18 CLI writes become explicit
  discards, 31 resource closes/removes are checked or explicitly discarded,
  9 unused declarations are removed, 2 ineffectual assignments are fixed, and
  12 staticcheck findings are fixed, including three deprecated
  `runtime.GOROOT()` calls in tests.
- **Configuration**: new `.golangci.yml`.
- **CI**: new `lint` job in `.github/workflows/ci.yml`; the `test` job is
  untouched. Marking the job as a required check is a repository setting, not a
  file change.
- **Docs**: README gains a formatting/linting section.
- **Dependencies**: none added; `golangci-lint` is pinned in CI and installed
  locally outside the module.
