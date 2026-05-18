# Identifiers

Read when:

- changing how Crabbox names leases, slugs, runs, or claims;
- debugging "why does `crabbox run --id` not find this lease?";
- adding a new lookup form (alias, provider id, anything that resolves to a
  lease).

Crabbox names every long-lived thing twice: once with a stable canonical ID
that machines compare, and once with a friendly slug that humans type. This
page lists the identifiers, where they come from, and how lookup resolves
across them.

## Lease ID

Canonical lease IDs look like:

```text
cbx_abcdef123456
```

The pattern is fixed: the literal `cbx_` prefix followed by 12 hex characters.
`isCanonicalLeaseID` enforces it as a regex; anything else is treated as a
slug or alias.

The CLI mints a provisional lease ID before calling the broker. The broker
may return a different final ID (when the Worker dedupes a retried request,
for example); the CLI then moves the local SSH key directory and claim file
from the provisional ID to the final ID with `MoveStoredTestboxKey` and
re-keys references accordingly.

Provider resources reference the lease ID through Crabbox labels:

```text
crabbox-lease=cbx_abcdef123456
```

That label is what `crabbox cleanup` and `crabbox list` use to map a provider
machine back to a Crabbox lease.

## Slug

Slugs are friendly, human-typeable lease names. They look like:

```text
blue-lobster
amber-crab
silver-shrimp
```

By default, slugs are generated from a stable hash of the lease ID, so the same
lease always gets the same generated slug. The vocabulary is small (14
adjectives, 8 nouns) because Crabbox is intentionally a small fleet. Fresh lease
commands can request a custom slug with `--slug <name>`:

```sh
crabbox warmup --slug update-flow-smoke
crabbox run --slug update-flow-smoke -- pnpm test:changed
crabbox checkpoint fork chk_abc123 --slug update-flow-smoke
```

`--slug` is creation-time metadata, not a rename operation. It is accepted only
when Crabbox is creating a new lease; existing leases keep their assigned slug.
When a requested or generated slug collides with an existing active lease,
`slugWithCollisionSuffix` appends a 4-hex suffix keyed by the seed:

```text
blue-lobster-1234
```

The collision path is rare in normal use - a single user's active leases
rarely exceed the 14 × 8 = 112 unique base slugs.

Slugs are normalized everywhere they are accepted. `normalizeLeaseSlug` keeps
only `[a-z0-9-]`, collapses runs of separators, and trims leading/trailing
dashes. `Blue_Lobster` and `BLUE-LOBSTER` resolve to `blue-lobster`. Requested
slugs must still contain at least one letter or digit and are capped at 41
characters after normalization so collision suffixes and provider names stay
portable.

## Provider Name

Each managed lease also gets a per-provider resource name that includes the
slug and a hash of the lease ID, so the provider console shows useful names:

```text
crabbox-blue-lobster-7f8a2c1d
```

That name is what shows up as the EC2 `Name` tag, the Hetzner server name,
and the Daytona sandbox name. It is derived from `leaseProviderName(leaseID,
slug)`; the function falls back to `crabbox-cbx-...` if the slug is empty.

## Run ID

Each `crabbox run` against a coordinator also gets a durable run handle:

```text
run_abcdef123456
```

A run is created before the lease is acquired so events can be appended for
leasing failures, sync failures, and command output even when the run never
reaches command-start. Run IDs are stable across a single invocation;
retrying the same command produces a new run.

`crabbox history`, `crabbox events`, `crabbox attach`, `crabbox logs`, and
`crabbox results` all accept run IDs. Slugs do not resolve to runs - only to
leases.

## Local Claims

Reusable leases get a JSON claim file stored under the user state directory:

```text
$XDG_STATE_HOME/crabbox/claims/cbx_abcdef123456.json
```

When `XDG_STATE_HOME` is not set, claims live next to user config in
`~/Library/Application Support/crabbox/state/claims` on macOS or
`~/.config/crabbox/state/claims` on Linux.

The claim payload looks like:

```json
{
  "leaseID": "cbx_abcdef123456",
  "slug": "blue-lobster",
  "provider": "aws",
  "repoRoot": "/Users/steipete/Projects/openclaw",
  "claimedAt": "2026-05-07T07:42:18Z",
  "lastUsedAt": "2026-05-07T07:55:12Z",
  "idleTimeoutSeconds": 1800
}
```

Claims do three things:

- bind a lease to one repo so wrappers and agents do not silently reuse a
  lease against a different checkout;
- give `crabbox run --id blue-lobster` a slug-to-canonical-ID translation
  without round-tripping the broker;
- power "is this lease still mine?" checks before destructive operations
  (`stop`, `cleanup`, `actions register`).

A conflicting claim (same lease, different repo) refuses commands by default;
`--reclaim` overrides the check and rewrites the claim atomically.

Static SSH leases tag their claims with `provider: ssh` so the resolver knows
the lease bypasses the coordinator. Coordinator-backed claims leave
`provider` blank because the coordinator owns provider tracking.

## SSH Key Storage

Per-lease SSH key directories are keyed by lease ID:

```text
~/.config/crabbox/testboxes/cbx_abcdef123456/id_ed25519
~/.config/crabbox/testboxes/cbx_abcdef123456/id_ed25519.pub
~/.config/crabbox/testboxes/cbx_abcdef123456/known_hosts
```

The provisional → final lease ID move uses `os.Rename` on the directory so
the key, public key, and known_hosts file all migrate atomically. The
provider key name (`crabbox-cbx-abcdef123456`) is what the cloud account
sees.

## Resolving An Identifier

`crabbox <command> --id <value>` accepts:

- a canonical `cbx_...` lease ID;
- a normalized slug (`blue-lobster`, `Blue Lobster`, `BLUE_LOBSTER` all resolve
  to the same lease);
- in coordinator mode, also the slug as known to the broker, regardless of
  case.

Resolution order:

1. Read the local claim store for the literal identifier or any slug match
   in `claims/`.
2. If a matching claim exists, use its `leaseID` as the canonical handle.
3. If no claim is found and a coordinator is configured, ask the coordinator
   to resolve the identifier (slug or canonical ID).
4. For static SSH and direct-provider modes, fall back to the provider's
   `Resolve` implementation (`SSHLeaseBackend.Resolve`).

The first source that returns a hit wins. This is why `--id blue-lobster`
works from any directory once the warmup ran in some other repo - the local
claim translates slug to lease ID before the broker is involved.

## Identifier Lifetime

```text
provisional lease ID  newLeaseID() call → broker returns final ID
final lease ID        broker accepts → stored in claim, key dir, labels
slug                  computed on first lease creation, stable forever
provider name         derived from lease ID + slug
run ID                minted per crabbox run when a coordinator is configured
```

Slugs are not recycled. When a lease ends, the slug stays free for any future
lease that happens to hash to it; the small vocabulary makes that
collision-by-hash possible but rare in practice.

Related docs:

- [Coordinator](coordinator.md)
- [SSH keys](ssh-keys.md)
- [Lifecycle cleanup](lifecycle-cleanup.md)
- [Source map](../source-map.md)
