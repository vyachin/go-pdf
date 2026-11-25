# Design

## Context

See `proposal.md`. The requirement mixes two independently failing rules —
"which root confines this reference" and "what to report when the file is
readable in principle but cannot be read" — and the validator flags its length.

## Goals / Non-Goals

**Goals:**

Make every requirement state one behaviour and keep the documented scenarios
intact, so a strict validation failure again means a new problem.

**Non-Goals:**

Changing confinement, symlink handling, `FontDir` resolution, the trusted
fallback-font rule or any diagnostic code.

## Decisions

1. **Confinement** keeps the root rules: stylesheets and images inside
   `BaseDir`, `@font-face` inside `FontDir` when set and `BaseDir` otherwise,
   absolute paths and escaping symlinks rejected, operator fallback fonts
   trusted, and the no-root behaviour described.

2. **Read failure** owns the single rule that a file which exists inside the
   root but cannot be read is reported with its reason and never as a boundary
   escape. Its scenario moves with it.

3. `REMOVED` plus two `ADDED` requirements express the split; the sync places the
   two new blocks where the old one stood, so the document order a reader is used
   to does not change.

## Risks / Trade-offs

A REMOVED requirement could look like a lost contract. Both new blocks quote the
old scenarios verbatim and the change's tasks require a scenario-by-scenario
comparison, so nothing is dropped.

## Migration Plan

Specification only. Revert the commit if the split loses a scenario.
