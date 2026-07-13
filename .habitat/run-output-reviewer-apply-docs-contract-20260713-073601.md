# Habitat reviewer — apply-docs-contract

_run_id reviewer-apply-docs-contract-20260713-073601 · verdict ok · 2026-07-13T07:38:22Z_

---

I've completed all four reviewer checks against the `apply-docs-contract` change. Here is my verdict.

---

# Review verdict: **PASS** ✅

Change: `apply-docs-contract` · Diff reviewed: `main...HEAD` (builder run `builder-apply-docs-contract-20260713-072830`)

## 1. Scope — PASS
- The substantive diff is exactly the docs migration + supporting files the change calls for: 12 pages migrated into `how-to/`, `reference/`, `explanation/`; 12 stubs left behind; new `docs/index.md`.
- `.mcp.json` is present at root with the correct template and the `TODO-change-3` placeholder intact (task 2.4). It lives on `main` already (seeded), so the requirement is satisfied.
- `tasks.md` checkbox updates are the builder's own bookkeeping; `.habitat/*` and `run-report.json` are harness-generated run artifacts — expected, not out-of-scope code changes.

## 2. Contract (Diátaxis-light) — PASS
- Structure correct: `index.md` + `how-to/` + `reference/` + `explanation/` all carry the migrated markdown; no empty dirs.
- **Front matter on every page**: all 26 pages have `status` + `last_reviewed`; **no `owner` field anywhere** (the lone `owner` grep hit is prose in `api-tokens.md`, not front matter).
- Status values valid: migrated-without-review pages = `draft`; old stubs = `deprecated`; all within the `current|draft|deprecated` enum.
- All 12 stubs are genuine pointers and every target resolves to an existing file. `contributing.md` correctly kept at docs-root (proposal permits it).
- `index.md` is a single paragraph, links to the README (not replacing it), and links each section — matches task 2.3.
- One language (English): no Dutch leaked into `docs/`.

## 3. Cage intact — PASS
No changes to `CLAUDE.md`, `.claude/agents/`, or CI config.

## 4. No secrets — PASS
The only key/token-shaped strings in the diff are placeholders/instructional (`...full PEM contents...`, `\n...`, `bb_xxxx…`, env-var names). No real credentials, tokens, or secret-bearing URLs.

---

## Non-blocking notes for Mark
1. **LICENSE not added.** The proposal's "Repo-specifiek" says "Geen LICENSE: voeg EUPL-1.2 toe," but this contradicts the same proposal's "Géén andere wijzigingen," and `tasks.md` has **no** LICENSE task. The builder reasonably omitted it. Not a task-completion failure — flagging the internal contradiction so you can decide whether a follow-up change should add EUPL-1.2.
2. Task **4.1** (open PR, then stop) is correctly still unchecked — that's yours to drive.

**Recommendation: approve / proceed to merge.** No changes required from the builder.
