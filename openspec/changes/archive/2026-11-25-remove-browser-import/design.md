# Design

## Context

See `proposal.md` for motivation and `specs/browser-import/spec.md` for the
capability removal. The engine currently validates collected stylesheets, then
conditionally mutates the document and stylesheets through `browser.Import` and
validates them again. Both `Convert` and `Validate` share this preparation path.
The CLI and WeasyPrint harness opt in on every call.

`internal/browser/import.go` contains approximately 950 lines. Its raw cascade
accessor, `Cascade.Declarations`, has no consumers outside that package. The
browser certificate is an engine and CLI acceptance fixture; a separate
profile-compatible certificate is already covered by rendering goldens.

## Goals / Non-Goals

**Goals:** have one profile acceptance path, remove importer-specific code and
options, and make the changed input and diagnostic behavior visible in tests and
migration documentation.

**Non-Goals:** expand supported CSS, move lowering into another package, change
the HTML parser's standard tree construction, or fix unrelated option-validation
and diagnostic issues. Native image loading, fonts, background painting and
supported CSS semantics are outside this removal.

## Decisions

### 1. Delete the compatibility branch and both option fields together

`prepare` will collect stylesheets, validate the parsed document once against the
profile and continue only if there are no errors. Remove `browser.Import`, its
page-geometry parsing used only for lowering, the second validation and diagnostic
replacement. Keep stylesheet-collection diagnostics, shared resource checks,
sorting and the ordinary page-style/font preparation.

Remove `ImportBrowser` from `pdfgen.Options`, `engine.Options`, option forwarding,
the CLI and corpus harness. `Strict` retains its documented warning-to-error
behavior. No automatic lowering or ignored compatibility field replaces the
removed option: either would retain an additional policy or suggest support that
no longer exists.

The resulting shared path is:

```text
parse HTML --> collect stylesheets --> validate profile --> layout --> PDF
                                            |
                                            +--> errors, no PDF
```

`Validate` uses the same preparation and layout checks without PDF serialization.
This change removes only the importer retry; pagination passes remain necessary.

### 2. Remove code used exclusively by the importer

Delete `internal/browser` and its unit tests. Remove `Cascade.Declarations` and
update cascade comments that mention the importer. Retain the cascade machinery
used by normal style computation; do not fold this removal into a cascade rewrite.

Native `background-image`, `background-size: cover`, PNG/JPEG data URIs, `:root`
and adjacent-sibling selectors are existing profile capabilities. Their code and
tests remain in place. Moving a subset of importer transformations into the
validator would undermine the agreed closed-profile behavior and is excluded.

### 3. Preserve useful fixtures and test the new rejection behavior

Retain `testdata/browser/certificate.html` byte for byte and update its notice to
describe a rejection fixture. Replace the import acceptance tests with coverage
that `Convert` and `Validate` reject it with deterministic profile diagnostics,
produce no PDF and emit no `browser/*` codes.

Use focused cases for formerly lowered flex columns, positioned images,
border-box sizing, browser values, text transforms, cosmetic properties and
unsupported media rules. They must be rejected in ordinary mode as well as in
strict mode. Reuse existing profile rejection coverage where it already proves
the behavior.

Replace CLI certificate acceptance with failure coverage: exit `1`, profile
diagnostics, no new output and an existing output left untouched. Keep the
profile-compatible visual certificate and its goldens; rejection of the browser
fixture does not justify changing supported rendering snapshots.

### 4. Reconcile the WeasyPrint corpus using observed blockers

All six committed cases are currently blocked. After removing the harness option,
run them through the normal profile and review each first error code. Update
`blocked_by` in `testdata/weasyprint/samples.json` and the matching documentation
from those observations, rather than replacing every `browser/*` code with a
guessed CSS code.

Preserve the upstream documents, hashes, reference oracle and state enforcement.
Do not weaken blocker assertions or regenerate reference data to accommodate the
removal.

### 5. Record the break and reconcile the pending diagnostics plan

Delete `docs/browser-import.md` and remove current promises of lowering from the
README, package documentation, CSS profile documentation and contributor guide.
Add an Unreleased migration entry: remove the option from Go literals, rewrite
templates to the supported profile, and handle ordinary profile diagnostics.
Keep historical changelog entries and archived changes as historical records.

The pending `complete-diagnostics-contract` change contains task 2.3 and a
browser-import-with-invalid-page-size scenario. Track their reconciliation in
this change's tasks. During implementation, remove the obsolete import-specific
motivation, task and scenario from that change's proposal, tasks and library API
delta while retaining its independent early option-validation requirements.
This planning capture does not edit those existing artifacts.

## Risks / Trade-offs

- **Existing callers stop compiling:** document the removed public field and
  follow the project's breaking-release policy; do not preserve a no-op option.
- **Previously importable templates fail, including in the CLI:** state this
  change explicitly and retain the browser certificate as executable evidence.
- **Corpus rejection reasons change:** inspect actual first errors and keep the
  corpus's deterministic blocker checks enabled.
- **Over-removal could affect native features:** preserve their existing tests
  and supported rendering goldens; delete helpers only after checking consumers.
- **Pending work could reintroduce an importer assumption:** reconcile the named
  diagnostics artifacts as a tracked implementation task.

## Migration Plan

Apply the implementation tasks together so API callers, tests, documentation and
the corpus agree. Run the focused affected-package tests followed by the normal
repository checks. Synchronize the removal delta through the OpenSpec lifecycle
when the implementation is complete; do not edit main specifications during this
planning capture.

Release notes must describe the source and behavior incompatibility under the
existing versioning policy. Publishing or tagging a release is outside this
change. If the removal must be rolled back before release, revert the complete
change, including corpus expectations and documentation.
