# Spec Delta

## ADDED Requirements

### Requirement: Stylesheet paths cannot escape through symlinks

When a base directory is configured, the engine SHALL open stylesheets through a root confined to that directory, so a symbolic link inside the base that points outside MUST be rejected. Lexical rejections of `..` and of absolute paths outside the base MUST keep working, and symlinks that stay inside the base MUST be followed.

#### Scenario: A symlink pointing outside
- **WHEN** the base directory contains `link.css` pointing to a file outside it and the document links `link.css`
- **THEN** validation fails with `css/link-path` and the outside file is not read

#### Scenario: A symlink inside the base
- **WHEN** the base directory contains `link.css` pointing to another stylesheet inside it
- **THEN** the stylesheet loads and its rules apply

#### Scenario: Lexical escape still rejected
- **WHEN** the document links `../../outside.css`
- **THEN** validation fails with `css/link-path`

#### Scenario: No base directory configured
- **WHEN** no base directory is configured
- **THEN** relative paths resolve from the working directory as before
