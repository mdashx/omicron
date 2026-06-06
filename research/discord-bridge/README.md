# Discord Bridge Prototype

Prototype Go service for the Discord bridge described in this directory.

## What it does

- connects to Discord via bot token
- keeps local chat and attachment logs
- lets one agent bind to one Discord channel
- queues inbound events for the bound agent
- adds bridge-owned reactions for ack / progress / final state
- exposes a small local HTTP API for join, poll, status updates, and completion

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
