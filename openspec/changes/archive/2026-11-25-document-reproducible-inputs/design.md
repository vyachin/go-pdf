# Design

## Context

See `proposal.md` for the finding and review reference.

`README.md` and `pdfgen/doc.go` make the abbreviated guarantee. `docs/fonts.md` already recommends shipping exact fonts and explains system-font variability. The current `pdf-output` requirement mentions fonts but omits external CSS and image bytes.

The fourth-review design mentions the same-font condition but has no dedicated complete-input documentation delta. RV4-57 separately removes a test environment hook; this proposal does not duplicate that implementation.

## Goals / Non-Goals

**Goals:**

Keep the deterministic output guarantee precise and reproducible without changing runtime behavior.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Define identical inputs as the engine build, HTML bytes, options, resource bytes and font/path resolution choices. A file changed in place is a changed input.

2. Explain that the fixed default date eliminates clock dependence, while a system fallback is development convenience and can vary by environment. Recommend explicit fonts and immutable resource bundles.

3. Do not freeze output across engine releases or introduce a global asset cache. The removal of the production `PDFTEST_FONT` hook remains owned by RV4-57.

**Alternative considered:** Mentioning font pinning alone leaves CSS and image mutations outside the written promise; define all resolved asset bytes as part of the input.

## Risks / Trade-offs

An abbreviated statement must not imply a promise across changed external inputs or engine versions. This clarifies the existing reproducibility scope and does not justify nondeterminism for identical complete inputs.

## Migration Plan

Update the documentation and its capability requirements together. Runtime behavior and dependency versions are unchanged; revert the documentation commit if the wording is inaccurate.
