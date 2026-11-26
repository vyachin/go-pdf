# Tasks

## 1. Regular-file gate

- [ ] 1.1 In `internal/fsx`, stat the resolved path through the root and reject anything whose mode is not regular before opening it. Verify with unit tests for a directory, a FIFO, a character device where the platform has one, and a regular file.
- [ ] 1.2 After the open, call `f.Stat()` and repeat the regular check so the decision describes the descriptor that was opened. Verify with a test that swaps a regular file for a FIFO between the stat and the open, or documents the residual window if the swap cannot be made deterministic.
- [ ] 1.3 Route the explicit `FallbackFont` reader through the same gate. Verify that naming a FIFO as `FallbackFont` returns `font/unavailable` instead of blocking.

## 2. Non-blocking open

- [ ] 2.1 Add a build-tagged open helper that uses `os.Root.OpenFile` with `syscall.O_NONBLOCK` on Unix and a plain open on Windows. Verify the package builds with `GOOS=windows go build ./...` and `GOOS=linux go build ./...`.
- [ ] 2.2 Use the helper for every asset read inside and outside a root. Verify with a FIFO image, stylesheet and font through the public `Convert` that each returns the documented diagnostic and the test completes under the package timeout.
- [ ] 2.3 Keep symlink confinement unchanged: a symlink to a regular file inside the root still reads, a symlink to a FIFO is rejected, and a symlink to a file outside the root is still a path escape. Verify all three in one table test.

## 3. Error mapping and record

- [ ] 3.1 Map the non-regular condition to `img/load`, `css/link-load` and `font/unavailable` at the existing adapter boundaries and keep the typed cause. Verify each code in the end-to-end test from task 2.2.
- [ ] 3.2 Add a `Fixed` entry to `CHANGELOG.md` stating that a FIFO or other non-regular asset reference is rejected instead of blocking the conversion. Verify the entry names no removed API and no new diagnostic code.

## Workflow follow-up

- Run `openspec validate fix-non-regular-asset-open --strict` and fix any reported issue.
- Sync the delta into `openspec/specs/library-api/spec.md` and archive the change after review.
