# Proposal

## Why

A configured `BaseDir` that cannot be opened leaves the stylesheet root `nil`; the
`nil` root means "no boundary", so a linked absolute path is read outside the
configured root and the failure is reported as `css/link-load` with no position. The
reference command also has two unsafe edges: `pdfgen template.pdf` resolves the
default output to the input path and truncates the source, and it reads the whole
input with `os.ReadFile` before the package's 8 MiB limit can reject it. The package
offers no public way to read the default limits, although `engine.DefaultLimits` exists
for exactly that purpose.

## What Changes

- A configured asset root that cannot be opened SHALL be an `options/asset-root`
  error and MUST stop asset reads; it MUST NOT fall back to unbounded reads.
- The command SHALL refuse to write when the resolved output is the input file, and
  SHALL write through a temporary file renamed into place after a complete PDF exists.
- The command SHALL check the input size against the public default limits before
  reading it.
- `pdfgen` SHALL export `DefaultLimits()` returning the zero-value defaults.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `css-profile`: a failed asset root stops reads instead of escaping confinement.
- `reference-cli`: the output never overwrites the input, and the input is size-checked.
- `library-api`: default limits are readable by callers.

## Impact

- **Code**: `internal/profile/styles.go`, `internal/fsx/fsx.go`,
  `internal/engine/fonts.go`, `cmd/pdfgen/main.go`, `pdfgen/pdfgen.go`.
- **Output**: a misconfigured root fails early with a configuration code; the command
  cannot destroy its input and refuses oversized files without loading them.
- **Tests**: root-failure, output==input, temp+rename and oversized-input regressions.
