# API tokens

Billbird supports long-lived bearer tokens for the REST API at `/api/v1/*`. Tokens act as the issuing user — they grant the same read/write access that user already has through the admin panel. They are intended for non-browser consumers such as the Gitsweeper Manager-MCP server.

## Lifecycle

1. **Create** a token in the admin panel under **API tokens** → *Create token*. Give it a label like `Manager-MCP` so future-you can tell tokens apart.
2. The plaintext is shown **once** in a highlighted box on the page that the form returns to. Copy it immediately. Billbird stores only a bcrypt hash; the plaintext is unrecoverable after you leave the page.
3. **Use** the token by setting the `Authorization: Bearer bb_...` header on any `/api/v1/*` request.
4. **Revoke** any token (your own from the panel; admins can revoke any user's tokens) when it is no longer needed or may have leaked. Revocation takes effect on the next request.

### Why the plaintext is rendered inline, not redirected to

A prior version did `Redirect("/admin/tokens?new_token=" + plaintext)` after generation. That puts the plaintext in:

- **Reverse-proxy access logs** (nginx, Cloudflare, anything in front of Billbird logs the request line by default — query string included).
- **Browser history** (`https://billbird.example.com/admin/tokens?new_token=bb_…` stays there until the user clears it).
- **Referer header** of the next navigation from that page, leaking the token to whatever external host the user clicks through to.

The current handler generates the token, then renders the tokens page *in-place* with the plaintext in the response body. There is no redirect and no URL with the secret in it. The page is a normal `POST`-response; a browser refresh shows the standard "Confirm form resubmission" prompt — refusing it is the safe default; confirming it generates a second (unused) token, easy to revoke.

## Format

```
bb_<43-character base64-url payload>
```

The literal `bb_` prefix identifies a Billbird token at a glance. The first 8 characters of the payload are stored in the database as the row's prefix for the listing UI; the rest is hashed with bcrypt (cost 12) and never persisted.

## Example

```bash
TOKEN='bb_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'

# Hours summary for May 2026
curl -H "Authorization: Bearer $TOKEN" \
    'https://billbird.example.com/api/v1/time-entries?from=2026-05-01&to=2026-05-31'

# Plan-vs-actual for issue org/repo#42
curl -H "Authorization: Bearer $TOKEN" \
    'https://billbird.example.com/api/v1/issues/org/repo/42/plan-vs-actual'
```

## Authorisation re-check

Every token-authenticated request re-checks that the token's owner is still a member of an `ALLOWED_ORGS` organisation. The decision is cached for up to 5 minutes per user. A user removed from every allowed org loses token-based API access within that window.

Cookie-authenticated requests rely on the existing session: org membership is verified at login and is not re-checked per request.

## What tokens cannot do

- **Create or revoke other tokens.** Token management is only available via the browser session, to limit the blast radius of a leaked token.
- **Use cookie-only routes.** Tokens are only valid on `/api/v1/*`. Routes like `/admin/...` still require a session cookie.
- **Outlive a revocation.** Once revoked, a token fails authentication immediately on the next request.

## Operational notes

- Tokens have **no expiry** by default. Revoke them manually when no longer needed.
- The `last_used_at` column is updated at most once per minute per token, to avoid write amplification under sustained MCP usage.
- Token rows are never physically deleted; revoked rows stay for audit.

## Security model

A leaked token is equivalent to a leaked password for that user, scoped to API read/write. There are no service accounts and no admin-only tokens in v1. Treat token files like credential files: store them outside the repository, do not commit them, and do not paste them into chat.
