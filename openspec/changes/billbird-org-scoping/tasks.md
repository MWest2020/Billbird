## 1. Self-hosting documentation

- [x] 1.1 Add a "Per-organisation deployment pattern" section near the top of `docs/self-hosting.md`, covering: separate Postgres database per organisation, separate GitHub App per organisation, separate secret store per organisation, separate backup cadence per organisation
- [x] 1.2 Cross-reference the new section from the Docker Compose and Kubernetes subsections so an operator never starts configuring before reading the topology guidance
- [x] 1.3 Clarify `ALLOWED_ORGS` semantics in the same document: typical value is one organisation; comma-separated multi-org values stay legal for consulting setups that consciously share data

## 2. Architecture documentation

- [x] 2.1 Add a "Deployment topology" subsection to `docs/architecture.md` recording the one-instance-per-organisation decision and its consequences (user-scoped tokens, no tenant column, per-instance backups)
- [x] 2.2 Update the system diagram (if rendered) to show one Billbird-plus-Postgres pair per organisation rather than a single shared instance

## 3. README

- [x] 3.1 Add a one-paragraph "Deployment model" note to `README.md` near "Quick start" that names the per-organisation pattern and links to the new self-hosting section

## 4. CHANGELOG

- [x] 4.1 Add a dated entry summarising the topology decision and noting that multi-tenant SaaS is now an explicit non-goal for v2

## 5. Verification

- [x] 5.1 Read the updated `docs/self-hosting.md`, `docs/architecture.md`, and `README.md` end-to-end as if onboarding a new operator: confirm the topology is unambiguous and consistent across the three documents
- [x] 5.2 Confirm no code or schema changes were made in this change (intentional doc-only scope)
