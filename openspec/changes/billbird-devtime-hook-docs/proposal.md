## Why

A separate time-source from `/log` issue comments has been discussed: a pre-commit hook on each dev's machine that estimates the time elapsed since the previous commit, writes it into the commit message as a `Time: 1h30m` footer, and lets the dev confirm before push. Two sources (issues + commits) then enable an A/B reconciliation: hours on `/log` should roughly match summed `Time:` footers per issue, surfaced as discrepancies in a reporting tool.

The hook itself is a few lines of bash plus a JSON snippet for `~/.claude/settings.json`. No new Billbird code, no new package, no new repo. Devs clone Billbird, copy two blocks into their local Claude Code settings, and the hook starts running on their next commit.

This change ships only the documentation. The reconciliation tool will land in Gitsweeper as a follow-up.

## What Changes

- New `docs/dev-time-hook.md`:
  - What the hook does (estimate from `git log -1`'s timestamp + working-file activity, present a guess via `claude` headless mode, dev edits or accepts, append `Time: 1h30m` to the commit message).
  - JSON snippet for `~/.claude/settings.json` — the `hooks.PreCommit` config block.
  - Shell script body — short, audit-friendly, no dependencies beyond `claude` CLI and standard POSIX tools.
  - A/B reconciliation note: the `Time:` footer is the second source of truth alongside `/log` comments; Gitsweeper will compare.
  - Security/safety: the hook runs locally as the dev, sees only their own working copy, writes only the commit message. No network beyond the Claude CLI itself.
- Cross-reference in `docs/commands.md` and `README.md` pointing devs at the hook page.
- `CHANGELOG.md` entry.

## Capabilities

### New Capabilities
<!-- none — this is a docs-only change -->

### Modified Capabilities
<!-- none — no requirements change, no behaviour change in any Billbird capability -->

## Impact

- **Code (Go)**: none. Doc-only.
- **Schema**: none.
- **REST API**: unchanged.
- **Devs**: those who opt in apply the snippet and get an auto-suggested `Time:` footer on each commit. Those who don't continue unchanged.
- **Future**: a `gitsweeper reconcile` MCP tool / CLI subcommand will compare commit footers to `/log` entries and report drift. Out of scope for this change.
