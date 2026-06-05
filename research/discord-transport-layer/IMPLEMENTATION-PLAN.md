# Pi Discord Transport Layer Implementation Plan

## Goal

Add a startup-loaded Discord transport layer to Pi that can receive Discord gateway events, normalize them into agent-friendly events, and send agent outputs back as Discord replies, reactions, and thread actions.

The transport must be enforced by runtime code, not by prompting the model.

---

## Plan by spec component

### 1. `DiscordTransportDefaults` and `DiscordTransportConfig`

**Build target**
- startup config loader
- default resolution
- strict validation
- fail-closed boot behavior

**Implementation steps**
- Add a built-in Discord transport config with sensible defaults.
- Load user overrides before the Pi agent loop begins.
- Merge overrides onto defaults.
- Validate resolved keys:
  - `enabled`
  - `tokenSource`
  - `commandPrefix`
  - `guildAllowlist`
  - `channelAllowlist`
  - `ownerAllowlist`
  - `dryRun`
  - `statePath`
  - `threadMode`
  - `replyMode`
  - `maxActionsPerTurn`
- Create the state directory recursively if needed.
- Refuse to start if Discord transport is enabled but config is invalid.

**Done when**
- Pi can start with defaults only, but still refuses malformed resolved config.

---

### 2. `DiscordSessionEnvelope`

**Build target**
- runtime session identity and metadata

**Implementation steps**
- Generate `sessionId` at startup using ISO timestamp + short random suffix.
- Capture `startedAt`, `source`, `host`, `runtimeVersion`, `packageVersion`, `botUserId`, `applicationId`, and `transportMode`.
- Store the envelope in Pi runtime state.
- Emit a `ready`/session-start style transport event after connection.

**Done when**
- every Discord-backed Pi session has a stable ID before the first inbound message.

---

### 3. `DiscordInboundEvent`

**Build target**
- normalized gateway event representation

**Implementation steps**
- Normalize gateway events immediately on receipt.
- Keep only the fields needed for routing and idempotency.
- Preserve raw payloads only in debug logs.
- Treat ready/error/heartbeat as transport lifecycle events.

**Done when**
- inbound Discord activity is represented in one stable internal shape.

---

### 4. `DiscordDirective`

**Build target**
- small deterministic command parser

**Implementation steps**
- Parse a tiny prefix grammar first.
- Support:
  - `!ping`
  - `!status`
  - `!echo <text>`
  - `!react <emoji>`
- Return `ignore` for non-matching messages.
- Keep parsing deterministic and side-effect free.

**Done when**
- command handling is testable without network access.

---

### 5. `DiscordActionPlan`

**Build target**
- pure action planning layer

**Implementation steps**
- Convert directives into data-only action plans.
- Support reply, reaction, thread, log-only, and no-op plans.
- Make `dryRun` print or log the plan instead of sending it.
- Enforce `maxActionsPerTurn` so a single turn cannot fan out indefinitely.

**Done when**
- execution is separated cleanly from planning.

---

### 6. `DiscordTransportState`

**Build target**
- minimal restart-safe memory

**Implementation steps**
- Store processed event IDs.
- Store processed action IDs.
- Store last-ready and heartbeat timestamps.
- Store per-channel cursor/last-seen message state if needed.
- Persist after successful action execution.

**Done when**
- the transport can restart without duplicate replies for already-processed messages.

---

### 7. `DiscordTransportRunLoop`

**Build target**
- end-to-end event processing loop

**Implementation steps**
- Connect Discord client.
- Receive gateway events.
- Scope-check guild/channel/owner access.
- Normalize inbound events.
- Parse directives.
- Build action plans.
- Execute or dry-run actions.
- Persist state after successful execution.

**Done when**
- the bot can handle a simple allowed channel flow reliably.

---

### 8. `DiscordTransportEnforcement`

**Build target**
- runtime guarantees independent of LLM behavior

**Implementation steps**
- Load the transport during Pi startup.
- Register hooks for inbound events, outbound actions, and shutdown.
- Validate records before execution.
- Fail startup if enabled transport wiring is missing.
- Keep transport decisions in runtime code only.

**Done when**
- the LLM is not part of the transport decision path.

---

## Suggested Pi system shape

### Runtime layers

1. **CLI/bootstrap layer**
   - loads defaults and user overrides
   - creates the session envelope
   - installs transport hooks before the model loop starts

2. **Discord transport layer**
   - connects to Discord
   - normalizes inbound events
   - plans actions
   - executes replies/reactions/threads

3. **Agent runtime**
   - consumes the normalized events
   - can generate higher-level responses

4. **State persistence**
   - stores processed IDs and cursors
   - prevents duplicate actions

### Enforced boundaries

- Discord may feed the agent.
- The agent may not decide whether Discord wiring exists.
- Pi runtime owns transport state.
- The transport layer owns Discord I/O.

---

## Suggested build order

### Phase 1 — config + envelope
- transport defaults
- config merge/resolution
- session ID generation
- startup validation
- ready event

### Phase 2 — event capture
- gateway event normalization
- command parsing
- action planning
- append/update state

### Phase 3 — enforcement
- plugin registration
- hook wiring
- startup failure modes
- scope checks and dedupe

### Phase 4 — hardening
- richer routing modes
- thread support
- reaction mappings
- crash recovery and replay

---

## Recommended first milestone

Pi starts with default Discord transport settings, connects with a bot token, accepts only allowlisted guild/channel messages, normalizes inbound events, parses a tiny prefix command set, emits deterministic action plans, sends or dry-runs replies and reactions, persists processed event IDs, and closes cleanly without duplicate replies.

---

## Follow-up work

- add thread creation and first-reply templates
- add richer mention-only / prefix / reaction routing modes
- add replay tooling for missed events
- add tests for startup failure, dedupe, and dry-run behavior
- add optional content-type routing for attachments
