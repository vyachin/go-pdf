# weasyprint-corpus Specification

## Purpose

Maintains a pinned corpus of third-party paged-media sample documents, keeps our
rendering of them frozen page by page, and measures it against the reference
output produced by the engine that authored the corpus, without which the
corpus's distance from this profile would be invisible.

## Requirements

### Requirement: Provenance is pinned and verifiable

The corpus SHALL record the upstream repository and the exact commit its
documents came from, and every vendored file MUST be a byte-identical copy whose
hash is committed beside it. Replacing the documents with a newer upstream
commit MUST regenerate the hashes, the reference oracle and our own goldens in
the same change.

#### Scenario: The pin is recorded

- **WHEN** a maintainer opens the corpus notice or manifest
- **THEN** the upstream repository URL and the commit it was taken from are named

#### Scenario: A modified sample is detected

- **WHEN** a vendored file no longer matches its committed hash
- **THEN** the suite fails and names the file and its upstream path

### Requirement: Only redistributable material is vendored

The corpus MUST NOT contain a document whose content is not licensed for
redistribution. A sample that is left out MUST be named with its reason, so the
corpus is not mistaken for the complete upstream set. Bundled fonts MUST keep
their own licence texts.

#### Scenario: An excluded sample is documented

- **WHEN** a reader opens the corpus notice
- **THEN** every excluded sample is listed with the reason it is absent

#### Scenario: Font licences travel with the fonts

- **WHEN** the corpus bundles a font
- **THEN** the licence text of that font family is committed next to it

### Requirement: Each case declares a state the suite enforces

Every case in the manifest SHALL declare one of three states: rejected by the
profile, converting but not yet faithful, or accepted. The suite MUST fail when
the observed behaviour disagrees with the declared state, so a promotion or a
regression is a reviewed edit rather than an unnoticed change.

#### Scenario: A blocked case starts converting

- **WHEN** a case declared blocked produces a PDF
- **THEN** the suite fails and names the state the case must be promoted to

#### Scenario: An accepted case regresses

- **WHEN** an accepted case no longer converts
- **THEN** the suite fails and shows its diagnostics

### Requirement: A blocked case is rejected with a stable reason

A case declared blocked MUST be rejected with at least one error diagnostic, MUST
NOT panic, and MUST produce identical diagnostics for two conversions of the same
input. The first diagnostic code MUST equal the code committed for that case, so
a change in what blocks the sample is visible in review.

#### Scenario: The blocker changes

- **WHEN** a change removes the diagnostic that was committed as the first
  blocker of a case
- **THEN** the suite fails and names both the committed and the observed code

#### Scenario: A blocked document panics

- **WHEN** processing a blocked case panics
- **THEN** the suite fails with the stack trace instead of reporting a rejection

### Requirement: A converting case freezes its page text

For a case that converts, the suite SHALL compare the text of our produced PDF
page by page with a committed golden, and MUST fail on any difference, including
a different number of pages. Regeneration MUST be explicit, a missing golden MUST
fail, and a golden without a case MUST fail.

#### Scenario: Text differs

- **WHEN** the produced page text differs from the golden
- **THEN** the failure shows the page number, the first differing lines and the
  path of the produced file

#### Scenario: Regeneration is explicit

- **WHEN** the suite runs with the update flag
- **THEN** it rewrites the goldens of the cases it processed and leaves the
  oracle and the upstream documents untouched

### Requirement: Fidelity to the reference output is measured from committed data

The corpus SHALL keep, for every case, the page count and the normalised text of
the reference PDF that upstream produced, in a committed oracle. For an accepted
case the suite MUST require the same page count and the same multiset of tokens;
for a converting but not accepted case it SHALL report the difference without
failing. The suite MUST run offline and MUST NOT require a PDF tool at test time.

#### Scenario: An accepted case has the wrong page count

- **WHEN** our PDF has a different number of pages than the oracle
- **THEN** the suite fails and reports both counts

#### Scenario: An accepted case lost content

- **WHEN** tokens present in the oracle are missing from our PDF, or extra tokens
  appear
- **THEN** the suite fails and reports the missing and the extra tokens

#### Scenario: Measurement before acceptance

- **WHEN** a case is in the converting-but-not-accepted state
- **THEN** the page and token difference is reported in the test log without
  failing the suite

#### Scenario: The suite needs no network and no external tool

- **WHEN** the suite runs on a machine without network access and without
  poppler
- **THEN** every case still runs from committed files
