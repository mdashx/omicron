# Agent RPC Scope

## Scope

This document intentionally narrows the project to a single upstream agent interface and a single downstream output target.

In scope:

- upstream: `Pi` via `pi --mode rpc`
- downstream: a very simple local CLI tool on the other end of the harness

Out of scope for the initial implementation:

- Claude integration
- Codex integration
- session-log tailing as a primary path
- PTY parsing as a primary path
- Discord, Slack, web UI, or other transports
- generic multi-agent abstraction beyond what is needed for Pi RPC

## Goal

Build a harness component that launches Pi in RPC mode, maintains session continuity, consumes Pi’s structured events, and relays useful status and completions to a very simple local CLI tool.

The resulting system should let a local CLI act as the user-facing transport while Pi RPC remains the only agent backend.

## Why this scope

Pi RPC is already the strongest available native interface in this repo’s ecosystem.

It gives us:

- structured command/response control
- structured event streaming
- session identity and persistence
- queueing semantics
- tool execution visibility
- thinking visibility when the provider exposes it

By focusing only on Pi RPC and a very simple local CLI first, we can validate the harness architecture without getting distracted by multiple upstreams or multiple outputs.

## Primary use case

1. A local CLI user sends a message to the harness.
2. The harness converts that message into a Pi RPC `prompt`, `steer`, or `follow_up` request.
3. Pi emits structured events over RPC stdout.
4. The harness normalizes the events into a smaller CLI-facing model.
5. The CLI displays:
   - simple progress or state updates
   - the final reply
   - optionally richer streamed output later

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
- mapping CLI inbound work to Pi RPC calls
- deciding when to use:
  - `prompt`
  - `steer`
  - `follow_up`
  - `abort`

### 3. `PiEventMapper`

Responsible for translating Pi RPC events into a smaller internal event model suitable for the CLI tool.

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

### 4. `CliAdapter`

Responsible for integrating the mapped Pi events with a very simple local CLI behavior.

Initial responsibilities:

- print simple working/in-progress state when Pi starts work
- print final completion output when Pi completes
- send exactly one completion output back to the CLI for each inbound CLI request
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
- `pending request id`

Session rules:

- one harness-controlled Pi RPC process per active harness instance
- one active Pi session per agent process unless explicitly reset
- reuse the same Pi session for continuing conversation in the same CLI-driven session
- allow future extension for explicit new-session/reset behavior, but do not require it for v1

## Messaging model

Map CLI work onto Pi RPC like this.

### Default

- first inbound message: `prompt`
- later inbound message while idle: `prompt`

### While Pi is already streaming

Choose one initial policy and keep it simple.

Recommended v1 policy:

- if new inbound CLI work arrives while Pi is busy, send it as `steer`

Possible future expansion:

- allow CLI-level choice between `steer` and `follow_up`

## Status model

Initial CLI output should remain minimal.

### v1 status lines

- when Pi starts processing: print a simple in-progress status line
- when Pi completes successfully: print the final assistant reply
- when Pi errors or is aborted: print a simple error or abort status line

### v1 output policy

- send only the final assistant reply text back to the CLI by default
- do not require token-by-token or delta-by-delta rendering yet

This keeps the CLI UX stable while the RPC integration is validated.

## Thinking visibility

The harness should capture thinking events from Pi RPC, but the CLI should not necessarily display them yet.

Policy:

- ingest `thinking_start`, `thinking_delta`, and `thinking_end` when present
- keep them in the internal event model and logs
- do not expose them to CLI users in v1 unless there is a specific product reason

This preserves future flexibility without complicating the initial CLI UX.

## Tool visibility

The harness should capture tool execution lifecycle for observability.

Capture from Pi RPC:

- `tool_execution_start`
- `tool_execution_update`
- `tool_execution_end`

CLI behavior in v1:

- do not send raw tool output to the CLI by default
- use tool lifecycle only for internal state and debugging

## Errors and recovery

Handle at least these failure modes.

### Pi process exits unexpectedly

- mark agent process unhealthy
- report failure status for the pending request if one exists
- require a restart or automatic relaunch depending on final implementation choice

### RPC parse error

- treat as process/protocol failure
- log raw line
- fail the active task safely

### No final assistant text available

- send a CLI-safe fallback message indicating completion without extractable final text
- still mark the task complete

### CLI output failure

- keep Pi session alive where possible
- log failure clearly
- do not corrupt Pi session state because downstream delivery failed

## Logging

Harness-owned logs should include:

- Pi process launch command and cwd
- Pi `sessionId` and `sessionFile` when known
- inbound request ids
- outbound completion ids/results
- raw RPC lines on parse failure
- summarized event timeline per handled request

## Intended first slice

The intended first slice inside this scope is:

- one Pi RPC process
- one active CLI-driven session
- one prompt in, one final reply out
- session continuity preserved
- simple CLI status output working correctly

Everything else in this document defines the allowed target surface and boundaries for that slice.

## Non-goals for this scope

These are explicitly deferred:

- generic adapter framework for many agent backends
- Claude stream integration
- Codex integration
- session-log tailing first architecture
- PTY fallback first architecture
- direct user exposure of thinking traces in the CLI
- rich streamed partial-response UX in the CLI

## Acceptance criteria

The initial implementation is successful when all of the following are true:

1. The harness launches Pi only through `--mode rpc`.
2. A CLI request can be forwarded to Pi and produce a final CLI reply.
3. The CLI can show started/completed/error status in a simple way.
4. The harness preserves Pi session continuity across multiple requests in the same harness session.
5. The harness can tolerate normal RPC event flow without depending on PTY or log scraping.
6. Tool and thinking events are captured internally even if not shown to CLI users.

## Recommendation

Treat this as a boundary document, not a sequencing document.

The key constraint is:

- Pi RPC is the only upstream
- a very simple local CLI is the only downstream
- logs and PTY are fallback/debugging aids, not the primary architecture

Within those boundaries, implementation can proceed incrementally.
