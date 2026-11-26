# Spec Delta

## ADDED Requirements

### Requirement: The profile document states the supported script capabilities (RV5-04)

`docs/profile.md` SHALL state which text directions and scripts the engine
supports: bidi reordering for right-to-left runs and the `dir` attribute; Hebrew
and other non-joining right-to-left scripts; joining scripts refused with
`text/unsupported-script`; mirrored punctuation not substituted; hyphenation and
CJK line breaking unchanged. The section MUST NOT claim full Unicode shaping, and
a test MUST compare its script list with the detector the engine uses.

#### Scenario: A reader checks an Arabic template

- **WHEN** the reader looks up script support in the profile document
- **THEN** the document names `text/unsupported-script` and says Arabic is refused
- **AND** it states that Hebrew is supported by reordering alone

#### Scenario: The list drifts from the code

- **WHEN** a script is added to or removed from the engine's detector
- **THEN** the documentation test fails until the profile section follows
