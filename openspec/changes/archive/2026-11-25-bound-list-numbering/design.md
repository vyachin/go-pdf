# Design

## Context

See `proposal.md` for the finding and review reference.

`internal/layout/box.go` calls `liIndex` from `buildElement`; `liIndex` walks `n.Parent.Children` from the beginning for each item. The same calculation runs for unordered lists even though they usually draw bullets. The existing `Lists` requirement covers visible markers but does not constrain this repeated scan.

No overlapping active change was found for this finding.

## Goals / Non-Goals

**Goals:**

Bound numbering work without changing supported list syntax or ordinary output.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Carry the next ordinal in the parent traversal, or precompute a parent-local map in one pass; never rescan a prefix for each child.

2. Scope numbering to the immediate list parent and preserve the current interpretation of `ol[start]`, nested lists and non-list siblings.

3. Charge each child visited before processing it and let the existing guard propagate cancellation and `resource/limit`; no elapsed-time assertion.

**Alternative considered:** Charging the old quadratic scan would bound it but retain unnecessary work; a single traversal removes the cause and makes accounting proportional.

## Risks / Trade-offs

Caching indexes globally would retain DOM nodes and violate conversion isolation; keep state in the builder. Existing numbering must be pinned before replacing the helper.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
