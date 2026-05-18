## 1. Database Schema

- [x] 1.1 Write migration `000007_create_plan_entries.up.sql` and `.down.sql`: same shape as `time_entries` minus `client_id`, plus `closing_comment_id` / `closing_comment_url` nullable columns
- [x] 1.2 Add partial unique index `uniq_active_plan ON plan_entries (repository, issue_number) WHERE status = 'active'`
- [x] 1.3 Write migration `000008_create_api_tokens.up.sql` and `.down.sql`: id, user_id, github_username, label, prefix, hash, created_at, last_used_at nullable, revoked, revoked_at nullable
- [ ] 1.4 Verify both migrations apply cleanly on a fresh and a migrated existing database (requires running Postgres)

## 2. Command Parser

- [x] 2.1 Extend `cmdPattern` in `internal/commands` to include `plan` and `unplan`
- [x] 2.2 Define `CmdPlan` and `CmdUnplan` constants and `parsePlanDuration` (reused `parseDuration` — same shape)
- [x] 2.3 Add unit tests for `/plan <duration>`, `/plan <duration> <desc>`, `/unplan`, invalid `/plan`, `/plan` with zero duration

## 3. Plan Entry Store

- [x] 3.1 New package `internal/planentry` with `Create`, `FindActive`, `MarkSuperseded` + `LinkSupersedeChain`, `SoftDelete` mirroring `internal/timeentry`
- [x] 3.2 `GetChain(planID int64)` returns the predecessor/successor chain for a given plan
- [x] 3.3 `ComputePlanVsActual(repository string, issueNumber int)` returns `{planned, logged, variance, status}` from `plan_entries` joined with `time_entries`
- [x] 3.4 Unit tests for status classifier (no_plan / under / on_target / over with 5 percent tolerance)
- [ ] 3.5 Integration test: confirm partial unique index rejects a duplicate active row (requires running Postgres)

## 4. Plan Command Handlers

- [x] 4.1 `/plan` handler: parse, verify org membership (reuse existing helper), create plan entry, optionally supersede previous, post confirmation comment
- [x] 4.2 `/unplan` handler: find active plan, soft-delete with closing comment metadata, post confirmation
- [x] 4.3 Error handling: missing/invalid duration, no active plan for `/unplan`, non-member auth failure — post matching error comments
- [ ] 4.4 End-to-end test: webhook delivery → `/plan` → entry created → confirmation posted (requires running deployment)

## 5. API Tokens — Domain

- [x] 5.1 New package `internal/apitoken` with `Generate` (returns plaintext + hash + prefix), `Verify(plaintext)`, `Revoke(id, byUsername)`
- [x] 5.2 Token format `bb_<32 random bytes base64-url no padding>`; bcrypt cost 12
- [x] 5.3 Last-used-at update throttled to ≥60s between writes per token (in-memory memo)
- [x] 5.4 Unit tests: format / prefix / uniqueness / parsePrefix edge cases
- [ ] 5.5 Integration test: round-trip generate→verify with real Postgres (requires running Postgres)

## 6. API Tokens — Middleware

- [x] 6.1 Extend `internal/auth` middleware to accept either session cookie or `Authorization: Bearer bb_...`
- [x] 6.2 Token path resolves to the same `APIAuthContext` as cookie path
- [x] 6.3 Org-membership re-check via GitHub App's installation tokens with 5-minute in-memory cache keyed by GitHub username
- [ ] 6.4 Integration tests: cookie alone, token alone, both, invalid token, revoked token, non-member token (requires running deployment)

## 7. REST API

- [x] 7.1 `GET /api/v1/plans` with filters (`repository`, `issue`, `status`, `since`, `until`)
- [x] 7.2 `GET /api/v1/plans/{id}` returning the plan, plus `GET /api/v1/plans/{id}/chain`
- [x] 7.3 `GET /api/v1/issues/{owner}/{repo}/{number}/plan-vs-actual`
- [x] 7.4 `POST /api/v1/tokens` (create), `GET /api/v1/tokens` (list own), `DELETE /api/v1/tokens/{id}` (revoke)
- [x] 7.5 Bearer auth middleware applied to all `/api/v1/*` routes (was previously open — change brings the API up to the rest of the app's auth posture)
- [ ] 7.6 Handler tests for each new endpoint covering happy path + 401 + 404 (requires running deployment or pgx mock)

## 8. Admin Panel

- [x] 8.1 Plans page template + handler: per-issue planned vs logged + status badge; uses `ComputePlanVsActual` directly
- [x] 8.2 Plan-history view template + route: full chain with comment links
- [x] 8.3 Token management page templates: list, create form, revoke action; plaintext shown exactly once on creation via redirect query
- [x] 8.4 Wire new routes in `internal/admin`; navigation updated with Plans and API tokens entries

## 9. Documentation

- [x] 9.1 `docs/commands.md`: add `/plan` and `/unplan` sections (mirroring `/log` and `/delete` style)
- [x] 9.2 New `docs/api-tokens.md`: lifecycle, hashing, scope, revocation, example `curl`
- [x] 9.3 `docs/architecture.md`: add `internal/planentry` and `internal/apitoken` to component list and project structure
- [x] 9.4 `README.md`: short blurb mentioning `/plan` and that API tokens enable external consumers
- [x] 9.5 `CHANGELOG.md`: dated entry covering plan command set and token capability

## 10. End-to-End Verification (deferred to deployment)

- [ ] 10.1 Manual run: install GitHub App on test repo, `/plan 4h` → confirmation, `/log 2h` → plan-vs-actual view shows under, `/log 3h` → over, `/unplan` → no_plan
- [ ] 10.2 Manual run: create a token in the admin panel, hit `/api/v1/plans` with `curl -H "Authorization: Bearer ..."`, revoke, confirm 401 on next call
- [ ] 10.3 Manual run: remove user from `ALLOWED_ORGS`, wait 5 minutes, confirm token-based call now 401s

## Notes

- Tasks marked `[ ]` are intentionally deferred: they require a running Postgres or a running GitHub-App deployment that the implementation environment does not have. The implementation paths for those tasks exist; only the verification is pending.
- Existing baseline gap (not addressed in this change): the repository's `cmd/billbird/main.go` referenced by the Dockerfile is absent. Whoever wires `main.go` will need to construct the new stores (`planentry.NewStore`, `apitoken.NewStore`), the membership checker (`auth.NewMembershipChecker`), and the API auth dependencies, and pass them through to the webhook, API, and admin handlers.
