# Design

## Context

See proposal.md. Both helpers currently do:

```go
bin := os.Getenv("PDFCPU")
if bin == "" { bin = "/tmp/gobin/pdfcpu" }
if _, err := os.Stat(bin); err != nil { skip }
```

## Goals / Non-Goals

**Goals:**

- One lookup rule for both packages.
- A build that cannot pass while the validator is missing in CI.
- A local run that still works and says the check was skipped.

**Non-Goals:**

- An independent renderer or pixel comparison.
- Installing tools locally from the test.

## Decisions

### 1. Lookup order: `PDFCPU`, `PATH`, local fallback

`PDFCPU` stays the explicit override; `exec.LookPath("pdfcpu")` covers CI where the binary lands in `GOPATH/bin`; `/tmp/gobin/pdfcpu` remains for developer machines that already use it. A helper per test package returns the path or an empty string.

*Why duplicate the helper*: the two packages are separate test binaries; a shared `internal/testpdf` package would add a non-test package for test-only code.

### 2. Strict mode through the environment

`PDF_REQUIRE_VALIDATOR=1` turns "not found" into `t.Fatal`. CI sets it; local runs do not, and the skip is logged in both packages so it is visible rather than silent.

*Why an environment variable and not a build tag*: CI can set one variable for the whole `go test ./...` run without per-package flags.

### 3. CI workflow

`.github/workflows/ci.yml` on push and pull requests:

1. checkout and `actions/setup-go` with Go 1.24;
2. `go install github.com/pdfcpu/pdfcpu/cmd/pdfcpu@latest`;
3. add `$(go env GOPATH)/bin` to `PATH` via `GITHUB_PATH`;
4. `go vet ./...`;
5. `go test ./...` with `PDF_REQUIRE_VALIDATOR=1`.

*Why not pin a pdfcpu version*: `@latest` keeps the validator current; a pinned version is a follow-up if reproducibility matters more than freshness.

## Risks / Trade-offs

- **`@latest` can break the build** when pdfcpu changes its CLI. → The failure is visible and the version can be pinned in one line.
- **Network access in CI** is needed to install the tool; acceptable for a build.
- **Local skip remains possible** by design; the log line is the signal, and CI is the enforcement.
