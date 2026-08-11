## Why

Billbird v1's design.md listed "Multi-tenancy or SaaS hosting" as a non-goal, but the running code allows `ALLOWED_ORGS` to contain multiple comma-separated organisations sharing one database. Without an explicit deployment-topology decision, two future paths stay equally open: shared multi-tenant SaaS, or one self-hosted instance per organisation. On 2026-05-18 the project committed to the second path. This change formalises that commitment so the next year of code decisions, deployment docs, and onboarding all assume the same shape.

The follow-on consequence matters: every Billbird instance already belongs to one organisation. API tokens (in `billbird-plan-command`), future role models, and any reporting layer can therefore stay user-scoped, not org-scoped. That is a cheaper architecture, and locking it in now prevents the schema from quietly drifting toward a multi-tenant assumption.

## What Changes

- Adopt **one Billbird instance per organisation** as the official deployment pattern. `ALLOWED_ORGS` typically contains one organisation; multi-org values stay legal for consulting setups where one team logs into one shared codebase, but a single value is the documented default.
- Explicitly mark **multi-tenant SaaS hosting as out-of-scope** for v2. Any future change toward SaaS requires its own proposal that revisits this decision.
- Update `docs/self-hosting.md` with a "Per-organisation deployment pattern" section: separate Postgres database per organisation, separate GitHub App per organisation, separate secret store per organisation.
- Update `docs/architecture.md` to record the topology decision and its consequences (data isolation, token scope, backup strategy).
- Add a short note to `README.md` so the model is visible without digging into docs.

No code change. No schema change. No new dependencies. No migration.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `deployment`: Add deployment-topology requirements (one instance per organisation, no shared database, multi-tenant SaaS out-of-scope).

## Impact

- **Docs only**: `docs/self-hosting.md`, `docs/architecture.md`, `README.md`, `CHANGELOG.md`.
- **Behaviour**: none. The running binary continues to honour whatever `ALLOWED_ORGS` is set to.
- **Future changes**: any proposal that would introduce shared multi-tenant hosting must reference and revise this change explicitly.
