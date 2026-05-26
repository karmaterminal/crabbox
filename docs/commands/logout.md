# logout

`crabbox logout` removes the stored broker token from user config.

```sh
crabbox logout
crabbox logout --json
```

The broker URL and provider stay in place so a later `crabbox login` or
`crabbox login --token-stdin` can reuse the configured URL. Per-lease SSH keys, repo
claims, and history records are unaffected.

After logout:

- `crabbox whoami` exits with auth code 3 (`auth failure`);
- `crabbox run` and `crabbox warmup` against the coordinator fail with the
  same code;
- direct-provider mode keeps working when local provider credentials
  (AWS SDK, `HCLOUD_TOKEN`) are present, because direct mode does not need
  the broker token.

Use logout when:

- a token has leaked or you want to rotate it;
- you are switching the operator identity on a shared workstation;
- you are testing the unauthenticated path.

To clear everything (URL, provider, token, profile defaults), edit the user
config file directly. `crabbox config path` prints the location.

Related docs:

- [login](login.md)
- [whoami](whoami.md)
- [Auth and admin](../features/auth-admin.md)
- [Configuration](../features/configuration.md)
