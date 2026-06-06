# Coding Agent Prompt: Discord Bridge Client PTY Harness

## Context

Implement the Discord bridge client harness described in:

- `research/discord-bridge-client/SPEC.md`
- `research/discord-bridge-client/IMPLEMENTATION-PLAN.md`
- `research/discord-bridge-client/README.md`

This harness should connect to the Discord bridge service, join as one logical agent, launch the real agent process inside a PTY, and mediate all bridge I/O on the agent's behalf.

## Goal

Add a startup-loaded Discord bridge client harness so an agent can appear to run in a normal terminal session while actually being supervised by a bridge-aware PTY wrapper.

The harness must own bridge join, event polling, prompt injection, status updates, and final completion delivery.

## What to optimize for

- correct startup wiring
- clear bridge/harness/agent boundaries
- fail-closed enforcement when harness wiring is invalid
- no LLM involvement in deciding whether bridge transport happens
- PTY-first behavior that preserves normal terminal assumptions
- durable local state for restart-safe operation

## Where to build it

Build the harness as a self-contained project in `research/discord-bridge-client/` alongside this `TICKET.md`.

That directory should be the working home for design notes, implementation scaffolding, and harness-specific code while prototyping. Keep the bridge client source there first and wire it into the main repo afterward if needed.

## Language suggestion

Consider writing the harness in **Go**.

Why Go fits well here:
- good fit for a long-lived local supervisor process
- straightforward PTY/process management libraries
- strong fit for polling loops and deterministic control flow
- easy static binary deployment
- good ergonomics for a small runtime wrapper around an existing CLI agent

## Where to look first in the codebase

Start with these files and concepts:

- `research/discord-bridge/SPEC.md`
  - server-side bridge contract

- `research/discord-bridge/README.md`
  - current bridge API and runtime shape

- `research/discord-bridge/http_api.go`
  - join, status, completion, and queue endpoints

- `research/discord-bridge/main.go`
  - bridge runtime model and config shape

- `packages/coding-agent/src/main.ts`
  - Pi CLI/bootstrap entry point
  - useful for understanding how Pi behaves as a terminal app

- `packages/coding-agent/src/core/agent-session-runtime.ts`
  - useful for understanding runtime/session replacement behavior if native embedding becomes attractive later

- `packages/agent/docs/durable-harness.md`
  - runtime/harness design notes

- `packages/agent/docs/hooks.md`
  - event flow semantics

- `research/discord-transport-layer/README.md`
  - prior transport ideas that may inform event normalization

## Suggested implementation shape

This is a suggested shape, not a prescription.

1. Resolve built-in harness defaults.
2. Merge user overrides onto the defaults.
3. Validate the resolved config.
4. Create the local harness state directory if missing.
5. Join the bridge with agent/channel identity.
6. Launch the real agent process inside a PTY.
7. Poll bridge events for the bound agent id.
8. Convert each bridge event into stable agent-facing input.
9. Inject that input into the PTY-backed agent session.
10. Publish bridge status reactions during work.
11. Send final completion replies back through the bridge.
12. Persist processed event ids to prevent duplicate completions.
13. Fail closed if bridge join or PTY wiring is missing.

## Example code direction

The concrete implementation will likely need something like this shape:

```go
cfg := LoadHarnessConfig()
client := NewBridgeClient(cfg.BridgeURL, cfg.AgentID)
if err := client.Join(cfg.CredsRef, cfg.GuildID, cfg.ChannelID); err != nil {
    log.Fatal(err)
}
ptyProc := StartPTY(cfg.Command, cfg.Args, cfg.Cwd, cfg.Cols, cfg.Rows)
for {
    events := client.PollEvents()
    for _, evt := range events {
        client.SetStatus(evt.MessageID, "💭")
        ptyProc.Inject(RenderBridgePrompt(evt))
        reply := ptyProc.WaitForTurnResult()
        client.Complete(evt.MessageID, reply.Text, reply.FinalReaction)
    }
}
```

A good implementation should still make its own judgment about the final API shape. This snippet is just orientation.

## Acceptance criteria

- The harness starts with defaults and no special config beyond bridge identity.
- The local harness state directory is created automatically if it does not exist.
- The harness can successfully join the bridge before processing work.
- The real agent process runs inside a PTY.
- Bridge events are polled and normalized into stable agent-facing input.
- The harness injects work into the PTY-backed agent session.
- Status reactions are published back through the bridge while work is running.
- Final replies are sent back through the bridge exactly once.
- Processed event ids are persisted to prevent duplicates.
- The implementation is enforced by runtime code, not by prompt text.
- The implementation stays readable enough that an onlooker can understand where the harness lives and why.

## Notes for the coding agent

- Read the bridge docs and code before changing anything.
- Prefer the smallest coherent PTY-based implementation that proves the shape.
- Preserve room for later improvements in PTY parsing, restart recovery, and richer prompt shaping.
- Do not overfit the implementation to this prompt; use judgment.
