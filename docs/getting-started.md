# Getting Started

Read when:

- you are new to Crabbox and want a working `run` in 10 minutes;
- you are evaluating Crabbox for a repo and want to see the shape;
- you want a reference for what a typical onboarding looks like.

This is a cookbook, not a reference. It walks through one repo end to end,
from install to `crabbox run -- pnpm test`. For deeper coverage, follow the
links in each step.

## Step 1. Install

```sh
brew install openclaw/tap/crabbox
```

Verify the install:

```sh
crabbox --version
crabbox doctor
```

`crabbox doctor` should print `ok` for `tools` (git, rsync, ssh,
ssh-keygen). It is fine if `auth` and `network` are still missing - we set
those next.

If you do not have Homebrew, GitHub Releases ship signed tarballs for macOS,
Linux, and Windows. Download the matching archive from
<https://github.com/openclaw/crabbox/releases>.

## Step 2. Log In

```sh
crabbox login --url https://broker.example.com
```

`login` opens a browser to the GitHub OAuth flow. The broker exchanges the
OAuth code, verifies your GitHub org membership, and writes a signed token
to your user config. From then on, every `crabbox` command authenticates
automatically.

```sh
crabbox whoami
```

Confirms the resolved owner, org, broker URL, and selected provider.

### Broker Access

Broker access is deployment-specific. Use the coordinator URL and GitHub
org/team allowlist from your team. A completed GitHub OAuth flow can still be
rejected when your account is outside that allowlist.

For a personal or third-party installation, choose one path:

- Use direct-provider mode with your own local cloud credentials when you want a
  quick private test lane and can accept local cleanup/state instead of broker
  usage history and shared spend caps.
- Self-host the Worker broker when you want broker-owned provider credentials,
  active-lease limits, monthly spend caps, `crabbox usage`, cleanup alarms, and
  a shared team endpoint.
- Request access only if the broker operator has a defined onboarding path for
  your org; a team endpoint is not automatically an open community broker.

Direct-provider examples:

```sh
crabbox doctor --provider hetzner
crabbox run --provider hetzner -- pnpm test
```

Self-hosting starts with the Worker/Durable Object deployment, provider secrets,
auth config, and budget limits in
[Infrastructure](infrastructure.md#self-hosted-broker-minimum). GitHub OAuth is
optional only for shared-token automation; browser login needs a GitHub OAuth
app and at least one allowed org/team setting.

If you are running Crabbox in a CI environment that cannot open a browser,
use shared-token auth:

```sh
printf '%s' "$TOKEN" | crabbox login \
  --url https://broker.example.com \
  --provider aws \
  --token-stdin
```

See [Auth and admin](features/auth-admin.md) for the full identity model.

## Step 3. Onboard A Repo

Inside the repo:

```sh
crabbox init
```

`init` writes three files:

```text
.crabbox.yaml                          repo defaults (profile, class, sync, env)
.github/workflows/crabbox.yml          Actions hydration stub (optional)
.agents/skills/crabbox/SKILL.md        agent-facing skill instructions
```

Open `.crabbox.yaml` and fill in:

- `profile`: a name for this lane (e.g. `project-check`);
- `class`: `standard`, `fast`, `large`, or `beast`;
- `sync.exclude`: directories that should not be sent to the runner;
- `env.allow`: env vars the remote command should see.

Then run:

```sh
crabbox sync-plan
```

`sync-plan` previews what would be sent: file count, total bytes, the
biggest files. If it shows surprises (a `dist/` folder, a `.cache/` you
forgot, a 2 GiB asset), tighten `sync.exclude` and re-run. The first sync
to a fresh runner is bound by this size.

## Step 4. Warm A Box

```sh
crabbox warmup
```

Warmup acquires a lease through the broker, provisions the runner,
bootstraps SSH and tooling, and prints a slug + lease ID:

```text
leased cbx_abcdef123456 slug=blue-lobster provider=aws server=i-0123 type=c7a.48xlarge ip=203.0.113.10 idle_timeout=30m0s expires=2026-05-07T17:30:00Z
```

The lease is now waiting for commands. Idle timeout (default 30m) and TTL
(default 90m) bound how long it lives before the broker reclaims it.

## Step 5. Run A Command

```sh
crabbox run --id blue-lobster -- pnpm test
```

What happens:

1. The CLI verifies SSH readiness on the lease.
2. It seeds remote Git from your origin/base ref, then rsyncs the dirty
   working tree.
3. It runs the command over SSH, streaming stdout/stderr.
4. It heartbeats the broker so the lease does not idle out mid-test.
5. It records a `run_...` history entry with sync time, command time, exit
   code, and (for Linux) bounded telemetry samples.

You can omit `--id` for a one-shot run:

```sh
crabbox run -- pnpm test
```

That acquires a fresh lease, runs the command, and releases the lease when
the command exits. Use this for ad-hoc tests; use `warmup` + `--id` for
iterative work.

## Step 6. Inspect History

```sh
crabbox history
crabbox events run_abcdef123456
crabbox logs run_abcdef123456
crabbox results run_abcdef123456
```

`history` lists recent runs for the lease or owner. `events` prints ordered
events (lease, sync, command, output chunks, finish). `logs` returns the
retained command output. `results` parses any JUnit reports the run
attached.

`/portal/runs/run_abcdef123456` renders the same data as a browser page if
you prefer a UI.

## Step 7. Stop The Lease

When you are done:

```sh
crabbox stop blue-lobster
```

Stop releases the lease, deletes the provider machine, removes the local
claim, and frees reserved cost. If you forget, the broker idle alarm
releases the lease automatically.

```sh
crabbox cleanup --dry-run
```

`cleanup` is a sweep for direct-provider leftovers. It refuses to run when
a coordinator is configured because brokered cleanup is the alarm's job.

## Common Variations

Use a kept lease across days:

```sh
crabbox warmup --idle-timeout 4h --ttl 8h
crabbox run --id blue-lobster -- pnpm test
crabbox run --id blue-lobster -- pnpm bench
crabbox stop blue-lobster
```

Open a desktop session:

```sh
crabbox warmup --desktop
crabbox vnc --id blue-lobster --open
```

Open a code-server tab:

```sh
crabbox warmup --code
crabbox code --id blue-lobster --open
```

Use a Mac Studio you already own:

```yaml
# .crabbox.yaml
provider: ssh
target: macos
static:
  host: mac-studio.local
  user: steipete
  port: "22"
  workRoot: /Users/steipete/crabbox
```

```sh
crabbox run -- xcodebuild test
```

Use AWS instead of the configured default:

```sh
crabbox run --provider aws --class beast -- pnpm test
```

## Where To Go Next

- [How Crabbox Works](how-it-works.md) - the mental model.
- [CLI](cli.md) - the full command surface and exit codes.
- [Commands](commands/README.md) - one page per command.
- [Features](features/README.md) - one page per feature.
- [Configuration](features/configuration.md) - YAML schema and precedence.
- [Providers](features/providers.md) - which provider to pick.
- [Provider authoring](features/provider-authoring.md) - add a new provider.
- [Troubleshooting](troubleshooting.md) - what to do when a step fails.
