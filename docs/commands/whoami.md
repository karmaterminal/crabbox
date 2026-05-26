# whoami

`crabbox whoami` verifies broker auth and prints the identity the
coordinator sees.

```sh
crabbox whoami
crabbox whoami --json
```

## Human Output

```text
user=alex@example.com org=openclaw auth=github broker=https://broker.example.com
```

The fields:

- `user` - the resolved owner email.
- `org` - the organization namespace, when set.
- `auth` - the authentication mode the coordinator accepted (`github` for
  signed login tokens, `bearer` for shared automation tokens).
- `broker` - the configured coordinator URL.

## JSON Output

```json
{
  "owner": "alex@example.com",
  "org": "openclaw",
  "auth": "github",
  "broker": "https://broker.example.com",
  "tokenSource": "user-config",
  "accessJwtVerified": false
}
```

JSON output also reports the forwarded auth mode, where the token came
from (`user-config`, `env`, `stdin`), and whether a verified Cloudflare
Access JWT was present.

## Identity Sources

Identity normally comes from the signed GitHub login token. The browser
flow embeds the verified GitHub email and allowed-org membership in a
short-lived signed token; the coordinator extracts owner/org from that
token, not from headers.

Shared bearer-token automation reports owner/org from `X-Crabbox-Owner` and
`X-Crabbox-Org`. The CLI fills those headers from:

- `CRABBOX_OWNER` env (highest precedence);
- `GIT_AUTHOR_EMAIL` or `GIT_COMMITTER_EMAIL` env;
- `git config user.email`;
- `CRABBOX_ORG` env for the org header.

Raw Cloudflare Access identity headers are ignored. Only a verified Access
JWT email (with the JWT validated against the Cloudflare team's public
keys) can become the bearer-token owner.

## Exit Codes

```text
0   identity resolved successfully
2   broker URL or token missing
3   auth failure (token rejected, GitHub org membership missing, etc.)
```

Use `whoami` in CI scripts before any long workflow to fail fast on auth
issues.

Related docs:

- [login](login.md)
- [logout](logout.md)
- [Auth and admin](../features/auth-admin.md)
- [Broker auth and routing](../features/broker-auth-routing.md)
