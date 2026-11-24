# Spec Delta

## MODIFIED Requirements

### Requirement: Local assets stay inside their configured root

Every local file named by the document SHALL be opened inside a configured root: stylesheets and images inside `BaseDir`, `@font-face` files inside `FontDir` when set, otherwise `BaseDir`. Absolute paths and symbolic links that leave the root MUST be rejected with a diagnostic naming the asset. A file that cannot be read for another reason MUST be reported with that reason rather than as a boundary escape. Operator-provided fallback fonts are trusted paths and are outside this rule. When no root is configured, the engine reads paths as before.

#### Scenario: Image outside the base directory
- **WHEN** an `<img src>` is an absolute path outside `BaseDir`
- **THEN** conversion fails with a diagnostic naming the path
- **AND** the file is not read

#### Scenario: Image reached through an escaping symlink
- **WHEN** `BaseDir` contains `logo.png` that links to a file outside it
- **THEN** conversion fails with a diagnostic
- **AND** the outside file is not read

#### Scenario: Image inside the base directory
- **WHEN** the document references an image that exists inside `BaseDir`, including through a symlink that stays inside
- **THEN** the image is loaded and embedded

#### Scenario: Unreadable file inside the root
- **WHEN** an asset exists inside the root but cannot be read because of permissions
- **THEN** the diagnostic reports the access error
- **AND** it does not claim that the path escapes the boundary

#### Scenario: Font outside the font root
- **WHEN** `@font-face` names a file outside `FontDir`
- **THEN** conversion fails with a `font/unavailable` diagnostic

#### Scenario: Trusted fallback font
- **WHEN** the run passes a fallback font path outside the roots
- **THEN** it is loaded as configured

#### Scenario: No root configured
- **WHEN** neither `BaseDir` nor `FontDir` is set
- **THEN** relative paths resolve from the working directory as before
