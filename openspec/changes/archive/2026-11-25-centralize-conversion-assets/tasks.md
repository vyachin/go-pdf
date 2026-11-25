# Tasks

## 1. Implementation and verification

- [x] 1.1 Introduce `internal/assets` and its narrow adapter interfaces, owned by one engine conversion and retaining `fsx.Root` confinement. Verification: package imports remain acyclic, every exit releases the registry, and focused tests preserve typed resource causes.
- [x] 1.2 Migrate images and fonts, sharing permitted identities and successful byte accounting across declared/explicit fallback uses. Verification: one permitted font used by declaration and fallback is read/parsed/admitted once.
- [x] 1.3 Route stylesheet reads through the shared infrastructure while keeping document-order inclusions and charset-aware parsing. Verification: the repeated-stylesheet/intervening-rule case preserves exact cascade order and encoding.
- [x] 1.4 Test duplicate paths, identical font declarations/fallback, distinct roots, symlink escapes, failure accounting and cleanup on cancellation. Verification: all isolation, confinement, cleanup and failed-admission regression cases pass.
- [x] 1.5 Verify cached digests and propagated font limits remain correct; run asset/profile/engine tests, the shuffled suite and race tests. Verification: digest-count, fallback-limit and asset/profile/engine tests pass under the shuffled/race suites.

## 2. Acceptance

- [x] 2.1 Verify every scenario in this change, including the review reproduction, and confirm the related changes retain their separate scope.
- [x] 2.2 Run `openspec validate centralize-conversion-assets --strict` and review the diff for unintended changes outside this proposal.
