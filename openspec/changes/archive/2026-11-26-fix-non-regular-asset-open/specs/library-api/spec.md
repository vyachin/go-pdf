# Spec Delta

## ADDED Requirements

### Requirement: Only regular files are opened as assets (RV5-08)

An asset reference SHALL be read only when it resolves to a regular file. A
directory, device, FIFO, socket or other non-regular file MUST fail with the
resource's load diagnostic — `img/load`, `css/link-load` or `font/unavailable` —
and MUST NOT be opened in a way that waits for a writer. The regular-file check
MUST NOT weaken root confinement or symlink resolution: a symlink to a non-regular
file MUST be rejected like the file itself.

#### Scenario: A FIFO referenced as an image

- **WHEN** a document references a FIFO as `<img src="probe.fifo">` and no writer
  will ever open it
- **THEN** the conversion returns `img/load` and no PDF
- **AND** the call completes instead of waiting in `open(2)`

#### Scenario: A FIFO referenced as a stylesheet

- **WHEN** a document links the same FIFO with `rel="stylesheet"`
- **THEN** the conversion returns `css/link-load` and no PDF
- **AND** the call completes instead of waiting in `open(2)`

#### Scenario: A FIFO referenced as a font

- **WHEN** a document declares the FIFO as an `@font-face` source
- **THEN** the conversion returns `font/unavailable` and no PDF
- **AND** the call completes instead of waiting in `open(2)`

#### Scenario: A directory is referenced

- **WHEN** an image, stylesheet or font reference resolves to a directory
- **THEN** the resource's load diagnostic is reported and no PDF is produced

#### Scenario: A symlink to a regular file inside the root

- **WHEN** an asset reference resolves through a symlink to a regular file that is
  inside the configured root
- **THEN** the file is read and the conversion succeeds

#### Scenario: A symlink to a FIFO inside the root

- **WHEN** an asset reference resolves through a symlink to a FIFO whose target is
  inside the root
- **THEN** the reference is rejected as non-regular with the resource's load
  diagnostic
