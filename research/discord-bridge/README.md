# Discord Bridge Prototype

Prototype Go service for a Discord-first bridge and operator control plane.

It sits between Discord and Pi-backed local agents, owns channel bindings and reactions, and provides a local web UI plus HTTP API for bridge-managed agent operation.

## Current status

This prototype currently supports:

- one bridge process connected to one Discord bot
- bridge-owned ack / progress / completion reactions
- local chat + attachment logging
- one active agent binding per channel
- bridge-managed Pi-backed agent clients via `agent-rpc --bridge`
- a server-rendered Go web UI with HTMX partial refreshes
- optional auto-start of one Pi bridge client per enabled Discord room
- persistent local runtime state under `~/.pi/discord-bridge`
- user-level `systemd` supervision for the bridge itself

## Architecture

### Bridge

The bridge process owns:

- Discord connectivity
- inbound message normalization
- outbound delivery
- reaction updates
- channel binding state
- managed-agent records
- local logs / audit trail
- the operator web UI

### Agent clients

Each managed room agent is a child process that runs:

```bash
go run ./cmd/agent-rpc --bridge
```

from the `research/agent-rpc` Go module, or an equivalent `agent-rpc --bridge` binary invocation.

The bridge remains the control plane. Pi RPC remains the agent-side upstream path.

### How attachments are presented to `agent-rpc`

When a Discord message includes attachments, the bridge saves them under:

```bash
~/.pi/discord-bridge/downloads/
```

and includes them in the bridge event delivered to `agent-rpc`.

For Pi-backed agents, the bridge currently formats inbound work into a text block like:

```text
[discord-bridge]
Author: serenitynow67676
Channel: 1504560627325079642
Timestamp: 2026-06-06T13:33:23Z
Attachments:
- image.png (/home/easter/.pi/discord-bridge/downloads/1504560627325079642/1512811645787963592_image.png)
Message:
```

This means the agent learns about attachments from the `Attachments:` section and can inspect the saved local file path directly.

## Runtime layout

Local runtime state lives under:

```bash
~/.pi/discord-bridge/
```

Important paths:

- `~/.pi/discord-bridge/bridge.env`
  - local env file used by the bridge service
- `~/.pi/discord-bridge/state.json`
  - persisted bridge state
- `~/.pi/discord-bridge/audit.jsonl`
  - append-only audit log
- `~/.pi/discord-bridge/logs/`
  - bridge-owned chat logs
- `~/.pi/discord-bridge/downloads/`
  - saved attachments
- `~/.pi/discord-bridge/launched-agents/`
  - per-agent runtime dirs, logs, and Pi session material

## Running the bridge directly

For ad hoc local runs:

```bash
cd research/discord-bridge
go run .
```

Required env:

```bash
export DISCORD_BOT_TOKEN=...
```

Useful optional env:

```bash
export DISCORD_BRIDGE_PORT=19444
export DISCORD_BRIDGE_HOST=127.0.0.1
export DISCORD_BRIDGE_STORAGE_ROOT=~/.pi/discord-bridge
export DISCORD_BRIDGE_ID=discord-bridge-main
export DISCORD_BRIDGE_DRY_RUN=false
export DISCORD_BRIDGE_DEFAULT_GUILD_ID=1478102509330497721
export DISCORD_BRIDGE_ASSIGNABLE_CHANNEL_IDS=1504560627325079642,1488999734944202802
export DISCORD_BRIDGE_AUTOSTART_ENABLED_CHANNELS=true
export DISCORD_BRIDGE_AUTOSTART_AGENT_PREFIX=room
```

## Running the bridge as a user service

The bridge is now set up to run under `systemd --user`.

Unit file:

```bash
~/.config/systemd/user/discord-bridge.service
```

Local env file:

```bash
~/.pi/discord-bridge/bridge.env
```

Useful commands:

```bash
systemctl --user status discord-bridge
systemctl --user restart discord-bridge
journalctl --user -u discord-bridge -n 100 --no-pager
```

Because `linger` is enabled, this user service can stay up across logout.

## Web UI

