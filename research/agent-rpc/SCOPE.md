# Agent RPC Scope

## Scope

This document intentionally narrows the project to a single upstream agent interface and a single downstream output target.

In scope:

- upstream: `Pi` via `pi --mode rpc`
- downstream: our existing Discord bridge

Out of scope for the initial implementation:

- Claude integration
- Codex integration
- session-log tailing as a primary path
- PTY parsing as a primary path
- web UI, Slack, or other transports
- generic multi-agent abstraction beyond what is needed for Pi RPC

## Goal

Build a harness component that launches Pi in RPC mode, maintains session continuity, consumes Pi’s structured events, and relays useful status and completions through the Discord bridge.

The resulting system should let Discord act as the user-facing transport while Pi RPC remains the only agent backend.

## Why this scope

Pi RPC is already the strongest available native interface in this repo’s ecosystem.

It gives us:

- structured command/response control
- structured event streaming
- session identity and persistence
- queueing semantics
- tool execution visibility
- thinking visibility when the provider exposes it

By focusing only on Pi RPC and the Discord bridge first, we can validate the harness architecture without getting distracted by multiple upstreams or multiple outputs.

## Primary use case

1. A Discord message arrives through the bridge.
2. The harness converts that message into a Pi RPC `prompt`, `steer`, or `follow_up` request.
3. Pi emits structured events over RPC stdout.
4. The harness normalizes the events into a smaller bridge-facing model.
5. The Discord bridge uses those events to:
   - update status reactions
   - send final replies
   - optionally stream intermediary status in the future

## Architecture

## Components

### 1. `PiRpcProcess`

Responsible for:

- launching `pi --mode rpc`
- managing stdin/stdout JSONL framing
- sending RPC commands
- receiving responses and events
- handling process lifecycle and shutdown

### 2. `PiSessionController`

Responsible for:

- starting or resuming Pi sessions
- tracking:
  - `sessionId`
  - `sessionFile`
  - `sessionName`
  - `cwd`
- mapping Discord inbound work to Pi RPC calls
- deciding when to use:
  - `prompt`
  - `steer`
  - `follow_up`
  - `abort`

### 3. `PiEventMapper`

Responsible for translating Pi RPC events into a smaller internal event model suitable for the Discord bridge.

Suggested mapped events:

- `session_started`
- `session_state`
- `assistant_text_delta`
- `assistant_thinking_delta`
- `assistant_message_complete`
- `tool_call_started`
- `tool_call_finished`
- `queue_update`
- `turn_complete`
- `agent_complete`
- `error`

### 4. `DiscordBridgeAdapter`

Responsible for integrating the mapped Pi events with the existing Discord bridge behavior.

Initial responsibilities:

- set working/in-progress reaction when Pi starts work
- clear/update reaction when Pi completes
- send exactly one completion message back through the bridge for each inbound Discord task
- log enough metadata for debugging session continuity

## Source of truth

For the initial implementation, Pi RPC is the source of truth.

Use Pi session JSONL files only as:

- optional audit artifacts
- debugging support
- recovery clues if the RPC process dies unexpectedly

Do not design the first implementation around log tailing.

## Session model

The harness must preserve Pi session identity across requests when desired.

Required tracked state:

- `agentId`
- `cwd`
- `pi sessionId`
- `pi sessionFile`
- `startedAt`
- `isStreaming`
- `pending Discord message id`

Session rules:

- one harness-controlled Pi RPC process per active agent binding
- one active Pi session per agent process unless explicitly reset
- reuse the same Pi session for continuing conversation in the same Discord-bound agent
- allow future extension for explicit new-session/reset behavior, but do not require it for v1

## Messaging model

Map Discord bridge work onto Pi RPC like this.

### Default

- first inbound message: `prompt`
- later inbound message while idle: `prompt`

### While Pi is already streaming

Choose one initial policy and keep it simple.

Recommended v1 policy:

- if new inbound Discord work arrives while Pi is busy, send it as `steer`

Possible future expansion:

- allow bridge-level choice between `steer` and `follow_up`

## Status model

Initial Discord bridge output should remain minimal.

### v1 reactions

- when Pi starts processing: set in-progress reaction
- when Pi completes successfully: set final success reaction
- when Pi errors or is aborted: set failure/abort reaction

### v1 messages

- send only the final assistant reply text back to Discord
- do not stream token-by-token or delta-by-delta into Discord yet

This keeps the bridge UX stable while the RPC integration is validated.

## Thinking visibility

The harness should capture thinking events from Pi RPC, but Discord should not necessarily display them yet.

Policy:

- ingest `thinking_start`, `thinking_delta`, and `thinking_end` when present
- keep them in the internal event model and logs
- do not expose them to Discord users in v1 unless there is a specific product reason

This preserves future flexibility without complicating the initial bridge UX.

## Tool visibility

The harness should capture tool execution lifecycle for observability.

Capture from Pi RPC:

- `tool_execution_start`
- `tool_execution_update`
- `tool_execution_end`

Bridge behavior in v1:

- do not send raw tool output to Discord by default
- use tool lifecycle only for internal state and debugging

## Errors and recovery

Handle at least these failure modes.

### Pi process exits unexpectedly

- mark agent process unhealthy
- report bridge failure status for the pending message if one exists
- require a restart or automatic relaunch depending on final implementation choice

### RPC parse error

- treat as process/protocol failure
- log raw line
- fail the active task safely

### No final assistant text available

- send a bridge-safe fallback message indicating completion without extractable final text
- still mark the task complete

### Discord bridge send failure

- keep Pi session alive
- log failure clearly
- do not corrupt Pi session state because downstream delivery failed

## Logging

Harness-owned logs should include:

- Pi process launch command and cwd
- Pi `sessionId` and `sessionFile` when known
- inbound Discord message ids
- outbound bridge completion ids/results
- raw RPC lines on parse failure
- summarized event timeline per handled request

## Intended first slice

The intended first slice inside this scope is:

- one Pi RPC process
- one active Discord-bound agent
- one prompt in, one final reply out
- session continuity preserved
- reactions updated correctly

Everything else in this document defines the allowed target surface and boundaries for that slice.

## Non-goals for this scope

These are explicitly deferred:

- generic adapter framework for many agent backends
- Claude stream integration
- Codex integration
- session-log tailing first architecture
- PTY fallback first architecture
- direct user exposure of thinking traces in Discord
- rich streamed partial-response UX in Discord

## Acceptance criteria

The initial implementation is successful when all of the following are true:

1. The harness launches Pi only through `--mode rpc`.
2. A Discord inbound request can be forwarded to Pi and produce a final Discord reply.
3. The bridge can show started/completed/error status via reactions.
4. The harness preserves Pi session continuity across multiple requests to the same bound agent.
5. The harness can tolerate normal RPC event flow without depending on PTY or log scraping.
6. Tool and thinking events are captured internally even if not shown to Discord users.

## Recommendation

Treat this as a boundary document, not a sequencing document.

The key constraint is:

- Pi RPC is the only upstream
- the Discord bridge is the only downstream
- logs and PTY are fallback/debugging aids, not the primary architecture

Within those boundaries, implementation can proceed incrementally.
