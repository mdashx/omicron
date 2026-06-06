# Discord Bridge Prototype

Prototype Go service for the Discord bridge described in this directory.

## What it does

- connects to Discord via bot token
- keeps local chat and attachment logs
- lets one agent bind to one Discord channel
- queues inbound events for the bound agent
- adds bridge-owned reactions for ack / progress / final state
- exposes a small HTTP dashboard plus local API for join, poll, status updates, and completion

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
```

## Dashboard

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

- `SPEC.md`
- `IMPLEMENTATION-PLAN.md`
- `TICKET.md`
