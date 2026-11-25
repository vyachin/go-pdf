# Proposal

## Why

Dependabot is already configured, but only action references are specified. The `gomod` entry in `.github/dependabot.yml` was written when the module had no requirements — its comment still calls it dormant — and it carries no cooldown and no grouping. The module now requires `golang.org/x/net` and `golang.org/x/text`, with `golang.org/x/image` planned, so module updates will start arriving with no written policy: no grouping rule, no cooldown, and nothing about `go.sum`, the `go` directive or the allowlist. The new dependency guard will reject a third-party module, but it says nothing about how a routine `x/` bump is reviewed.

## What Changes

- The `gomod` entry SHALL become a first-class, specified behavior: weekly, a cooldown for a fresh release, a pull-request limit, a `chore` commit prefix, the `dependencies` label and a group for patch updates. The dormant comment SHALL go.
- A module update pull request SHALL refresh `go.sum`, leave the `go` directive unchanged, and add or remove no module requirement; a release that renames the module path is therefore a human change, not an automatic one.
- Because every required module is currently below v1, a minor or major update SHALL arrive alone and only patch updates MAY be grouped; the actions policy keeps its own grouping.
- The manual-pin scenarios SHALL speak of a dependency reference rather than an action reference, so they cover module pull requests too.
- `CONTRIBUTING.md` SHALL describe both ecosystems in one paragraph.

Not included: auto-merge, a second Dependabot configuration file, upgrading a module in this change, and Dependabot security updates as committed policy — that is a repository setting, recommended in the design and verified by hand.

## Capabilities

### Modified Capabilities

- `code-quality`: requirements for automated module updates and for the properties of a module update pull request, plus generalized wording in the manual-pin scenarios.

## Impact

- **CI**: `.github/dependabot.yml` only; no workflow change.
- **Repository settings**: Dependabot security updates enabled; auto-merge stays off (manual tasks).
- **Documentation**: `CONTRIBUTING.md`.
- **Product code**: none; no module is upgraded here.
