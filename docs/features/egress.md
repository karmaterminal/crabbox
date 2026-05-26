# Mediated Egress

Read when:

- browser or app QA needs a lease to use the same public internet path as an
  operator workstation;
- adding the `crabbox egress` command family;
- comparing mediated browser/app egress with Tailscale exit nodes, Cloudflare
  Tunnel, or full-VM routing;
- wiring Mantis-style visual QA for Discord, Slack, or other web apps that
  are sensitive to source IP, browser login, or regional routing.

Status: implemented as a CLI-first bridge. The shipped slice supports
`egress start`, `host`, `client`, `status`, and browser launches with
`desktop launch --egress`.

## Goal

Some QA scenarios need the runner to look like it is browsing from the same
network as the human or agent driving the test. Discord and Slack are good
examples: login, bot verification, abuse heuristics, and regional behavior can
change when the browser comes from a fresh cloud IP.

The first Crabbox egress goal is:

```text
Chrome or an app inside a Crabbox lease
  uses a local proxy inside the lease
  and exits to the internet from the operator machine running Crabbox.
```

This is intentionally per-app/per-process egress. It should make browser QA and
Mantis scenarios reproducible without changing every route on the VM. Full
machine routing can be added later through a Linux exit node or a dedicated
gateway when a scenario truly needs all traffic to move.

## Non-Goals

Mediated egress is not:

- a public open proxy;
- a replacement for provider firewalls or SSH access controls;
- a transparent VM-wide VPN in the first implementation;
- a way for the Cloudflare Worker itself to become the internet egress point;
- a place to store browser login state, app credentials, or provider secrets.

The Cloudflare Worker is the mediator. The operator machine is the egress point.

## Existing Pieces

Crabbox already has two bridge models that are close to the desired shape:

- WebVNC: `crabbox webvnc` keeps an SSH tunnel to the lease VNC service and
  connects a local bridge process to the coordinator with a one-use ticket. The
  browser portal then talks to that bridge through the Worker Durable Object.
- Code portal: `crabbox code` starts a code-server process on the lease and
  proxies HTTP/WebSocket traffic through a ticketed coordinator bridge.

Those bridges establish the important boundaries:

- the Worker owns authenticated routing, tickets, status, and cleanup;
- bridge agents connect outbound to the Worker;
- each bridge is tied to one lease and short-lived ticket material;
- the portal is not allowed to reach private runner services by itself.

Mediated egress should reuse that model instead of introducing an unrelated
proxy service.

## Architecture

Mediated egress has two long-running agents and one Worker Durable Object
session.

```text
                    Cloudflare Worker / Fleet Durable Object
                   +----------------------------------------+
                   | ticket auth, socket pairing, status,   |
                   | allowlist metadata, cleanup, counters  |
                   +-------------------+--------------------+
                                       |
                    paired WebSocket streams over HTTPS
                                       |
        +------------------------------+------------------------------+
        |                                                             |
+-------v-----------------+                             +-------------v------+
| lease egress client     |                             | host egress agent  |
| runs inside the lease   |                             | runs on operator   |
| listens on 127.0.0.1    |                             | machine / gateway  |
+-----------+-------------+                             +-------------+------+
            |                                                           |
            | HTTP CONNECT / proxy                                      | TCP
            |                                                           |
      +-----v------+                                             +------v-----+
      | Chrome /   |                                             | internet   |
      | Slack app  |                                             | from host  |
      +------------+                                             +------------+
```

The lease side exposes a loopback proxy such as `127.0.0.1:3128`. Chrome or a
desktop app is launched with:

```sh
--proxy-server=http://127.0.0.1:3128
```

The host side opens the real outbound TCP connections. Remote services see the
operator machine's internet path, not the cloud provider's default egress IP.

## Setup And Traffic Flow

```text
Operator CLI
    |
    |  crabbox egress start --id blue-lobster --profile discord --daemon
    v
Resolve lease through coordinator
    |
    +-- if local coordinator is Access-protected:
    |       use --coordinator https://broker.example.com
    |       so the lease can connect without private Access credentials
    |
    v
Create shared egress session
    |
    +--> create client ticket
    |       |
    |       v
    |   SSH to lease
    |       |
    |       v
    |   install/run crabbox egress client
    |       |
    |       v
    |   listen on 127.0.0.1:3128 inside lease
    |
    +--> create host ticket
            |
            v
        run local crabbox egress host
            |
            v
        connect outbound to coordinator

Runtime browser request
    |
    |  Chrome --proxy-server=http://127.0.0.1:3128
    v
Lease-local proxy
    |
    |  HTTP CONNECT host:443
    v
Cloudflare Worker / Fleet Durable Object
    |
    |  pair lease client + host agent by leaseID/sessionID
    v
Host egress agent on operator machine
    |
    |  enforce allowlist, open TCP connection
    v
Internet service sees operator public IP
```

