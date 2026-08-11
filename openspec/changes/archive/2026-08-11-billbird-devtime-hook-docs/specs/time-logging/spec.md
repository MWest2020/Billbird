## ADDED Requirements

### Requirement: Alternative time-source via commit-message footer

The project documentation SHALL describe a complementary time-source: a pre-commit hook on the developer's machine that estimates time elapsed since the previous commit, prompts the developer to confirm or edit, and writes a `Time: <duration>` footer into the commit message. The hook SHALL NOT replace `/log` issue comments — both sources coexist so a later A/B reconciliation tool (planned for Gitsweeper) can surface discrepancies.

The hook SHALL be deliverable as documentation only: a JSON snippet for `~/.claude/settings.json` and a short shell script that the developer copies into their local Claude Code configuration. No code in Billbird itself is required, and no Billbird behaviour changes when the hook is or isn't installed.

#### Scenario: Hook produces a Time footer
- **WHEN** a developer with the hook installed runs `git commit` on a repo where work has been done since the last commit
- **THEN** the commit message gains a `Time: 1h30m` footer (or similar duration), suggested by the hook and confirmed by the developer

#### Scenario: Hook is optional
- **WHEN** a developer does not install the hook
- **THEN** their workflow is unchanged; `/log` issue comments remain the only path Billbird sees and reports normally

#### Scenario: Hook does not contact Billbird
- **WHEN** the hook runs
- **THEN** it operates entirely on the dev's local working copy and the `claude` CLI; no Billbird API call is made and no token is required at the developer machine

### Requirement: Documentation cross-references

`docs/commands.md` and `README.md` SHALL each link to the new `docs/dev-time-hook.md` so devs discover the optional second time-source from the same entry points they use to learn `/log`.

#### Scenario: Dev follows the docs from the README
- **WHEN** a developer reads `README.md` looking for how to record time
- **THEN** they see both `/log` (the primary) and a link to the dev-time hook (the optional commit-side complement)
