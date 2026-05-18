## ADDED Requirements

### Requirement: One Billbird instance per organisation
Billbird SHALL be deployed as a single self-hosted instance per organisation. Each instance SHALL have its own Postgres database, its own GitHub App registration, and its own secret store. Two organisations SHALL NOT share a database in any production deployment.

#### Scenario: Two organisations adopt Billbird
- **WHEN** organisations A and B independently adopt Billbird
- **THEN** each runs its own Billbird instance against its own Postgres database, with its own GitHub App credentials and webhook secret

#### Scenario: Consulting team logs across orgs from one instance
- **WHEN** a consulting team belongs to organisations A and B and sets `ALLOWED_ORGS=org-a,org-b` on a single instance
- **THEN** the deployment remains a single-tenant Billbird from organisation A's and B's perspective — the team consciously accepts shared data because they are the same operator

### Requirement: No multi-tenant database partitioning
The Billbird database schema SHALL NOT carry an `organization_id` (or similar tenant) column on `time_entries`, `plan_entries`, `clients`, `label_mappings`, `sessions`, or `api_tokens`. Tenant isolation SHALL be provided at the deployment boundary (one instance + one database per organisation), never inside the schema.

#### Scenario: New table added by a future change
- **WHEN** a future change introduces a new domain table (e.g. `forecasts`)
- **THEN** that table follows the same rule and does not carry a tenant column

#### Scenario: Existing schema audit
- **WHEN** an operator inspects the schema of an active Billbird database
- **THEN** no table contains a tenant-discriminator column, confirming the deployment-boundary isolation invariant

### Requirement: Multi-tenant SaaS hosting out-of-scope
Multi-tenant SaaS hosting (one Billbird instance serving multiple unrelated organisations from one database) SHALL be out of scope for v2 of the project. Any proposal that introduces multi-tenant SaaS SHALL explicitly revise this requirement and the supporting design notes.

#### Scenario: SaaS proposal arrives
- **WHEN** someone proposes a change introducing tenant scoping inside the database
- **THEN** the proposal cites this requirement, motivates revising it, and provides a migration plan for existing single-tenant deployments

### Requirement: Per-organisation deployment documentation
The project documentation SHALL include a "Per-organisation deployment pattern" section in `docs/self-hosting.md` describing: separate database, separate GitHub App, separate secret store, separate backup strategy. The architecture documentation SHALL record the topology decision and its consequences.

#### Scenario: New operator reads the docs
- **WHEN** a new operator opens `docs/self-hosting.md`
- **THEN** the per-organisation deployment pattern is explained before the Docker Compose and Helm sections, so the operator chooses the correct number of instances before configuring any one of them

#### Scenario: Architect reviews the topology
- **WHEN** an architect reads `docs/architecture.md`
- **THEN** the topology decision (one instance per organisation) and its consequences (user-scoped tokens, no tenant column, per-instance backups) are recorded
