# Design

## Context

`.github/dependabot.yml` already has two entries. `github-actions` is fully specified: weekly on Monday, a seven-day default cooldown and a thirty-day cooldown for majors, a limit of three open pull requests, one group for minor and patch bumps and one for majors, a `ci` commit prefix and the `dependencies` label. `gomod` is a placeholder from when the module had no requirements: weekly on Monday, a limit of three, a `chore` prefix, the label, and a comment that calls it dormant.

`go.mod` now requires `golang.org/x/net` and `golang.org/x/text`; `golang.org/x/image` follows with the font and CFF work. All are v0 modules. The `code-quality` capability specifies the actions entry, the manual pins, the no-auto-merge rule and, once it lands, the runtime dependency guard. `CONTRIBUTING.md` documents the actions policy and nothing about modules.

See `proposal.md` for motivation.

## Goals / Non-Goals

**Goals:**

- Module updates are a specified behavior, not an accidental consequence of a placeholder entry.
- A module pull request is small enough to review and carries everything the build needs.
- The manual pins and the allowlist stay untouched by automation.

**Non-Goals:**

- Upgrading a module in this change.
- A second configuration file or a separate policy per module.
- Auto-merge.
- Dependabot security updates as committed configuration: they are a repository setting.

## Decisions

### 1. The existing `gomod` entry becomes the policy

One file, one shape: the module entry gains the same schedule, limit, prefix and label it had, plus a cooldown and a patch group, and its dormant comment is replaced by the rules it now follows. A second file or a second entry per module would hide the policy in duplication.

### 2. A seven-day cooldown for modules, without a separate major cooldown

`cooldown.default-days: 7` for modules. The actions entry keeps its thirty-day major cooldown, because a new action major changes the runtime every CI job executes. A module major already arrives alone and the tests run against it; a longer wait would only delay a fix. The cooldown exists to keep a release published an hour ago out of the next job.

### 3. Patch updates are grouped; a minor or major is alone

Every required module is currently v0, where the semantic-versioning rules allow a minor release to change the API. A grouped patch pull request is routine; a minor is where the reviewer has to read the release notes, so it gets its own pull request. The actions policy keeps its own grouping — an action bump cannot change the package's API, a module bump can.

```yaml
groups:
  modules-patch:
    patterns: ["*"]
    update-types: ["patch"]
```

Minor and major updates match no group, so Dependabot opens one pull request each, bounded by the existing limit of three.

### 4. Rules the configuration cannot express stay in the spec, with the guard behind them

`go.sum` is refreshed by Dependabot itself; the `go` directive and a requirement's presence are not touched by a version update. The one case worth stating is a major that moves a module to a new path: Dependabot can propose it, and it would add a requirement. The spec forbids that automatically — it is a `code-quality` decision — and the reviewer enforces it. The runtime dependency guard cannot help here, because a new path may still be a `golang.org/x/...` module; review is the control.

### 5. The manual-pin scenarios speak of dependency references

The requirement already covers the `go` directive; its scenarios only said "an action reference". Changing that phrase to "a dependency reference" makes the same rule apply verbatim to a module pull request, with no behavior change.

### 6. Security updates are a repository setting

Dependabot security updates are enabled in the repository settings, not in `dependabot.yml`, and cannot be verified from the repository. The design recommends enabling them and records that they follow the same no-auto-merge rule; the spec does not state an unverifiable requirement. The task list includes the manual check.

## Risks / Trade-offs

- **Pull-request noise** → the limit of three open pull requests, a patch group, a weekly schedule and a cooldown.
- **A minor release breaks the API** → the full suite, the golden layers, the race detector and the validators run on the pull request; the solo pull request makes the release notes visible.
- **A configuration field rejected for the ecosystem** → GitHub reports a configuration error after the merge; the task checks the Dependabot page and removes a field the ecosystem does not accept.
- **A path-renaming major passing the guard** → review, guided by the spec sentence; no automated control can tell a legitimate `x/` module from a renamed one.
- **Security updates cannot be asserted in the repository** → manual verification in the settings, recorded in the tasks.

## Migration Plan

None: a configuration entry, a specification and a documentation paragraph. Reverting restores the placeholder entry and withdraws the two requirements.
