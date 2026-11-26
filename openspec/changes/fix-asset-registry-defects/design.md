# Design

## Context

See `proposal.md` — Why. `assets.Registry` keeps one cache per conversion keyed by
kind, canonical identity and decoding inputs. A read performs five steps: resolve
and contain the reference, check the cache, `stat` the file, reserve its size
against the kind total, then read and insert. The mutex protects the map and the
totals, but not the sequence: two readers of one key both miss, both reserve, and
the second insert merely replaces the first, so the bytes are charged twice. The
reserved amount comes from `stat`, while `Record` keeps only the bytes, so a refund
uses a different number than the charge. `fsx.classifyOpenError` decides an escape
by searching the error text for "escapes", which a file name can contain. In
`engine.prepareDocument`, stylesheets and fonts are loaded before
`validateMetadata`, so an asset failure returns before the metadata diagnostic is
produced.

## Goals / Non-Goals

**Goals:**

- One identity is read, admitted and charged once per conversion, whatever the
  number of callers and their timing.
- Accounting describes the bytes that were admitted, and every refund reverses
  exactly one charge.
- An open failure names its real cause, independent of the file name.
- An invalid option is reported on the first pass, whether or not an asset is
  broken.

**Non-Goals:**

- Making the engine convert assets in parallel. No public path does that today;
  the registry contract becomes correct for the adapters that may.
- Changing the cache key, the identity rules or the per-kind totals.
- Adding a diagnostic code or a public option.
- Caching failures: a failed read stays retryable on the next call, as today.

## Decisions

**Single-flight per key with a published record, not a held lock.** The first
caller creates a `readCall` (done channel, record, error) under the mutex; later
callers find it and wait outside the lock; the leader reads, inserts the record and
closes the channel. Rejected alternative: hold the registry mutex for the whole
read — it serializes unrelated assets and the font/image parsing that happens while
the lock would be held; rejected alternative: document the registry as
single-goroutine — it leaves the double reserve latent and contradicts the
"adapters may call from several goroutines" contract.

**Reserve from `stat`, then reconcile with the bytes read.** The pre-read check is
kept because it rejects an oversized file before it is opened. After the read, the
charge becomes `len(bytes)`: a shortfall is refunded immediately, a surplus is
checked against the remaining total and either charged or refused (with the
reservation refunded and the bytes discarded). `Record` stores the charged amount
so `Discard` reverses exactly that number. Rejected alternative: charge only after
the read — it reads a file that cannot fit the total, bounded only by the per-file
limit; rejected alternative: keep trusting `stat` — it is the defect.

**Classify from containment, not from the message.** On an open failure the
resolver reruns the containment check; only a result of `ErrOutsideRoot` becomes a
path escape, and every other cause is preserved. Rejected alternative: match the
unexported error type `os.Root` returns for an escape — it is not part of the
public API and changed across releases; rejected alternative: keep the substring
test — any file name can defeat it.

**Validate metadata in the option gate.** `validateMetadata` needs only the parsed
document, which exists before assets, so it moves next to the `PageSize`, `Margin`,
`Date` and `Charset` checks. If it fails, the existing invalid-option branch runs:
inline styles are validated for a complete report, and linked assets and fonts are
not opened. Rejected alternative: keep the order and append metadata errors later —
the early return after an asset failure has already skipped them; rejected
alternative: validate metadata twice — two sources of the same diagnostic.

**Failures are delivered, not cached.** Waiters of an in-flight call share its
error, but the in-flight entry is removed when the call ends, so a later caller
retries instead of inheriting a transient failure. Rejected alternative: store the
error in the record cache — a cancelled or transient read would poison the
conversion.

## Behavioral verification

Count actual underlying reads and calls to the font parser independently from
successful cache entries and byte totals. Existing sharing tests count reads but
do not establish the already documented one-parse guarantee. Instance-local test
instrumentation must detect duplicate parsing even when `keepFont` returns the
first stored pointer. Concurrent registry reads are synchronized with channels;
font parsing is tested through supported adapter paths without adding parallel
conversion work. Separate conversions each perform their own parse. Failed
attempts are counted separately and remain retryable, so an exactly-once assertion
never turns a transient failure into a cached success or error. No instrumentation
may depend on mutable global state or a production environment variable.

## Risks / Trade-offs

- **A waiter cannot cancel independently of the leader** → all callers of one
  conversion share its context, so cancellation still stops the read; a caller that
  wants its own deadline runs its own conversion.
- **A file that grows between `stat` and the read is read before the surplus is
  refused** → the per-file `MaxBytes` still bounds the allocation, and the total is
  corrected or the asset rejected; this is the documented trade-off of keeping the
  early rejection.
- **Classification re-checks containment after a failure** → a file replaced
  between the open and the check can change which of the two errors is reported;
  both are accurate for a state the file was in during the call.
- **An invalid title now suppresses a linked-asset diagnostic on the first pass** →
  the option error is the one the caller must fix first, and a second call reports
  the asset; the release note names the order.
- **Single-flight shares one `Record` value** → callers receive a copy, as today,
  so a customer mutating its copy cannot corrupt the cache.

## Migration Plan

Not applicable: no public API, option or output-format change. Diagnostic content
changes in two narrow cases (permission failure code; option error before asset
error) and the release note lists them under `Fixed`. Rollback is a revert of the
change; no stored state or configuration is involved.

## Open Questions

None.
