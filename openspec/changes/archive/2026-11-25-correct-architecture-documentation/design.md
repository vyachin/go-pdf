# Design

## Context

See `proposal.md` for the finding and review reference.

`CONTRIBUTING.md` is the architecture entry point. The repository contains `internal/pdf/writer.go`, `internal/engine/convert.go`, `internal/layout`, `internal/text/shape.go` and `internal/backend/raster`; it has no `internal/backend/pdf` package. `text.Scale` and PDF descriptor conversion use floating point.

No overlapping active change was found for this finding.

## Goals / Non-Goals

**Goals:**

Make the contributor diagram a reliable navigation and ownership guide.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Keep one concise diagram with real repository paths and arrows matching imports/control flow; place the debug raster as another layout consumer, not a production PDF prerequisite.

2. Document that geometry is stored in integer millipoints while some scaling uses explicit rounding; do not claim that floating point is absent.

3. Describe current code. Label the registry and source-provenance proposals as future work until they are implemented instead of advertising planned refactors as delivered.

**Alternative considered:** Keeping a purely conceptual nonexistent package name makes navigation misleading; use actual package paths and identify conceptual stages separately.

## Risks / Trade-offs

Architecture prose can drift after package moves. Validate paths and ownership during review using tracked files; avoid brittle tests that only compare prose strings.

## Migration Plan

Update the documentation and its capability requirements together. Runtime behavior and dependency versions are unchanged; revert the documentation commit if the wording is inaccurate.
