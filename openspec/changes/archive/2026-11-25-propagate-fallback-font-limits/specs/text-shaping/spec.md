# Spec Delta

## ADDED Requirements

### Requirement: System fallback discovery preserves resource failures

System fallback discovery SHALL distinguish an unavailable candidate from a
per-file, aggregate or parser resource limit. A resource refusal MUST terminate
the conversion with `resource/limit`; discovery MUST NOT continue to another
family or silently select a different face because of that refusal. Cancellation
MUST be returned as the context error. A terminal resource error MUST NOT generate
a dependent `text/font` missing-family diagnostic.

#### Scenario: An aggregate cap refuses the fallback

- **WHEN** an injected available system font exceeds `MaxTotalFontBytes: 1`
- **THEN** both public entry points report `resource/limit` instead of `text/font`

#### Scenario: A variant exceeds the total

- **WHEN** the regular face fits but loading the selected family's variant crosses the aggregate limit
- **THEN** conversion fails with `resource/limit` rather than silently changing the selected typography

#### Scenario: An absent candidate permits discovery

- **WHEN** a candidate file does not exist and a later candidate is available within all limits
- **THEN** discovery may select the later candidate without a resource diagnostic

#### Scenario: Fallback discovery observes cancellation

- **WHEN** the context is cancelled during fallback loading
- **THEN** the call returns the context error without trying the remaining candidates
