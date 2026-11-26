# Design

## Context

See `proposal.md` — Why. The asset layer is layered: `internal/asset` turns a URL
reference into a path, `internal/fsx.Root` enforces the boundary, `internal/assets`
admits and identities the bytes, and `internal/engine` and `internal/profile` ask
for assets per kind. `fsx.Open("")` deliberately returns a Root without a boundary,
so today "no `BaseDir`" means "no confinement", not "the working directory". Fonts
have a second exception: an absolute `@font-face` source is read as written even
when `FontDir` is set. `FallbackFont` is caller-selected and is read as an
operating-system path, which stays correct because the document cannot choose it.

## Goals / Non-Goals

**Goals:**

- One confinement rule for images, stylesheets and document-referenced fonts.
- A safe default that still lets a zero `Options` convert a document whose assets
  sit next to it.
- An explicit, per-conversion opt-in for callers that must keep absolute paths.
- Documentation that states the enforced rule instead of implying it.

**Non-Goals:**

- Sandboxing against the process owner. A caller that sets `AllowAbsoluteAssets`,
  or that points a root at `/`, keeps the access it asks for; this is a default
  policy, not a security boundary against the caller.
- Reading assets from an `fs.FS`; that is a separate change (`RV5-48`).
- Changing how `FallbackFont` is resolved: it is explicit caller input, not a
  document reference, and keeps its current meaning.

## Decisions

**Root precedence: kind root, then `BaseDir`, then the document directory, then the
working directory.** The order matches the existing `FontDir` fallback and the
reference command, which already resolves assets against the input file's
directory. When `Options.File` is set, `filepath.Dir(File)` is the root; the string
is still used verbatim in diagnostics. Rejected alternative: require `BaseDir`
always — it breaks the zero-`Options` contract; rejected alternative: always use
the working directory — it ignores the document's location and makes the CLI and
the library disagree.

**Absolute references: inside a configured root, or behind the opt-in.** With no
root there is nothing to test containment against, so the reference is rejected.
Rejected alternative: reject absolute references unconditionally — legitimate
callers that assemble a template and its assets in a temporary directory would have
no way to express it; rejected alternative: keep the per-kind exception for fonts —
the same reference would be safe as a font and unsafe as an image, which is the
inconsistency the change exists to remove.

**The opt-in is `Options.AllowAbsoluteAssets`.** "Assets" scopes it to document
references and keeps `FallbackFont` outside it. Rejected alternative:
`AllowUnconfinedAssets` — it suggests the root check itself can be disabled, while
the field only admits an absolute reference when no root exists; rejected
alternative: no opt-in at all — a rejection would silently break existing
deployments with no migration path other than editing templates.

**Enforce in `fsx`/`assets`, not in each caller.** Every adapter already routes
through the registry, so one boundary implementation covers images, stylesheets
and fonts. Rejected alternative: check the reference in `internal/profile` and
`internal/engine/images.go` — two implementations that already drifted once.

**Reuse the existing diagnostics.** An outside-root reference produces `img/load`,
`css/link-path` or `font/unavailable`, exactly as a relative escape does today.
Rejected alternative: a new `asset/outside-root` code — it expands the stable
catalogue and forces callers to update matching for a condition that already has a
code.

## Risks / Trade-offs

- **Breaking default for absolute paths** → the release is a major one and the
  changelog names both behaviours; the migration is one option, and a caller can
  keep its old behaviour with `AllowAbsoluteAssets: true`.
- **`Options.File` is a label for some callers** (`"<stdin>"`, `"invoice.html"`) →
  `filepath.Dir` of a bare name is `.`, so the root stays the working directory;
  the package documentation states that `File` is interpreted as a path only for
  choosing the default root.
- **A symlink can point outside the root** → containment is decided after
  `EvalSymlinks`, and the existing symlink tests are extended with a link that
  leaves the root.
- **The opt-in weakens confinement** → it is per conversion, defaults to false, and
  the documentation says it exists for caller-built temporary layouts.
- **Windows drive letters** → the boundary uses `filepath.IsAbs` and
  `filepath.Rel`, not string prefixes; a test runs the resolution table on the host
  filesystem.

## Migration Plan

1. Land the option and the new default in one change; the reference command keeps
   working because it already sets `BaseDir` and `FontDir` from the input path.
2. A caller migration: set `BaseDir`/`FontDir` to the directory that holds the
   assets, or set `AllowAbsoluteAssets: true` to keep the old reading.
3. Release as the next major version with a `Changed` entry that names the two
   breaking cases and the one-line migration.
4. Rollback is configuration-only: the previous behaviour is available without a
   code change once the option exists.

## Open Questions

None.
