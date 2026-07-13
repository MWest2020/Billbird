# Habitat builder — apply-docs-contract

_run_id builder-apply-docs-contract-20260713-072830 · verdict ok · 2026-07-13T07:35:55Z_

---

Committed to the branch as `5178d26`. Task 4.1 (open PR) needs `gh`/push, which isn't available here and which task 1.1 assigns to the habitat-harness — so I've left 4.1 unchecked and prepared the PR body below for the harness/Mark. Here's my run report.

## Run report — `apply-docs-contract`

**Branch:** `habitat/builder/apply-docs-contract` · **Commit:** `5178d26`

### Done (tasks.md 1.1 → 3.1 ✅)
- **Structure & migration (2.1):** moved all 13 docs into the contract layout via `git mv` (history preserved):
  - `how-to/` — setup, self-hosting, dev-time-hook, operations
  - `reference/` — api-tokens, commands, configuration, labels, webhook-idempotency, client-attribution, corrections
  - `explanation/` — architecture
  - `contributing.md` stays in docs root (contract allows this)
  - **Redirect stubs** left at every old path (`status: deprecated`) — README and external links point at the old paths, so these keep them working.
  - Cross-directory internal links updated to resolve to new locations; a link-resolution check passes with zero broken links.
- **Front matter (2.2):** every page starts with YAML `status` + `last_reviewed`, no `owner`. Migrated-without-review pages → `status: draft`, `last_reviewed: 2026-07-13`. Stubs → `deprecated`.
- **index.md (2.3):** one-paragraph intro, status note, link to README, links to each present section.
- **.mcp.json (2.4):** already present in root with the seed template and the `TODO-change-3` placeholder — unchanged, as instructed.
- **Self-check (3.1):** only permitted files carry markdown (index + contributing + stubs in root; content in the three submaps); every page has front matter; single language (English); no empty dirs.

### Not done / flagged for you (Mark)
- **Task 4.1 — open PR:** `gh` is not installed and task 1.1 says the harness owns branches/push, so I committed but did not open the PR. Ready-to-use:
  - **Title:** `docs: apply handbook docs contract`
  - **Body checklist:**
    - [x] `docs/` follows the contract (how-to / reference / explanation; index + reference minimum met)
    - [x] Front matter on every page (`status` + `last_reviewed`, no `owner`); migrated-without-review = `draft`
    - [x] Existing docs migrated; redirect stubs left at old paths for external/README links
    - [x] One language (English)
    - [x] README preserved; `docs/index.md` links to it, does not replace it
    - [x] `.mcp.json` present with `TODO-change-3` placeholder
    - [ ] **LICENSE (EUPL-1.2) — NOT added, needs your decision (see below)**

- **LICENSE (EUPL-1.2):** The proposal's "Repo-specifiek" section asks to add EUPL-1.2, citing "standaardbeleid project.md" — but **there is no `project.md` in this repo**, and it is **not a checkbox in `tasks.md`**. I could not fetch the canonical EUPL-1.2 text (no outbound network for Bash `curl`/`wget`/`python`, and WebFetch failed), and I won't hand-type a legal document from memory — an inaccurate license is worse than none. Per the builder rule "don't improvise; stop and report," I left this for you: add the canonical `LICENSE` at merge, or confirm it belongs to a separate change.

I've stopped here as instructed — no merge.
