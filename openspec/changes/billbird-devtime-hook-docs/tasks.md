## 1. Documentation

- [x] 1.1 Write `docs/dev-time-hook.md` covering: purpose, A/B reconciliation context, JSON config snippet for `~/.claude/settings.json`, shell script, dev workflow (confirm/edit before push), security/scope notes
- [x] 1.2 Reference the page from `docs/commands.md` (one-liner near the `/log` section) and from `README.md` (one bullet in the Documentation list)
- [x] 1.3 Add a dated `CHANGELOG.md` entry summarising the new docs and noting the future Gitsweeper reconcile tool

## 2. Verification

- [x] 2.1 Read `docs/dev-time-hook.md` end-to-end as if onboarding a dev — confirm the snippet is correct and the steps are unambiguous
- [x] 2.2 Confirm the JSON snippet uses Claude Code's documented hook schema (`~/.claude/settings.json` → `hooks.PreToolUse` or similar)
- [x] 2.3 Confirm the bash script has no hidden assumptions (works on macOS + Linux, requires only POSIX + `claude` CLI)

## Notes

- No openspec spec deltas needed: this change introduces no requirements and modifies no capability. The proposal is the contract.
- Live verification on a real dev machine is intentionally deferred — that's an operator step.
