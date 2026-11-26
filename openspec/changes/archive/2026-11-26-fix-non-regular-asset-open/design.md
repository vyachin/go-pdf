# Design

## Context

See `proposal.md` — Why. `fsx.Root` exposes `Read`, `ReadLimit`, `Size` and
`Resolve`; all of them reach the file through `os.Root`, which follows symlinks and
confines paths but has no regular-file policy. `assets.Registry` stats a file to
reserve its size and then calls `ReadLimit`, which opens the file with
`os.Root.Open`. A FIFO therefore blocks inside the open before any byte is read,
and the context is only observed by the guard after the read returns. The explicit
`FallbackFont` path uses the same byte reader outside a root, so it shares the
failure mode.

## Goals / Non-Goals

**Goals:**

- A reference to a non-regular file ends in the resource's load diagnostic.
- No asset open waits for a writer, even if the file type changes between the check
  and the open.
- The regular-file gate is shared by every reader, including the explicit
  caller-selected font, so no path keeps the hang.

**Non-Goals:**

- A general filesystem sandbox: regular files anywhere the root allows remain
  readable, and a caller that names `/dev/...` as `FallbackFont` still reads it if
  it is a character device that behaves like a file. The change rejects
  non-regular files, it does not audit what a regular file contains.
- Rejecting hard links, sparse files or files that grow during the read; those are
  regular and are bounded by the existing per-file limits.
- Locking an asset against concurrent replacement by another process.

## Decisions

**Stat before the open, and fstat after it.** The pre-open stat is what prevents the
block in the common case; the post-open `f.Stat()` is what makes the decision about
the file that was actually opened. Rejected alternative: open first and check
afterwards — it is exactly the FIFO case that blocks, so the check would never run.

**Open with `O_NONBLOCK` where the platform defines it.** `os.Root.OpenFile` makes
the flag usable without leaving the root. A FIFO opens immediately with no writer
and is then rejected by the post-open check; for a regular file the flag has no
effect on Unix reads. Rejected alternative: run the open in a goroutine and time it
out — the goroutine stays blocked inside `open(2)` with a descriptor and a stack,
so a hostile document could accumulate stuck goroutines; rejected alternative:
pre-stat only — a file replaced between stat and open reproduces the hang.

**The gate lives in `fsx`, not in the adapters.** Images, stylesheets, fonts and
the explicit fallback all call the same byte reader, so one check covers them and
the diagnostics stay the caller's mapping. Rejected alternative: check in
`internal/assets.Read` only — the explicit `FallbackFont` path bypasses the
registry and would keep the hang.

**Non-regular maps to the resource's existing load diagnostic.** No new code: the
caller already distinguishes `img/load`, `css/link-load` and `font/unavailable`, and
a caller matching on them does not need to learn a fourth name. Rejected
alternative: `asset/not-regular` — it expands the stable catalogue for a condition
that is a load failure.

**Symlinks are resolved first, then the target is classified.** A symlink to a
regular file inside the root keeps working; a symlink to a FIFO is rejected like the
FIFO. Rejected alternative: reject all symlinks — it would break the documented
"a symlink inside the root is followed" rule and the existing tests.

## Risks / Trade-offs

- **TOCTOU remains between the stat and the open** → `O_NONBLOCK` keeps the open
  from blocking and the post-open `fstat` rejects the replaced file; the residual
  window can only turn a regular file into another regular file.
- **Platform differences** → the flag helper is build-tagged (`unix` sets
  `O_NONBLOCK`, Windows returns 0), and the FIFO tests are skipped where the
  platform cannot create one. The regular-file check itself is portable.
- **`/proc` and device files that report as regular** → the read may still return
  `EAGAIN`; the reader maps it to the load error, which is the same diagnostic.
- **A caller that intentionally reads a character device as a font** → it now
  receives `font/unavailable` instead of a stream; the change documents that assets
  are files, and the reference command was never able to install a device as a
  font in practice.

## Migration Plan

Not applicable: no public API, option, output format or diagnostic code changes.
Existing documents that reference regular files are unaffected. Release note under
`Fixed`; the FIFO regression test is the durable record.

## Open Questions

None.
