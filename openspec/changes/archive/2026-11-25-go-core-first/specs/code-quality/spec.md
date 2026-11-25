# Spec Delta

## ADDED Requirements

### Requirement: The Go project's implementation is preferred

Where the standard library or a `golang.org/x/...` module provides a facility the engine needs, the engine SHALL use it and MUST NOT commit a private substitute. A private implementation MAY remain only where the Go project's module does not cover the need, and each exception MUST be named with its reason in `CONTRIBUTING.md`.

#### Scenario: Normalization

- **WHEN** text must be brought to canonical composition
- **THEN** `golang.org/x/text/unicode/norm` performs it, and no composition table or generator is committed

#### Scenario: A JPEG header

- **WHEN** the writer needs the width, height or colour components of a JPEG
- **THEN** `image/jpeg` reports them, and no marker parser is committed

#### Scenario: UTF-16

- **WHEN** PDF strings, ToUnicode maps or extracted text are converted to or from UTF-16
- **THEN** `unicode/utf16` performs it, surrogate pairs included

#### Scenario: A justified exception

- **WHEN** a private implementation remains in the repository
- **THEN** `CONTRIBUTING.md` names the facility and why the Go project's module does not cover it

#### Scenario: The exception disappears

- **WHEN** a Go-project module starts covering a recorded exception
- **THEN** the change that adopts the module removes the private implementation