Teardown runs in the opposite direction: `crabbox egress stop` stops the local
host daemon and asks the lease to kill the remote client; releasing a lease also
clears coordinator-side egress sockets and session status.

## Command Shape

The CLI is explicit enough for debugging but ergonomic for the common
desktop-browser case.

Low-level commands:

```sh
crabbox egress host --id blue-lobster --profile discord
crabbox egress client --id blue-lobster --listen 127.0.0.1:3128
crabbox egress status --id blue-lobster
crabbox egress stop --id blue-lobster
```

Operator-friendly orchestration:

```sh
crabbox egress start --id blue-lobster --profile discord --daemon
crabbox desktop launch --id blue-lobster \
  --browser \
  --url https://discord.com/login \
  --egress discord \
  --webvnc \
  --open
```

`egress start`:

1. resolve the lease;
2. create a host ticket and start the host bridge locally;
3. create a client ticket and start the lease-side proxy over SSH;
4. write the active proxy endpoint into lease-local state;
5. print status and cleanup commands.

Today the orchestrated `egress start` path is Linux-only because it installs a
Linux helper and starts it with POSIX shell commands. Non-Linux targets should
use manual target-specific setup until Crabbox grows native helper install
commands for those operating systems. If your coordinator needs Cloudflare
Access credentials, use a public coordinator route for `egress start`, or run
the low-level pieces manually with an explicit secret-handling plan.

`desktop launch --egress <profile>` passes the configured lease-local proxy to
the browser command. Start `egress start` first so something is listening on the
lease proxy port.

## Worker API

The coordinator exposes ticketed routes next to the WebVNC and code bridge
routes:

```text
POST /v1/leases/{leaseID}/egress/ticket
GET  /v1/leases/{leaseID}/egress/host?ticket=...
GET  /v1/leases/{leaseID}/egress/client?ticket=...
GET  /v1/leases/{leaseID}/egress/status
```

The ticket request should include:

```json
{
  "role": "host",
  "profile": "discord",
  "allow": ["discord.com", "*.discord.com"],
  "sessionID": "egress_..."
}
```

The Worker tracks enough state to answer status and clean up stale bridges:

```text
leaseID
sessionID
owner/org
profile
allowlist
hostConnected
clientConnected
activeConnections
bytesIn
bytesOut
lastHostnames
createdAt
lastSeenAt
expiresAt
```

Like WebVNC/code, agent WebSocket upgrades should be accepted only after a
one-use ticket is consumed by the Fleet Durable Object. Cloudflare Access
service-token headers may get the request through the edge, but Crabbox ticket
auth still owns the bridge authorization.

## Stream Protocol

WebVNC can forward one raw byte stream. Egress needs many concurrent TCP
connections because a browser opens several sockets at once.

The bridge protocol needs multiplexed streams:

```text
hello      { role, sessionID, protocolVersion }
open       { connId, host, port }
open_ok    { connId }
data       { connId, bytes }
close      { connId }
error      { connId, message }
stats      { activeConnections, bytesIn, bytesOut }
```

The lease egress client parses HTTP proxy requests from Chrome. For `CONNECT
host:port`, it asks the host agent to open a TCP connection. For plain HTTP
absolute-form requests, it can either proxy them directly or translate them to
a stream to port 80.

The first implementation may use JSON control frames and base64 data chunks for
simplicity. The protocol should reserve a version field so a later binary frame
format can avoid base64 overhead without changing the CLI surface.

## Security Model

Mediated egress must default closed.

Required guardrails:

- no listener bound to a public interface;
- one-use, short-lived tickets bound to lease, owner/org, role, and session;
- explicit domain allowlist or named profile;
- idle timeout and lease TTL cleanup;
- bounded active connections per session;
- bounded per-frame size;
- hostname logging only, not URLs or payload;
- no proxy passwords, tickets, or credentials in logs;
- host agent refuses destinations outside the allowlist;
- session closes when either side disconnects for longer than a short grace
  period.

The host agent is powerful because it opens internet connections from the
operator network. It should show a clear startup summary before connecting:

```text
lease: blue-lobster
profile: discord
allowed: discord.com, *.discord.com, discordcdn.com, *.discordcdn.com
listening: none public; outbound websocket only
```

## Profiles

Profiles keep common browser QA scenarios repeatable without turning egress
into a blanket tunnel.

Intended config shape:

```yaml
egress:
  enabled: false
  listen: 127.0.0.1:3128
  browserProxy: true
  profiles:
    discord:
      allow:
        - discord.com
        - "*.discord.com"
        - discordcdn.com
        - "*.discordcdn.com"
        - hcaptcha.com
        - "*.hcaptcha.com"
    slack:
      allow:
        - slack.com
        - "*.slack.com"
        - slack-edge.com
        - "*.slack-edge.com"
```

Profiles should be merged like other config: flags over env over repo config
over user config over defaults. Repo config can define scenario profiles; user
config can define local preferences such as the default listen address.

