# Discord Bridge Client PTY Harness Prototype

Prototype Go harness that connects to the Discord bridge service, joins as one agent, and runs the actual agent command inside a PTY.

## What it does

- joins the Discord bridge with one agent/channel binding
- launches the configured command inside a PTY
- polls bridge events for the bound agent
- injects bridge-originated prompts into the PTY session
- publishes bridge-owned status reactions
- sends exactly one completion reply per processed event
- persists processed event ids locally

## Run

```bash
cd research/discord-bridge-client
go run .
```

Default config file:

```bash
~/.pi/discord-bridge-client/config.json
```

If that file exists, `discoagent` loads it automatically.

Minimal env without a config file:

```bash
export DISCORD_BRIDGE_AGENT_ID=main
export DISCORD_BRIDGE_CREDS_REF=local-session
```

Useful optional env:

```bash
export DISCORD_BRIDGE_URL=http://127.0.0.1:19444
export DISCORD_BRIDGE_GUILD_ID=1478102509330497721
# optional: omit channel id and let the bridge auto-assign one
export DISCORD_BRIDGE_CHANNEL_ID=1504560627325079642
export DISCORD_BRIDGE_COMMAND=pi
export DISCORD_BRIDGE_COMMAND_ARGS='-c'
export DISCORD_BRIDGE_CWD=/home/easter/omicron
export DISCORD_BRIDGE_CLIENT_STATE_ROOT=~/.pi/discord-bridge-client
export DISCORD_BRIDGE_POLL_INTERVAL_MS=1500
export DISCORD_BRIDGE_IDLE_COMPLETE_MS=2500
```

## Notes

This is a first PTY-based harness prototype. Completion detection is intentionally simple: the harness watches PTY output and treats the turn as complete after a quiet period.

Research docs:
- `SPEC.md`
- `IMPLEMENTATION-PLAN.md`
- `TICKET.md`
