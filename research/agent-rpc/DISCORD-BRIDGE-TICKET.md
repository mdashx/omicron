# Coding Agent Prompt: Connect Agent RPC Harness to Discord Bridge

## Context

A working Go prototype of the agent RPC harness now exists in:

- `research/agent-rpc/`

Key docs:

- `research/agent-rpc/RESEARCH.md`
- `research/agent-rpc/SCOPE.md`
- `research/agent-rpc/SPEC.md`
- `research/agent-rpc/IMPLEMENTATION-PLAN.md`
- `research/agent-rpc/TICKET.md`

Key prototype code:

- `research/agent-rpc/cmd/agent-rpc/main.go`
- `research/agent-rpc/harness/config.go`
- `research/agent-rpc/harness/harness.go`
- `research/agent-rpc/harness/jsonl.go`
- `research/agent-rpc/harness/types.go`

There is also prior Discord bridge work and a PTY-based bridge client prototype in:

- `research/discord-bridge/`
- `research/discord-bridge-client/`

This ticket is for the next step: connect the Go agent RPC harness to the existing Discord bridge.

## Goal

Extend the Go `agent-rpc` harness so it can join the existing Discord bridge, poll bridge events, forward inbound Discord-originated work into Pi RPC, and send final assistant replies and status updates back through the bridge.

The important architectural change is:

- upstream remains Pi via `--mode rpc`
- downstream becomes the existing Discord bridge

Do not regress to PTY-first control or PTY-first output.

Pi RPC should remain the authoritative control and event source.

## What to optimize for

- preserve the current Go harness as the control core
- keep Pi RPC as the source of truth
- integrate with the existing bridge protocol rather than inventing a new one unnecessarily
- maintain session continuity per bound bridge agent
- deliver exactly one final completion per inbound bridge event
- keep status/reaction updates deterministic
- avoid reintroducing PTY scraping for normal success
- keep the bridge integration readable and localizable in the codebase

## Scope of this ticket

In scope:

- bridge join
- bridge event polling
- bridge event dedupe/bookkeeping
- mapping bridge inbound work to Pi RPC `prompt`/`steer`
- final completion delivery through bridge HTTP API
- status updates through bridge HTTP API
- preserving one active Pi session per bound bridge agent process

Out of scope:

- generic multi-transport plugin framework
- Claude integration
- Codex integration
- PTY-first execution path
- rich streaming Discord output of token deltas
- visible thinking output to Discord users by default
- rewriting the bridge service protocol unless truly necessary

## Where to build it

Build this as a continuation of the Go prototype in:

- `research/agent-rpc/`

If new bridge-specific package files are needed, keep them under that directory first.

A reasonable shape would be something like:

- `research/agent-rpc/bridge/`
- or additional bridge-oriented files under `research/agent-rpc/harness/`

Keep the current CLI path working if possible, but the bridge path becomes the new focus.

## Where to look first

Read these before changing code:

### Agent RPC prototype

- `research/agent-rpc/harness/harness.go`
- `research/agent-rpc/cmd/agent-rpc/main.go`

### Bridge client prototype and prior art

- `research/discord-bridge-client/README.md`
- `research/discord-bridge-client/http_api.go`
- `research/discord-bridge-client/http_api_test.go`
- `research/discord-bridge-client/pty_agent.go`
- `research/discord-bridge-client/pi_logs.go`

### Bridge service docs

- `research/discord-bridge/SPEC.md`
- `research/discord-bridge/IMPLEMENTATION-PLAN.md`
- `research/discord-bridge/TICKET.md`

## Existing bridge API expectations

The prior bridge work strongly suggests these HTTP behaviors exist or are intended:

- `POST /join`
- `GET /agents/{agentId}/events`
- `POST /agents/{agentId}/status`
- `POST /agents/{agentId}/complete`

Use the existing bridge contract if it already works.

Do not invent a second bridge protocol unless you discover a real incompatibility.

## Suggested implementation shape

This is suggested shape, not a prescription.

1. Add bridge-aware config to the Go harness.
2. Add a bridge join client.
3. Add a bridge event poll loop.
4. Add local processed-event bookkeeping.
5. Convert bridge inbound events into Pi RPC requests.
6. Reuse the existing harness prompt flow for Pi RPC execution.
7. Send status updates back to the bridge.
8. Send exactly one final completion back to the bridge.
9. Preserve one active Pi session per harness instance.
10. Keep CLI mode as a local test/debug path if practical.

## Example code direction

A likely shape is something like:

```go
type BridgeConfig struct {
    BridgeURL string
    AgentID   string
    CredsRef  string
    GuildID   string
    ChannelID string
}

type BridgeClient struct {
    BaseURL string
    AgentID string
}

func (c *BridgeClient) Join(ctx context.Context, cfg BridgeConfig) error
func (c *BridgeClient) PollEvents(ctx context.Context) ([]InboundEvent, error)
func (c *BridgeClient) UpdateStatus(ctx context.Context, messageID string, reaction string) error
func (c *BridgeClient) Complete(ctx context.Context, messageID, content, finalReaction string) error
```

And then in the harness loop:

```go
for {
    events, err := bridge.PollEvents(ctx)
    if err != nil {
        // retry / reconnect behavior
        continue
    }

    for _, evt := range events {
        if alreadyProcessed(evt.ID) {
            continue
        }

        _ = bridge.UpdateStatus(ctx, evt.MessageID, "💭")
        result, err := h.Prompt(ctx, formatBridgeMessage(evt))
        if err != nil {
            _ = bridge.UpdateStatus(ctx, evt.MessageID, "⚠️")
            markProcessed(evt.ID)
            continue
        }

        _ = bridge.Complete(ctx, evt.MessageID, result.Text, "✅")
        markProcessed(evt.ID)
    }
}
```

A better final structure is welcome if it is cleaner.

## Behavioral expectations

### Join

- the harness should join the bridge before processing work
- rejoin should be safe on restart/reconnect

### Inbound bridge work

- map idle inbound work to Pi RPC `prompt`
- if Pi is already streaming, map new inbound work to `steer` in the first implementation

### Status updates

At minimum:

- set an in-progress reaction/status when work starts
- set a success status when work completes
- set an error/failure status when work fails

### Completion delivery

- send exactly one completion per inbound bridge event
- use final assistant text derived from Pi RPC, not PTY scraping
- if no final text is available, send a safe fallback completion message

### Session continuity

- preserve one active Pi session for the harness instance
- reuse that Pi session across multiple bridge events
- log `sessionId` and `sessionFile` when available

## What not to do

Do not:

- switch the main control path back to PTY
- make log-tailing the primary output path
- require Discord-specific behavior inside the Pi prompt
- emit chain-of-thought or internal thinking to Discord by default
- over-generalize into a transport framework before this bridge step works

## Acceptance criteria

- the Go harness can join the existing Discord bridge
- the harness can poll bridge events for one logical agent id
- one inbound bridge event can be forwarded to Pi RPC and produce one final bridge completion
- bridge status updates are emitted during processing
- repeated bridge events reuse the same Pi session within the running harness instance
- the bridge integration does not require PTY scraping or session-log tailing for normal success
- processed event ids are tracked well enough to avoid duplicate completions in normal operation
- the code remains readable enough that an onlooker can identify the bridge boundary and the Pi RPC boundary quickly

## Notes for the coding agent

- keep the current Go harness core intact and extend it
- prefer small bridge-specific additions over large rewrites
- if the cleanest shape is a bridge polling runner plus the existing harness core, that is good
- use the existing bridge API shape unless you find a concrete mismatch
- if you need local persistence for processed event ids, keep it minimal and explicit
