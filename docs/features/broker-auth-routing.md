# Broker Auth And Routing

Read when:

- changing coordinator authentication;
- changing Cloudflare routes or Access policy;
- debugging bearer-token automation or GitHub browser login.

The broker is exposed through Cloudflare Workers routes:

```text
https://broker.example.com
https://broker-access.example.com
https://crabbox-coordinator.example.workers.dev
fallback.example.com/*
```

## Route Model

`https://broker.example.com` is the normal coordinator route. It is public at
the Cloudflare edge so `crabbox login` can complete a browser-based GitHub
OAuth flow. The Worker still requires Crabbox auth for every non-health route.

`https://broker-access.example.com` is the same Worker behind a Cloudflare
Access application. It exists for automation and proof that Crabbox works when
an operator wants an outer Cloudflare gate in front of the coordinator. Requests
to this route must pass two checks:

1. Cloudflare Access accepts the service-token headers before the request
   reaches the Worker.
2. The Crabbox Worker accepts either the shared operator bearer token, the
   separate admin bearer token for admin routes, or a signed Crabbox user token.

That means the Access service token is not a Crabbox admin token. It only gets
the HTTP request through Cloudflare Access. The Worker still decides what the
caller can do.

The current Access app is `Crabbox Coordinator Service Token` on
`broker-access.example.com`. Its policy is `non_identity` service-token auth,
scoped to the local Crabbox CLI service token rather than any token in the
account.

Normal users run `crabbox login --url <broker-url>` for first login, which opens GitHub and stores a signed Crabbox user token. The coordinator needs a GitHub OAuth app with callback:

```text
https://broker.example.com/v1/auth/github/callback
```

Self-hosted coordinators need their own GitHub OAuth app. The callback URL on
that app must exactly match the public Worker URL plus
`/v1/auth/github/callback`, and the Worker `CRABBOX_PUBLIC_URL` must use that
same public origin.

Worker secrets:

```text
CRABBOX_GITHUB_CLIENT_ID
CRABBOX_GITHUB_CLIENT_SECRET
CRABBOX_GITHUB_ALLOWED_ORG
CRABBOX_GITHUB_ALLOWED_ORGS
CRABBOX_GITHUB_ALLOWED_TEAMS
CRABBOX_SESSION_SECRET
```

GitHub browser login requires active membership in the allowed GitHub org before
the coordinator mints a Crabbox user token. Set `CRABBOX_GITHUB_ALLOWED_ORG` or
comma-separated `CRABBOX_GITHUB_ALLOWED_ORGS`; if unset, the Worker falls back
to `CRABBOX_DEFAULT_ORG`, then rejects login if no allowed org is configured. The OAuth app must request
`read:user user:email read:org`.

Set comma-separated `CRABBOX_GITHUB_ALLOWED_TEAMS` to require membership in at
least one team after org membership passes. Entries are GitHub team slugs. Use
`team-slug` for the selected org or `org/team-slug` when multiple orgs are
allowed.

Trusted automation can still use the shared operator bearer token configured in the CLI and Worker. Shared-token callers are normal automation, not admin callers. The CLI sends:

```text
Authorization: Bearer <token>
X-Crabbox-Owner: <email>
X-Crabbox-Org: <org>
```

If the coordinator route is also protected by Cloudflare Access, the CLI can
send Access credentials before the Worker receives the request. Configure
`CRABBOX_ACCESS_CLIENT_ID` and `CRABBOX_ACCESS_CLIENT_SECRET` for a Cloudflare
Access service token, or `CRABBOX_ACCESS_TOKEN` to forward an already minted
Access JWT as `cf-access-token`. These Access credentials only satisfy
Cloudflare Access; the Worker still requires the Crabbox bearer token or a
signed Crabbox user token. When `CRABBOX_ACCESS_TEAM_DOMAIN` and
`CRABBOX_ACCESS_AUD` are configured, the Worker verifies
`Cf-Access-Jwt-Assertion` against Cloudflare Access certs before using any
Access identity. Raw `cf-access-authenticated-user-email` headers are ignored.

An Access-protected route such as `https://broker-access.example.com` can use a service-token-only (`non_identity`) Access app, so automated clients can prove both layers independently: first Cloudflare Access, then the Worker bearer or signed user token.

Local config shape:

```yaml
broker:
  url: https://broker.example.com
  token: <crabbox-shared-token-or-user-token>
  adminToken: <crabbox-admin-token>
  access:
    clientId: <cloudflare-access-client-id>
    clientSecret: <cloudflare-access-client-secret>
provider: aws
```

Set `CRABBOX_COORDINATOR=https://broker-access.example.com` when you want a
command to use the Access-protected route without changing the default public
broker URL. `crabbox config show` reports the Access credential state as
`access_auth=service-token` without printing secrets.

Useful proof commands:

```sh
curl -i https://broker-access.example.com/v1/health
CRABBOX_COORDINATOR=https://broker-access.example.com bin/crabbox doctor
CRABBOX_COORDINATOR=https://broker-access.example.com bin/crabbox whoami
CRABBOX_LIVE=1 CRABBOX_AUTH_SMOKE_ACCESS=1 CRABBOX_COORDINATOR=https://broker-access.example.com CRABBOX_BIN=bin/crabbox scripts/live-auth-smoke.sh
CRABBOX_LIVE=1 CRABBOX_LIVE_PROVIDERS=aws CRABBOX_COORDINATOR=https://broker-access.example.com CRABBOX_BIN=bin/crabbox scripts/live-smoke.sh
```

The first command should fail at Cloudflare Access without credentials. The auth
smoke should pass when local Access credentials, shared broker auth, and admin
broker auth are configured. The provider smoke additionally proves the same
route can lease, run, and release a real machine.

Owner selection for bearer-token requests:

```text
CRABBOX_OWNER
GIT_AUTHOR_EMAIL
GIT_COMMITTER_EMAIL
git config user.email
```

`CRABBOX_ORG` sets the org header. Raw Cloudflare Access email headers do not
override CLI-provided owner/org headers. If the Worker can verify an Access JWT
and that JWT contains an email, that verified Access email becomes the bearer
request owner. Normal `crabbox login` requests use the signed GitHub token
identity.

GitHub user tokens are signed by the Worker and are not admin tokens. Admin
routes require the separate admin token. The `broker.example.com/*` route is
the canonical CLI and browser-login endpoint. `broker-access.example.com/*` is
the service-token-protected endpoint.
`https://crabbox-coordinator.example.workers.dev` and `fallback.example.com/*`
are fallbacks.

Related docs:

- [Coordinator](coordinator.md)
- [Security](../security.md)
- [Infrastructure](../infrastructure.md)
- [config command](../commands/config.md)
