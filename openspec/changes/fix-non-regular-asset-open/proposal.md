# Proposal

## Why

The asset reader resolves a reference and calls `os.Open` on the confined path
without checking what kind of file it is. Opening a FIFO blocks in `open(2)` until
a writer appears; a probe that references one as an image did not return for more
than three seconds, and a cancelled context cannot interrupt it because the block
is inside the open call, before any guard or context check. A directory, device or
socket reference is likewise opened and read until the size limit fires. A hostile
or accidental reference can therefore wedge a conversion that the package promises
is bounded and cancellable.

## What Changes

- Only a regular file is read. A directory, device, FIFO, socket or other
  non-regular file fails with the resource's load diagnostic (`img/load`,
  `css/link-load`, `font/unavailable`) instead of being opened for reading.
- Opening an asset no longer waits for a writer: the regular-file check runs before
  the open, and the open itself is non-blocking on platforms that support it, so a
  file swapped between the check and the open cannot wedge the conversion.
- The check does not weaken root confinement or symlink resolution. A symlink to a
  regular file inside the root still reads; a symlink to a FIFO or to a file
  outside the root is rejected.
- Regression tests use a FIFO through the public entry points, with the test
  timeout as the failure mode; platforms without FIFOs skip that case.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: asset reading gains a regular-file precondition and a
  non-blocking-open requirement on top of the existing no-write and confinement
  rules.

## Impact

- `internal/fsx` (open path and regular-file gate), `internal/assets` (request
  admission and error mapping), `internal/engine` and `internal/profile` (the
  diagnostic each kind reports).
- No public API or option change. Release note under `Fixed`.
- Tests gain a FIFO fixture helper; Windows builds skip the FIFO cases because the
  platform does not create them the POSIX way.