The bridge UI is a conventional server-rendered Go web app with HTMX partial refreshes.

Primary views:

- Overview
- Managed Agents
- Bindings
- Activity
- Managed Agent Detail

The UI is intended to show:

- bridge health
- Discord connection state
- managed-agent lifecycle state
- channel ownership/bindings
- queue/work state
- recent operational activity

Debug logs are secondary detail, not the homepage.

### Opening the UI

If the bridge is bound locally only:

```bash
http://127.0.0.1:19444/
```

If the bridge is bound for remote access, use your host/VPN/Tailscale IP:

```bash
http://<host-or-vpn-ip>:19444/
```

The current local deployment is configured to bind on all interfaces via:

```bash
DISCORD_BRIDGE_HOST=0.0.0.0
```

## Auto-start one agent per enabled room

If enabled:

```bash
DISCORD_BRIDGE_AUTOSTART_ENABLED_CHANNELS=true
```

the bridge will:

1. read the enabled assignable Discord rooms
2. create one managed-agent record per room
3. synthesize agent ids like:

```text
room-<channelId>
```

4. auto-launch a Pi-backed bridge client for each room

The prefix is configurable via:

```bash
DISCORD_BRIDGE_AUTOSTART_AGENT_PREFIX=room
```

This is useful when you want the bridge to automatically stand up one always-on Pi agent per enabled Discord chat room.

## Manual agent launching

The UI also supports manually creating or editing managed agents and starting/stopping/restarting them.

By default, the bridge launches Pi-backed clients using:

```bash
go run ./cmd/agent-rpc --bridge
```

from the `research/agent-rpc` module directory when available, and otherwise falls back to an `agent-rpc --bridge` executable if present in `PATH`.

## Local HTTP API

Bridge/control endpoints currently include:

- `GET /status`
- `POST /join`
- `GET /agents/{agentId}/events`
- `POST /agents/{agentId}/status`
- `POST /agents/{agentId}/complete`

UI endpoints currently include:

- `GET /`
- `GET /managed-agents`
- `GET /managed-agents/{agentId}`
- `GET /bindings`
- `GET /activity`

HTMX partial routes currently include:

- `GET /partials/overview`
- `GET /partials/managed-agents-table`
- `GET /partials/bindings-table`
- `GET /partials/activity-feed`

Managed-agent actions currently include:

- `POST /managed-agents`
- `POST /managed-agents/{agentId}/start`
- `POST /managed-agents/{agentId}/stop`
- `POST /managed-agents/{agentId}/restart`

## Notes on secrets

Do not commit Discord bot credentials into the repo.

Use a local env file such as:

```bash
~/.pi/discord-bridge/bridge.env
```

or another local secret source.

A typical local `bridge.env` may include:

```bash
PATH=/home/easter/.pi/agent/bin:/home/easter/.local/bin:/home/easter/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
DISCORD_BRIDGE_HOST=0.0.0.0
DISCORD_BRIDGE_AUTOSTART_ENABLED_CHANNELS=true
DISCORD_BRIDGE_AUTOSTART_AGENT_PREFIX=room
DISCORD_BOT_TOKEN=...
```

## Known prototype limitations

This is still a research/prototype project.

Notable current limitations:

- managed-agent lifecycle is improved but not yet a full supervisor/reconciler
- room autostart is bridge-driven but still prototype-grade
- state/history migration is minimal
- the UI is much better than the old debug dashboard, but still evolving
- a production-grade reconciliation / restart-policy model is still future work

## Related docs in this directory

Start with:

- `SCOPE.md`
- `SPEC.md`
- `IMPLEMENTATION-PLAN.md`
- `TICKET.md`
- `UI-AGENT-MGMT-TICKET.md`
- `ADMIN-COMMANDS-PROPOSAL.md`
- `MODEL-PICKER-TICKET.md`

The current proposal direction for Discord command handling is: ordinary chat remains normal prompt input, while leading slash commands are intended to enter a bridge-controlled admin command layer first, where the bridge can resolve aliases, run bridge-native commands, and optionally allow permission-gated passthrough to Pi.
