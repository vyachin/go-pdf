# Proposal

## Why

Four defects in asset admission make the same conversion report different results
depending on race, file type and option order. Eight concurrent reads of one 1 MiB
file against a 1.5 MiB total make seven fail, because each reader reserves the
bytes before the cache is populated. The aggregate total is reserved from the
`stat` size, so a file that reports zero and yields 1 MiB returns its bytes while
committing nothing. An open error is classified by looking for the substring
"escapes" in its message, so a mode-000 file named `escapes.css` is reported as a
path escape instead of a load failure. And the metadata overrides are validated
after linked assets, so an invalid `Title` plus a missing stylesheet reports only
the stylesheet and hides the option error.

## What Changes

- Concurrent reads of one asset identity are single-flight: one read, one charge,
  every caller receives an equivalent record or the same typed failure.
- The aggregate total is charged for the bytes actually admitted; the pre-read
  `stat` check stays as a fast rejection, and a size that does not match the bytes
  read is reconciled or refused. A refund returns exactly what was charged.
- Open failures are classified from the containment check, not from message text: a
  permission failure inside a root is a load failure, and only a reference that
  leaves the root is a path escape.
- The metadata that produces `options/title`, `options/author`,
  `options/subject` and `options/keywords` is validated with the other options,
  before any linked asset or font is opened; an invalid option still validates
  inline styles and then stops, so the option error is never hidden.
- Regression tests cover the eight-goroutine reservation, a size that lies, a
  mode-000 file whose name contains "escapes", and a bad title combined with a
  missing stylesheet.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-api`: the asset-policy requirement gains single-flight identity,
  actual-byte accounting, containment-based error classification and an option
  validation order that covers document metadata.

## Impact

- `internal/assets` (single-flight, accounting, refunds), `internal/fsx`
  (classification), `internal/engine` (metadata order), `internal/profile`
  (stylesheet error mapping).
- No public API or diagnostic-code change. For a permission failure the reported
  code changes from `css/link-path` to `css/link-load`; both codes stay in the
  stable catalogue, and the new code names the actual cause.
- Release note under `Fixed`.
