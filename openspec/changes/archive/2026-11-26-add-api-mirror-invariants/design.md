# Design

## Context

`pdfgen` is the only public package; `Options`, `Limits` and `Diagnostic` are
declared there so their godoc is the consumer documentation, while the engine
and `internal/diag` keep their own types. Three hand-written functions bridge
them: `toEngine` on `Options`, `toEngine` on `Limits`, `fromEngineLimits` and
`fromDiags`. The bridge is field-by-field, so an added field compiles on either
side and is silently dropped by the other. `TestDefaultLimitsMatchesConversion`
lists the fields explicitly, which catches a forgotten default only for the
fields someone remembered to list. The public package already has a source-level
docs test (`TestExportedDocs`), and the engine has `TestDocumentedLimitsMatchTheCode`.

## Goals / Non-Goals

**Goals:**

- A failing test when `Options`, `Limits` or `Diagnostic` gains or loses a field
  on one side only.
- A failing test when a field is not carried across the boundary in either
  direction.
- Reflective default equality instead of a hand-written chain.
- Godoc that states the semantics of the internal field it mirrors.

**Non-Goals:**

- Replacing the mirror with a type alias or generated code.
- Changing any field name, default, type or public behaviour.
- Rewriting the engine's own type documentation beyond the two stale fields.

## Decisions

**Reflection over a longer hand-written chain.** A test compares
`[]struct{Name, Type}` from both types and the counts; another sets every public
field to a unique value and compares the internal result field by field. The
manual chain is what failed to catch the drift, so extending it would keep the
same gap. Alternative rejected: keep the chain and add a comment asking for
updates — comments do not fail.

**No type alias and no code generation.** `type Options = engine.Options` would
put an internal package path into the public godoc and make every engine field
rename a public breaking change; a generator needs a build step the project does
not have and would still need review of the generated diff. Alternative
rejected: generate the mirror from `engine` — it couples release tooling to an
internal layout and obscures the public documentation.

**Unique markers by field kind for the round trip.** The test derives each value
from the field index and kind (`int` → `i+1`, `string` → `v<i>`, `bool` → true,
`time.Time` → a fixed instant, nested `Limits` → a distinctive value), so a
missing mapping produces a mismatch that names the field. `Diagnostic` is checked
in both directions through `fromDiags`; `Options` has no reverse conversion by
design, so it is checked public-to-engine only. Alternative rejected: set only
one or two fields per type — the dropped field is usually the new one.

**Defaults through reflection.** The existing manual comparison becomes a loop
over the reflected fields of `engine.Limits` and `pdfgen.Limits`, so a new limit
is compared automatically. The existing conversion-backed assertion (an
oversized input is rejected) stays, because it proves the default is applied
rather than merely copied.

**Godoc checked at the source level.** The two known drifts are corrected in
`internal/engine/engine.go`: `MaxTableCells` becomes "the grid area of one
table: its column count times its row count", matching the public field and the
code; `RenderPNG` becomes a debug preview of the first page and drops the CLI
claim. A test reads the file and requires the phrases. The full preview contract
and its deprecation belong to the separate RenderPNG change; this change only
makes the mirrored description truthful.

## Risks / Trade-offs

- **Reflection is brittle to intentional renames** → that is the point: a
  rename must update both sides and the test names the field.
- **Marker values collide with defaults** → values start at 1 and strings are
  prefixed, so a zero value means "not mapped".
- **The source-level godoc test is textual** → it requires a short phrase, not a
  full sentence match, so rewording is possible while the claim cannot silently
  return.
- **Two changes touch the RenderPNG sentence** → this change removes the CLI
  claim; the preview change adds the contract and `Deprecated:` paragraph. Either
  order is valid, and the final text satisfies both.

## Migration Plan

None: tests and comments only. No exported identifier, default, diagnostic or
output changes, so the release note can omit the change; the archive records that
the mirror is now checked.
