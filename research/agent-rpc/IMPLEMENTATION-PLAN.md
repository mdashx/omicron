# Agent RPC Harness Implementation Plan

## Goal

Build a narrow first implementation of the agent RPC harness that launches Pi through `--mode rpc`, preserves session continuity, consumes Pi’s structured RPC events, and exposes a very simple local CLI on the other side.

The implementation must enforce transport, session, and event behavior in runtime code, not by prompting the model to remember CLI or harness rules.

---

## Plan by spec component

### 1. `AgentRpcHarness`

**Build target**
- harness process lifecycle
- startup wiring
- high-level ownership boundaries

**Implementation steps**
- Create a local harness process entry point under `research/agent-rpc/`.
- Resolve defaults and user overrides before launching the upstream agent.
- Make the harness the only owner of:
  - upstream process lifecycle
  - downstream CLI interaction
  - session continuity bookkeeping
  - request/completion correlation
- Fail fast if the upstream launch config is malformed.

**Done when**
- the harness can start as a standalone local process with defaults only.

---

### 2. `AgentRpcHarnessConfig`

**Build target**
- config resolution and defaulting

**Implementation steps**
- Define a config shape with defaults for:
  - `agentId`
  - `command`
  - `args`
  - `cwd`
  - session persistence options
  - idle/debug behavior
- Default the upstream launch to `pi --mode rpc`.
- Permit config/env overrides.
- Keep config outside the upstream prompt path.

**Done when**
- a new install can launch the harness with no custom config beyond normal Pi prerequisites.

---

### 3. `UpstreamRpcProcess`

**Build target**
- Pi RPC subprocess wrapper
- strict JSONL framing
- process supervision

**Implementation steps**
- Launch Pi with `--mode rpc`.
- Implement strict LF-delimited JSONL stdin/stdout handling.
- Send one command at a time and correlate responses.
- Read both:
  - command responses
  - streamed events
- Treat invalid JSONL or protocol drift as an upstream process/protocol failure.
- Add clean shutdown behavior.

**Done when**
- the harness can launch Pi, send a `prompt`, and receive valid responses/events reliably.

---

### 4. `UpstreamSessionState`

**Build target**
- session continuity tracking
- state inspection

**Implementation steps**
- Call `get_state` after startup and after each completed request.
- Track:
  - `sessionId`
  - `sessionFile`
  - `sessionName`
  - `isStreaming`
  - `cwd`
- Keep one active Pi session per harness instance unless explicitly reset later.
- Reuse the same session for sequential CLI requests.

**Done when**
- two or more prompts in the same harness instance clearly share the same Pi session identity.

---

### 5. `DownstreamCliRequest`

**Build target**
- simple local CLI input path
- deterministic request-to-RPC mapping

**Implementation steps**
- Add a very small CLI wrapper or REPL-like loop.
- Accept a simple text request from stdin/argv.
- Map idle requests to `prompt`.
- If Pi is already streaming, map new requests to `steer` for v1.
- Preserve a request id locally for logging and completion matching.
- Keep request formatting minimal and stable.

**Done when**
- a user can type one prompt locally and the harness routes it correctly to Pi RPC.

---

### 6. `UpstreamRpcEventProjection`

**Build target**
- event normalization layer
- smaller internal harness event model

**Implementation steps**
- Normalize Pi RPC events into a smaller internal shape.
- Handle at least:
  - agent lifecycle
  - message lifecycle
  - assistant text updates
  - thinking updates
  - tool execution lifecycle
  - queue updates
  - errors
- Preserve unknown-event fallback behavior instead of failing hard.
- Keep RPC as the sole source of truth for normal execution flow.

**Done when**
- the harness can produce a stable internal event stream from Pi RPC without needing logs or PTY output.

---

### 7. `CliStatusProjection`

**Build target**
- minimal status UX for the local CLI

**Implementation steps**
- Print simple state transitions such as:
  - started
  - working
  - completed
  - error
- Keep status separate from final assistant content.
- Avoid full token-by-token rendering in v1 unless it is essentially free.

