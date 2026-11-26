# Design

## Context

See `proposal.md` — Why. Every asset request today carries a filesystem root:
`assets.Request` has `Dir` (an operating-system directory) and `Explicit` (a
caller-selected path read outside the roots), and `fsx.Root` enforces containment
with `os.Root`. `internal/profile` joins a stylesheet href with `BaseDir` using
`filepath`, and the engine reads `FallbackFont` through the explicit path. An
`fs.FS` has different rules: slash-separated names, no host paths, no symlinks, and
an `Open` that the caller may implement however it likes.

## Goals / Non-Goals

**Goals:**

- An additive option that makes the library usable with `embed.FS`, `fstest.MapFS`
  and `os.DirFS` without extracting files.
- One reader for every asset kind, with the same limits, diagnostics and
  determinism as the filesystem path.
- No change for a caller that leaves the option nil.

**Non-Goals:**

- A network or database source; a caller that wants one implements `fs.FS`.
- Mixing a source for document assets with host-filesystem resolution for
  `FallbackFont` inside one conversion.
- Writing to the source, closing it, or taking ownership of its lifetime.
- Changing the filesystem default root (that is `RV5-07`).

## Decisions

**Use the standard `fs.FS`, not a bespoke provider interface.** `embed.FS`,
`os.DirFS`, `fstest.MapFS` and `fs.Sub` already implement it, and the caller can
compose a read-only view with `fs.Sub` before passing it. Rejected alternative: a
custom `AssetSource` with `Open`/`Stat`/`ReadFile` — more public surface, no
standard adapters, and the same functionality through a type every Go developer
already has.

**The source is authoritative for the whole conversion.** When it is set, document
references and `FallbackFont` both resolve through it. Rejected alternative: keep
`FallbackFont` on the host filesystem — an embedded deployment would still need a
font on disk, defeating the point; rejected alternative: try the source first and
fall back to the host — two authorities for one reference, and the result would
depend on what happens to exist on the machine.

**`BaseDir` and `FontDir` are source paths; their default is the source root.**
`Options.File` stays a diagnostic label and is not interpreted as a source path.
Rejected alternative: treat `File` as the document path in the source — callers
pass host paths or labels such as `<stdin>`, which are not valid `fs` names.

**Validate source paths before opening.** The URL path is cleaned with `path.Clean`
and must satisfy `fs.ValidPath`; an absolute name or a name that still contains
`..` is rejected. Rejected alternative: `filepath.Join` and `filepath.Rel` — they
introduce host separators and volume rules into a namespace that is defined with
slashes; rejected alternative: `fs.Sub` per root — an extra wrapper whose errors
would need a second mapping to the existing diagnostics.

**Read through `fs.Open` with a limit, not `fs.ReadFile`.** `fs.ReadFile` can read
an arbitrarily large object before the size check, while `fs.Open` plus
`io.LimitReader` keeps the per-file `MaxBytes` bound and reuses the filesystem
reader's accounting. Rejected alternative: `fs.ReadFile` then check `len` — the
allocation has already happened.

**Identity is the source path, not a digest.** Within one conversion the cleaned
source path plus the kind and the decoding inputs identifies the record, exactly as
the canonical path does for the filesystem. Rejected alternative: hash every source
asset for identity — an extra pass over every byte for a namespace the conversion
already owns.

**Diagnostics map like a filesystem read.** A missing object, a rejected path and a
read error keep `img/load`, `css/link-load` and `font/unavailable`; no new code is
introduced. Rejected alternative: a `source/*` code family — it doubles the
catalogue for the same failures.

## Risks / Trade-offs

- **A source may return more bytes than `MaxBytes` or read slowly** → the reader
  wraps `Open` with `io.LimitReader`, and the work guard charges the admitted
  bytes; a pathological implementation is the caller's own code, not document input.
- **`fs.FS` has no symlinks** → a reference cannot escape through one, and the
  containment rule reduces to the `fs.ValidPath` check; the difference from the
  filesystem source is documented.
- **Backslash in a source name** → it is a file-name character in `fs.FS`, while a
  filesystem treats it as a separator on Windows; URL references use slashes, and
  the documentation states the namespace rule.
- **`FallbackFont` cannot name a system font when `Source` is set** → the caller
  ships the font in the source or uses `@font-face`; documented in the option's
  godoc and `docs/fonts.md`.
- **The source may expose data the caller did not intend** → it is caller-supplied
  and read-only by type; the library never writes to it.

## Migration Plan

Not applicable: additive field, nil by default. A caller adopts it by setting
`Source` and, when its assets live in a subdirectory, `BaseDir`/`FontDir`; no
existing conversion changes. Release note under `Added`; the option is documented in
the package godoc and README.

## Open Questions

None.