## Browser And Desktop Integration

`--browser` leases already install a browser wrapper exposed through `BROWSER`
and `CHROME_BIN`. Egress should integrate at that seam.

Planned behavior:

- `crabbox egress start` launches the lease-local proxy at
  `127.0.0.1:3128` by default;
- `crabbox desktop launch --egress <profile>` passes
  `--proxy-server=http://127.0.0.1:<port>` when launching Chrome/Chromium;
- a later `crabbox run --egress <profile>` may opt command processes into
  `HTTP_PROXY`, `HTTPS_PROXY`, and `ALL_PROXY`, but should never do this by
  default for every run.

This keeps browser QA easy while avoiding surprising build or package-manager
traffic through a workstation.

## Portal Integration

The portal lease detail page shows egress status when a session exists:

- profile and allowlist;
- host/client connected state;
- copyable start/status/stop commands.

The portal should not expose raw proxy URLs or ticket values. It should treat
egress like WebVNC/code: a bridge that exists only while local agents are
running.

Connection counts, byte counters, and recent hostnames are still CLI/API-only
follow-ups once the bridge reports structured runtime stats.

## Comparison With Alternatives

### Tailscale Exit Node

A Tailscale exit node can route the whole VM through another machine. That is
useful when every process must share the same egress path. It is also more
fragile: OS forwarding, NAT, ACLs, and route approval all have to line up.

Use Tailscale exit nodes later for full-machine scenarios. Use mediated egress
first for browser/app QA.

### Cloudflare Tunnel TCP

A named Cloudflare Tunnel plus Access can expose private TCP services without a
public listener. It is useful as an operational building block, but it still
needs host and lease processes plus lifecycle management. Keeping the first
implementation inside the existing Worker/Durable Object bridge gives Crabbox
one auth, status, and cleanup model.

### Cloudflare Worker As Egress

Workers should not be the source of browser internet traffic for this feature.
The goal is not "use Cloudflare's IP"; it is "use the operator machine's
internet". The Worker mediates the two sides.

## Implementation Plan

### Phase 1: CLI-Only Mediated Proxy

Done:

- egress ticket and status routes in the Fleet Durable Object;
- host/client WebSocket bridge attachments;
- multiplexed stream protocol with connection IDs;
- `crabbox egress host`, `client`, `start`, `status`, and `stop`;
- domain allowlist enforcement on the host side;
- tests for ticket use, allowlist rejection, request parsing, and status
  reporting.

### Phase 2: Browser Wiring

- Add `desktop launch --egress <profile>`. Done.
- Add optional browser wrapper support for `CRABBOX_BROWSER_PROXY_SERVER`.
- Add lease-local egress state beyond the active proxy port.
- Add a live smoke that launches a browser through the proxy and proves the
  observed public IP matches the host agent path.

### Phase 3: Portal And Daemon UX

Done:

- portal egress status on the lease detail page;
- daemon supervisor behavior matching WebVNC;
- duplicate-daemon replacement and cleanup;
- clearer cleanup on lease stop/expiry.

Remaining:

- Add docs and examples for Discord and Slack QA.

### Phase 4: Full-Machine Options

- Keep mediated per-app egress as the default.
- Add a separate full-route mode only when the target is a suitable Linux
  gateway or a confirmed Tailscale exit node.
- Document full-route mode as higher-risk and provider/OS dependent.

## Verification

Useful proof for the first implementation:

```sh
crabbox warmup --provider hetzner --desktop --browser
crabbox egress start --id blue-lobster --profile discord --daemon
crabbox desktop launch --id blue-lobster \
  --browser \
  --url https://discord.com/login \
  --egress discord \
  --webvnc \
  --open
```

Expected evidence:

- `egress status` reports host and client connected;
- a browser IP check shows the host-side egress IP;
- Discord loads inside the WebVNC desktop;
- the host agent logs only allowed hostnames and byte counters;
- stopping the lease tears down the bridge and local proxy.

## Source Map

Planned implementation files:

- CLI command router: `internal/cli/cli_kong.go`
- egress command implementation: `internal/cli/egress.go`
- coordinator client ticket/status calls: `internal/cli/coordinator.go`
- desktop/browser launch integration: `internal/cli/desktop.go`
- browser wrapper bootstrap: `internal/cli/bootstrap.go`, `worker/src/bootstrap.ts`
- Worker top-level WebSocket routing: `worker/src/index.ts`
- Fleet Durable Object bridge state and routes: `worker/src/fleet.ts`
- Worker request/record types: `worker/src/types.ts`
- portal lease detail status: `worker/src/portal.ts`

Related docs:

- [Interactive desktop and VNC](interactive-desktop-vnc.md)
- [Broker auth and routing](broker-auth-routing.md)
- [Browser portal](portal.md)
- [Tailscale](tailscale.md)
- [Configuration](configuration.md)
