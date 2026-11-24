# Tasks

## 1. Dependabot configuration

- [x] 1.1 Add `.github/dependabot.yml` with `version: 2`, `package-ecosystem: github-actions`, `directory: /`, a weekly schedule, `cooldown` (`default-days: 7`, `semver-major-days: 30`), two groups (`actions-minor` for minor and patch, `actions-major` for major), `commit-message.prefix: ci` and the `dependencies` label - verification: `ruby -ryaml -e 'p YAML.load_file(".github/dependabot.yml")["updates"].map { |u| [u["package-ecosystem"], u["directory"], (u["groups"] || {}).keys] }'` prints both entries and no YAML error occurs
- [x] 1.2 Add the dormant `gomod` entry for `/` (weekly, `commit-message.prefix: chore`, `dependencies` label) and leave `go.mod` untouched - verification: `go.mod` still declares no `require` and `git status --short go.sum` reports nothing
- [x] 1.3 Confirm the `github-actions` entry covers every reference in the workflow: four distinct actions across the two jobs, with `lint` still behind `test` on `checkout` and `setup-go` - verification: `grep -o 'uses: [^ ]*' .github/workflows/ci.yml` lists only actions covered by the entry (the ecosystem watches `.github/workflows/` as a whole, so no per-action entry is expected)

## 2. Documentation

- [x] 2.1 Add the update-policy paragraph to `README.md` next to the existing CI description: weekly grouped action updates, majors in their own pull request with a human review, no auto-merge, and the manual pins (`ubuntu-24.04` with its poppler, `pdfcpu`, the linter version, the `go` directive) - verification: the paragraph names `.github/dependabot.yml` and lists exactly the pins that the `code-quality` delta excludes from automation; the rendered Markdown reads correctly around the neighbouring paragraphs

## 3. Workflow consistency

- [x] 3.1 Bring both jobs onto one major per action, by letting the first major-group pull request move `lint`'s `checkout` and `setup-go` to `v7`, or by editing the two references directly if that pull request is rejected or delayed - verification: `grep -n 'actions/checkout@\|actions/setup-go@' .github/workflows/ci.yml` shows one major per action
- [x] 3.2 Confirm no merge automation exists or was added: no `dependabot/fetch-metadata` step, no `gh pr merge`, no auto-approve workflow - verification: `grep -rn 'fetch-metadata\|pr merge\|auto-merge' .github/` returns nothing

## 4. Activation and acceptance

- [ ] 4.1 Merge the configuration to `main`, because Dependabot reads it only from the default branch, then confirm the first scheduled run produced the expected pull requests: one grouped minor/patch and, separately, one grouped major that aligns the `lint` job - verification: the pull requests carry the `ci` commit prefix and the `dependencies` label, and each opened against the expected refs
- [ ] 4.2 Confirm the dependency pull requests leave the coupled pins alone: `ubuntu-24.04`, `poppler-utils`, `pdfcpu@v0.11.0`, the `golangci-lint` version input and `go 1.24` are unchanged in their diffs - verification: review each pull request's diff for those strings and find none

## 5. Change validation

- [x] 5.1 Run `openspec validate dependabot-updates --strict` - verification: the command exits successfully and reports no warnings
