# Design

## Context

See `proposal.md` and the fourth review pass in `TODO.md`. The 66 findings fall into
two kinds. Some are gaps: behaviour that no requirement states (the GPOS pair table,
the PNG bit depth, the diagnostic catalogue). Others are violations of an existing
requirement that no scenario exercises (a declared windows-1252, a `<style>` rule
next to an invalid option, adjacent-sibling matching). The twelve improvement
proposals are a third kind: work that is not a defect but a change of mechanism or
process, listed under "Improvements for separate follow-up" in `TODO.md`. Every
finding and every proposal already has a reproduction or a measured cost in
`TODO.md`; the missing pieces are a normative statement, a fixture and a fix.

The engine is layered `html → css → profile → layout → pdf` around one
`budget.Guard`, one `diag.List` and integer millipoint geometry. Each layer owns the
diagnostics it can decide, and both public entry points share `prepare` and
`layoutDocument`. The fix keeps that shape: no new package, no new dependency, no
second acceptance path.

## Goals / Non-Goals

**Goals:**

- One normative requirement per finding and per improvement proposal, named after
  the behaviour and tagged with its `RV4-` or `IMP-` id, so a reviewer can match a
  fix or a rule to a spec and a test.
- Bound every stage whose cost grows with the input, and prove the bound with a
  fixture whose `MaxOperations` stops it.
- Make the two entry points and their documentation agree.
- Make the documents true.

**Non-Goals:**

- Expanding the CSS profile (flex, grid, floats, named pages, hyphenation).
- Replacing the HTML front end, the CSS grammar or the layout algorithm.
- A new public API surface beyond the documented fixes.
- Rewriting the test oracle or adding a new external dependency.

## Decisions

### 1. One requirement per finding, additive

Each `RV4-` item becomes its own requirement in the capability that owns the
behaviour, even where a general requirement already covers the class. A general
requirement ("the work budget covers every stage", "selector matching is bounded")
states the invariant but hides which stage regressed; a specific requirement and a
named scenario keep the regression individually testable and traceable. When the
change is archived the specific requirements sit beside the general ones; a later
cleanup may fold a specific requirement back into its general form once the
scenario exists there.

### 2. Bound by construction, then charge

A path that allocates per parsed unit gets a cap and a charge before the
allocation: the GPOS pair table gets a declared-pair cap, the CSS parser a stored-
problem cap, the diagnostics list a per-sheet cap. A path whose cost is O(bytes) but
is currently O(1) in units gets a proportional charge: shaping per rune, the writer
per serialised byte, the measurement per built line. A path whose cost is O(n²) gets
an algorithmic fix rather than a charge: a child index for adjacent siblings, a
width-keyed line cache for `measure`. The rule is: a cap where the format allows a
cheap bound, a charge where the cost is legitimate, an algorithm where the cost is
an accident.

### 3. `Validate` is defined by what it can decide

`Validate` runs preparation and layout and stops before serialisation. Instead of
duplicating the writer in `Validate`, the metadata checks move into `prepare` so
both entry points share them, and the documentation names serialisation (including
its work budget) as the only uncovered stage. A test asserts that the two
diagnostic lists are equal for every document that fails before serialisation.

### 4. Positions come from the token, not from the tree walk

A node's source span is recorded from its own token during construction. Where the
parser cannot supply that (an implied node), the node inherits its ancestor's
range, and a sliced `<style>` or `<script>` span is verified against the node's
text before it is used. This is the only way foster parenting can preserve both the
content and the position, because tree order and source order differ by design.

### 5. An asset reference is a URL path

Image, stylesheet and font references are resolved as URL paths: the query and the
fragment are removed, the path is percent-decoded, ASCII whitespace is trimmed, and
a scheme-relative `//host` is remote. The image format comes from magic bytes, not
from the extension, and the diagnostic names the detected format. A JPEG is
checked for an EOI before it is embedded; a failed load is refunded from the
aggregate totals.

### 6. Documents derive their facts

The limits table is generated from `engine.DefaultLimits()`, the diagnostic
catalogue from the code, and the image pin from the workflow and the renderer
record. A test fails when a document, the code and the workflow disagree. The
`RV4-` requirements in this change state the behaviour; the documentation tasks
make the prose match it.

### 7. Release the removal as v2.0.0

Removing an exported field is a breaking change. The Unreleased section becomes
v2.0.0 with a migration section; a v1 branch is not maintained. A CI check refuses
to tag a changelog whose Unreleased section is marked breaking as a minor.

### 8. Fix the instance, then prevent the class

An `RV4-` requirement states the behaviour a caller observes: the conversion fails,
the document has no empty page, the catalogue lists the code. An `IMP-`
requirement states the mechanism that keeps the class from returning: one code
registry, one pipeline, one resolver, one media-list parser, a generated table, a
release gate, an invariant test, a benchmark. The two are deliberately separate, even
when they touch the same code, because a reviewer of a fix should be able to accept
the behaviour without also accepting a refactor, and a reviewer of the refactor
should be able to judge it on its own. A fix that passes its `RV4-` scenario but
leaves the `IMP-` rule unimplemented is a legitimate intermediate state; the change
is not complete until both are done.

## Risks / Trade-offs

- **Output bytes change.** Capping `/BaseFont`, preserving 16-bit PNGs and packing
  `TJ` strings change the committed snapshots and the render goldens. Each is one
  reviewed update, as `docs/golden.md` requires; the deterministic guarantee is
  about repeated runs, not about crossing a minor version.
- **More charges slow a within-budget document.** The per-rune and per-byte charges
  add a few counter updates; the benchmarks in `CONTRIBUTING.md` are refreshed with
  the change and remain well inside the defaults.
- **Stricter validation rejects documents that converted before.** A declared
  windows-1252 with UTF-8 bytes, `height: 50%`, a CSS-wide keyword in
  `font-family` and a pseudo-element in a non-final compound become errors. Each is
  documented in the migration section; the alternative is silent wrong output.
- **A relative path in `fsx` changes meaning.** No in-tree caller passes a relative
  path today, so the change is safe; a test pins the new meaning.
- **A worker that relied on system fonts still is not deterministic across
  machines.** The change documents the condition rather than removing the fallback;
  the guarantee is "same document, options and fonts".
- **The standing rules cost CI time.** A coverage floor, a validator on every job
  that writes a PDF and an invariant test per stage add minutes; the benefit is that
  the next instance of a class already fixed cannot land silently. The floors are
  set from the current numbers, not from an aspiration.

## Migration

- `Options.ImportBrowser` is removed in v2.0.0; callers delete the field and rewrite
  the template within the profile.
- `CharsetSupported("")` returns true; callers that used it as "a charset was
  named" check `Options.Charset != ""`.
- A document that declared windows-1252, used a percentage `height`, a CSS-wide
  keyword in `font-family`, or a pseudo-element outside the last compound now
  fails; the diagnostics name the declaration.
- A 16-bit PNG keeps its depth, so its PDF is larger than before.
