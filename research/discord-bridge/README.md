# Discord Bridge Prototype

Prototype Go service for the Discord bridge described in this directory.

## What it does

- connects to Discord via bot token
- keeps local chat and attachment logs
- lets one agent bind to one Discord channel
- can auto-assign a channel when the agent joins without one
- queues inbound events for the bound agent
- adds bridge-owned reactions for ack / progress / final state
- exposes a small HTTP dashboard plus local API for join, poll, status updates, completion, and launching new `agent-rpc --bridge` processes

## Run

```bash
cd research/discord-bridge
go run .
```

Required env:

```bash
export DISCORD_BOT_TOKEN=...
```

Optional env:

```bash
export DISCORD_BRIDGE_PORT=19444
export DISCORD_BRIDGE_HOST=127.0.0.1
export DISCORD_BRIDGE_STORAGE_ROOT=~/.pi/discord-bridge
export DISCORD_BRIDGE_ID=discord-bridge-main
export DISCORD_BRIDGE_DRY_RUN=false
# optional explicit auto-assignment list
export DISCORD_BRIDGE_DEFAULT_GUILD_ID=1478102509330497721
export DISCORD_BRIDGE_ASSIGNABLE_CHANNEL_IDS=1504560627325079642,1488999734944202802
# optional: auto-create and auto-start one Pi bridge client per enabled room
export DISCORD_BRIDGE_AUTOSTART_ENABLED_CHANNELS=true
export DISCORD_BRIDGE_AUTOSTART_AGENT_PREFIX=room
```

If the explicit auto-assignment env vars are omitted, the bridge will try to infer assignable Discord channels from `~/.openclaw/openclaw.json`.

If `DISCORD_BRIDGE_AUTOSTART_ENABLED_CHANNELS=true`, the bridge will synthesize one managed agent per enabled assignable Discord channel and auto-launch a Pi-backed bridge client for each room. The generated agent ids default to `<prefix>-<channelId>` where the prefix comes from `DISCORD_BRIDGE_AUTOSTART_AGENT_PREFIX`.

## Dashboard

The dashboard includes a simple **Launch Agent** control that can start a new `agent-rpc --bridge` process and bind it to a Discord channel. By default it launches `go run ./cmd/agent-rpc --bridge` from the `research/agent-rpc` module directory when available, and falls back to `agent-rpc --bridge` otherwise. It shows a dropdown of assignable channels and includes channel names when available. Leave channel blank to let the bridge auto-assign one from its configured assignable channels. The dashboard also includes a harness log preview panel so you can inspect agent-side issues directly from the UI.


Open:

```bash
http://127.0.0.1:19444/
```

To expose it on your VPN/Tailscale, bind the service to a VPN-reachable host/IP, for example:

```bash
export DISCORD_BRIDGE_HOST=0.0.0.0
# or your Tailscale/VPN IP
```

Then open `http://<vpn-host>:19444/` from another machine on the VPN.

## Local API

- `GET /status`
- `POST /join`
- `GET /agents/{agentId}/events`
- `POST /agents/{agentId}/status`
- `POST /agents/{agentId}/complete`

Start with the research docs in this directory:

- `SCOPE.md`
- `SPEC.md`
- `IMPLEMENTATION-PLAN.md`
- `TICKET.md`
- `UI-AGENT-MGMT-TICKET.md`
