# Tasks

## 1. Enforce the depth

- [x] 1.1 Add a nesting counter to the CSS parser and return a `resource/limit` problem at 65 levels — verify: a 500,000-level `@media` sheet parses to one diagnostic and no stack growth.
- [x] 1.2 Make `RulesForPrint` and `profile.validateRules` iterative or depth-capped with the same limit — verify: the same sheet reaches validation and returns the same diagnostic.
- [x] 1.3 Add a subprocess regression that runs the 4.5 MB sheet and asserts exit code 0 with a diagnostic — verify: the test fails on the parent commit (exit code 2) and passes after the fix.

## 2. Tests and documentation

- [x] 2.1 Add a fuzz seed for nested at-rules and keep `FuzzParse` green — verify: `go test ./internal/css -run '^$' -fuzz '^FuzzParse$' -fuzztime 10s` reports no new crash.
- [x] 2.2 Document the nesting limit in `docs/profile.md` — verify: the documented value matches the parser constant.
