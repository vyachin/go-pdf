# Design

## Context

`fsx.Open` returns `(nil, err)` on failure; `Root.Open` interprets a nil receiver as
"no boundary". `imageLoader` already has the right guard pattern; stylesheet
collection does not. The CLI writes with `os.WriteFile` directly.

## Goals / Non-Goals

**Goals:** no fallback to unbounded reads; the CLI cannot destroy its input; the
public defaults are usable.

**Non-Goals:** sandboxing the CLI beyond the existing root rules; changing the exit
code table.

## Decisions

- When `fsx.Open` fails, return `options/asset-root` from `prepare` and stop before
  collecting stylesheets or fonts. Never pass a nil root to `readConfined`; make
  `Root.Open` return `ErrOutsideRoot` for a nil receiver with a configured root.
- Export `pdfgen.DefaultLimits()` returning `engine.DefaultLimits()` (already the
  resolved zero value).
- In the CLI: `os.Stat` the input and compare `MaxInputBytes` before `ReadFile`; resolve
  the output with `filepath.Abs` + `os.SameFile` against the input and refuse a match;
  write to `os.CreateTemp(dir, ".pdfgen-*")`, `Sync`, `Close`, `Rename`, and remove the
  temp file on any error.
- Keep `-o ""` as "not set"; document that `-` is not stdout (or reject it explicitly).

## Risks / Trade-offs

- Refusing `output == input` may surprise a caller who intentionally overwrites a
  generated PDF named as input → the input is HTML, so the case is always a mistake;
  report it clearly.
- Temp+rename changes permissions on the output → preserve `0o644` and document it.
