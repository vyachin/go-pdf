# Design

## Context

See `proposal.md` for the finding and review reference.

`fontRegistry.loadFontFile` in `internal/engine/fonts.go` returns only `*font.Font` and discards `admitFont`, limited-read and parse errors. `loadSystemFamily` treats every nil as absence. The shared guard records parser exhaustion but not a per-file or aggregate refusal, so the comment that the guard carries the reason is inaccurate.

RV4-44 covers work charges for fallback parsing and RV4-62 suppresses errors after guard exhaustion. This change separately preserves per-file and aggregate-limit errors that never enter the guard; coordinate the same functions without duplicating those requirements.

## Goals / Non-Goals

**Goals:**

Return the actual resource cause instead of changing typography or diagnosing a missing family.

**Non-Goals:**

Adding public configuration, external dependencies or unrelated HTML/CSS features.

## Decisions

1. Return a typed outcome/error from fallback loading. A missing candidate may continue search; a documented bound or context error is terminal for the conversion.

2. Propagate terminal errors through `systemFaces`, `pick` and `Face`, or store the first registry failure explicitly; never infer resource failure from a nil face.

3. Translate budget/font limit errors to one `resource/limit` and return cancellation unchanged, while preserving existing behavior for genuinely absent candidates.

**Alternative considered:** A nil face plus the current guard is insufficient because per-file and aggregate failures do not enter that guard; propagate a typed cause.

## Risks / Trade-offs

Production font discovery must be tested with injected candidates, not host-installed fonts or a new environment hook. Stopping on a limit is an intentional correction to the documented failure semantics.

## Migration Plan

Land the owning-package regressions and implementation together. Review intentional output or diagnostic changes before updating affected goldens; retain the ordinary-input checks. No public API migration or new dependency is required. Revert the implementation commit and its snapshot updates together if a regression is found.
