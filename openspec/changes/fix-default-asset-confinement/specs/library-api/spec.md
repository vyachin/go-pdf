# Spec Delta

## ADDED Requirements

### Requirement: Asset references are confined by default (RV5-07)

A relative asset reference SHALL resolve against its root: `BaseDir`, or `FontDir`
for fonts and then `BaseDir`, then the directory of `Options.File`, then the process
working directory. Every relative reference MUST stay inside that root. An absolute
reference MUST be rejected unless it lies inside a configured root or
`Options.AllowAbsoluteAssets` is set. A rejection MUST produce the resource's load
diagnostic and no PDF.

#### Scenario: An absolute image with no configured root

- **WHEN** a document with `Options{File: "invoice.html"}` references
  `<img src="/etc/hosts">`
- **THEN** the call returns `img/load` naming the reference and no PDF
- **AND** the file is not embedded

#### Scenario: An absolute stylesheet and font with no configured root

- **WHEN** the same zero-root configuration links `/etc/hosts` as a stylesheet and
  declares it as an `@font-face` source
- **THEN** the stylesheet fails with `css/link-path` and the font with
  `font/unavailable`
- **AND** neither file is read as CSS or as a font

#### Scenario: The document directory is the default root

- **WHEN** `Options{File: "/srv/docs/invoice.html"}` converts `img src="logo.png"`
  and no `BaseDir` is set
- **THEN** `/srv/docs/logo.png` is the file that is read

#### Scenario: A configured root permits inside and denies outside

- **WHEN** `BaseDir` is `/srv/assets` and the document references `logo.png`,
  `../secret.png` and `/etc/hosts`
- **THEN** `logo.png` under `/srv/assets` is read
- **AND** `../secret.png` and `/etc/hosts` fail with `img/load` and no PDF

#### Scenario: The opt-in restores reading an absolute path

- **WHEN** `AllowAbsoluteAssets` is true and no root is configured, and the
  document references an absolute image, stylesheet and font
- **THEN** each existing file is read as written and the conversion succeeds

#### Scenario: No root and no file name uses the working directory only

- **WHEN** both `File` and the roots are empty, and the document references
  `logo.png` and `../secret.png`
- **THEN** `logo.png` resolves in the working directory and `../secret.png` fails
  with the resource's load diagnostic