**Done when**
- a user can tell whether the harness is idle, working, or failed from CLI output alone.

---

### 8. `CliCompletionDelivery`

**Build target**
- one final reply per request

**Implementation steps**
- Derive final assistant reply text from Pi RPC events/state.
- Emit exactly one final completion output for each request.
- Add a safe fallback message if Pi completes without extractable final text.
- Make completion correlation deterministic via request id tracking.

**Done when**
- one inbound CLI request reliably yields one final assistant reply.

---

### 9. `ThinkingCapture`

**Build target**
- internal capture of thinking lifecycle

**Implementation steps**
- Record thinking lifecycle events when Pi/provider/model exposes them.
- Keep them in the internal event model and debug logs.
- Do not require showing them to the user in v1.

**Done when**
- thinking events are preserved internally when available, without being required for normal UX.

---

### 10. `ToolLifecycleCapture`

**Build target**
- internal capture of tool execution lifecycle

**Implementation steps**
- Record:
  - `tool_execution_start`
  - `tool_execution_update`
  - `tool_execution_end`
- Keep these for observability, debugging, and later richer UX.
- Do not send raw tool output to the user by default in v1.

**Done when**
- the harness can explain which tools were used during a request even if the CLI remains minimal.

---

### 11. `HarnessAuditState`

**Build target**
- local bookkeeping and debugging state

**Implementation steps**
- Track processed request ids.
- Track active request id.
- Track whether the upstream process is alive.
- Record `sessionId` and `sessionFile` when known.
- Log raw RPC lines on parse failure.
- Keep downstream request state separate from upstream session state.

**Done when**
- the harness can avoid duplicate completion behavior and diagnose basic continuity failures.

---

### 12. `AgentRpcHarnessInvariants`

**Build target**
- runtime guarantees independent of LLM behavior

**Implementation steps**
- Enforce that the upstream is launched only through RPC.
- Enforce that the CLI is the only downstream consumer in this prototype.
- Enforce that logs and PTY are not required for normal success.
- Keep all request routing and completion rules in harness code only.

**Done when**
- the core control path works even if the model never mentions the CLI or harness structure.

---

## Proposed system shape

### Runtime layers

1. **CLI/bootstrap layer**
   - loads harness defaults and user overrides
   - launches the upstream RPC process
   - starts the local CLI loop

2. **RPC client layer**
   - sends commands to Pi RPC
   - receives responses and events
   - preserves session continuity data

3. **Event projection layer**
   - converts raw Pi RPC events into a smaller internal harness model
   - separates final text, status, thinking, and tool lifecycle

4. **CLI adapter**
   - accepts user input
   - prints minimal state and final completions

5. **Harness audit state**
   - stores request/session/process bookkeeping
   - provides debugging breadcrumbs

### Enforced boundaries

- Pi RPC may provide agent behavior.
- The harness owns all request routing and output shaping.
- The CLI is only a consumer of harness output and a source of user input.
- Session logs and PTY output are fallback/debugging aids, not the primary runtime path.

---

## Suggested build order

### Phase 1 — startup + one-shot request
- config resolution
- Pi RPC process launch
- one `prompt` request
- one final reply output

### Phase 2 — event projection + session continuity
- `get_state`
- session identity tracking
- normalized event stream
- minimal status output

### Phase 3 — busy handling + observability
- `isStreaming` detection
- `steer` behavior while busy
- tool lifecycle capture
- thinking capture
- basic audit state

### Phase 4 — hardening
- parse failure handling
- upstream exit handling
- fallback completion behavior
- improved logs/tests

---

## Recommended first milestone

The harness starts locally with defaults, launches Pi in RPC mode, accepts one CLI prompt, prints simple working status, emits one final assistant reply, and preserves the same Pi session across repeated prompts without depending on PTY or log scraping.

---

## Follow-up work

- add a richer interactive CLI UX
- add explicit new-session/reset commands
- add replay or audit viewing tools
- add tests for response correlation, busy-state steering, and restart behavior
- add alternate downstreams later only after the CLI path is stable
