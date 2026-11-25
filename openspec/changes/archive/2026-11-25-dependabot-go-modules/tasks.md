# Tasks

## 1. Configuration

- [x] 1.1 Turn the `gomod` entry into the specified policy: add `cooldown.default-days: 7`, add the `modules-patch` group (`patterns: ["*"]`, `update-types: ["patch"]`), keep the weekly schedule, the limit of three, the `chore` prefix and the label — check: the YAML parses and the entry matches the design
- [x] 1.2 Replace the "dormant" comment with the rules the entry now follows: patch grouped, minor and major alone, the `go` directive manual, no new requirement — check: the comment names the spec requirement it implements
- [x] 1.3 Explain in the file why actions keep a thirty-day major cooldown and modules do not — check: the comment is one or two lines and matches the design

## 2. Verification

- [x] 2.1 Parse the file locally as YAML and review it against the Dependabot schema — check: no unknown keys, `groups` and `cooldown` under the `gomod` entry
- [ ] 2.2 After the merge, open the repository's Dependabot page and confirm the configuration is accepted and the module entry is active — check: no configuration error, the entry lists the watched modules
- [ ] 2.3 Confirm the first scheduled run behaves as specified: one pull request for patch updates, a separate one for a minor — check: the pull requests match the grouping rule

## 3. Repository settings

- [x] 3.1 Enable Dependabot alerts and security updates — check: the setting is on in the repository's code-security settings
- [x] 3.2 Confirm auto-merge is still disabled for dependency pull requests — check: no auto-merge workflow and the setting is off

## 4. Documentation and acceptance

- [x] 4.1 `CONTRIBUTING.md`: one paragraph covering both ecosystems, the module grouping rule and the manual `go` directive — check: the paragraph replaces the actions-only text
- [x] 4.2 `openspec validate dependabot-go-modules --strict` — check: valid
- [x] 4.3 `go vet ./...` and `go test ./...` — check: green, although no product code changed
