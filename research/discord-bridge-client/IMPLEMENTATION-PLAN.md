# Discord Bridge Client PTY Harness Implementation Plan

## Goal

Add a Discord bridge client harness that joins the Discord bridge service as one logical agent, runs the real agent inside a PTY, and mediates all bridge I/O on the agent's behalf.

The harness must enforce join, polling, prompt injection, status updates, and completion delivery in runtime code, not by prompting the agent to remember Discord behavior.

---

## Plan by spec component

### 1. `BridgeClientHarness`

**Build target**
- startup config loader
- local supervisor lifecycle
- bridge client bootstrap
- PTY process manager

**Implementation steps**
- Add a built-in harness config with sensible defaults.
- Load user overrides before the first bridge join.
- Validate bridge URL, agent id, command, cwd, and PTY settings.
- Start the harness as a local supervisor process.
- Keep the harness responsible for all bridge-facing behavior.

**Done when**
- the harness can start with defaults only, but still fails malformed resolved config.

---

### 2. `BridgeClientConfig`

**Build target**
- config resolution and defaulting

**Implementation steps**
- Define defaults for:
  - bridge URL
  - agent id
  - creds ref
  - guild/channel binding
  - command and args
  - cwd
  - poll interval
  - PTY cols/rows
- Permit config/env overrides.
- Keep harness config outside the agent prompt path.

**Done when**
- a new harness install can join a local bridge with minimal setup.

---

### 3. `BridgeJoinSession`

**Build target**
- explicit join handshake

**Implementation steps**
- Call `POST /join` before processing bridge events.
- Rejoin idempotently on reconnect.
- Refuse to consume queue events before a successful join.
- Persist enough join state to recover cleanly.

**Done when**
- the harness can reliably reclaim its agent/channel binding.

---

### 4. `AgentPTYProcess`

**Build target**
- PTY-backed agent runtime

**Implementation steps**
- Spawn the agent command inside a pseudo-terminal.
- Keep stdout/stderr merged through the PTY.
- Preserve terminal semantics for interactive/TUI agent behavior.
- Track PTY process lifecycle and restart/exit behavior.

**Done when**
- the agent behaves like it is attached to a normal terminal.

---

### 5. `BridgeEventPollLoop`

**Build target**
- event polling and queue drain loop

**Implementation steps**
- Poll `GET /agents/{agentId}/events` on a steady interval.
- Normalize bridge queue items into harness work items.
- Deduplicate locally if needed.
- Keep transport mechanics out of agent prompt logic.

**Done when**
- the harness can receive and process bridge events deterministically.

---

### 6. `BridgePromptInjection`

**Build target**
- harness-owned agent input shaping

**Implementation steps**
- Map bridge events into stable agent-facing input text.
- Include author, channel, timestamps, and attachments when relevant.
- Feed the result into the PTY-backed agent process.
- Keep the bridge source explicit but compact.

**Done when**
- Discord-originated requests can be shown to the agent through the PTY without direct Discord integration inside the agent.

---

### 7. `BridgeStatusProjection`

**Build target**
- bridge-side progress publishing

**Implementation steps**
- Detect working phases such as queued, thinking, tool-use, and complete.
- Publish status changes through `POST /agents/{agentId}/status`.
- Use only bridge-approved reaction values.
- Keep status publishing deterministic and outside model prose.

**Done when**
- the bridge can reflect the agent's live state through reactions.

---

### 8. `BridgeCompletionDelivery`

**Build target**
- final response return path

**Implementation steps**
- Capture the final agent output for a bridge-originated turn.
- Send the reply with `POST /agents/{agentId}/complete`.
- Optionally include one bridge-approved final reaction.
- Prevent duplicate completion sends.

**Done when**
- Discord receives exactly one final response per handled bridge event.

---

### 9. `HarnessSessionState`

**Build target**
- local durable harness state

**Implementation steps**
- Persist processed event ids locally.
- Persist current active message metadata when needed.
- Persist last join state and PTY liveness hints if useful.
- Keep harness state separate from bridge state and agent session files.

**Done when**
- the harness can restart without replaying already-completed Discord work.

---

### 10. `HarnessEnforcement`

**Build target**
- runtime guarantees independent of model behavior

**Implementation steps**
- Load the harness before the agent starts receiving bridge work.
- Enforce bridge join, polling, prompt injection, status updates, and completion in runtime code.
- Fail startup if enabled harness wiring is missing.
- Keep Discord/bridge semantics outside the model decision path.

**Done when**
- the model is not part of the transport control-plane decision path.

---

## Suggested system shape

### Runtime layers

1. **Harness bootstrap layer**
   - loads defaults and user overrides
   - validates bridge + PTY configuration
   - performs bridge join before work begins

2. **Bridge client loop**
   - polls events
   - publishes status
   - sends completion

3. **PTY agent runtime**
   - runs the actual agent CLI/TUI
   - receives harness-injected input
   - emits output observed by the harness

4. **Harness persistence**
   - stores processed event ids and restart-safe local state

### Enforced boundaries

- The bridge owns Discord transport and durable chat memory.
- The harness owns agent/PTY supervision and bridge client behavior.
- The agent only sees harness-mediated input/output.

---

## Suggested build order

### Phase 1 — config + bridge join
- harness defaults
- config merge/resolution
- join handshake
- local state setup

### Phase 2 — PTY process + polling
- PTY spawn
- event poll loop
- queue processing
- input injection

### Phase 3 — status + completion
- status publishing
- final reply delivery
- final reaction choice wiring

### Phase 4 — hardening
- dedupe
- restart recovery
- stronger prompt shaping
- better PTY lifecycle controls

---

## Recommended first milestone

The harness starts with default settings, joins the local bridge as one agent/channel binding, launches `pi` inside a PTY, polls bridge events, injects Discord-originated requests into the PTY session, publishes status reactions back through the bridge, and sends one final completion reply back to Discord without duplicate handling.

---

## Follow-up work

- add richer PTY transcript parsing
- add stronger restart/replay handling
- add structured bridge event templates
- add multi-agent harness supervisor mode
- add tests for join recovery, duplicate prevention, and PTY failure recovery
