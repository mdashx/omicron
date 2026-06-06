# Discord Bridge Service Implementation Plan

## Goal

Add an always-on Discord bridge service that connects to a Discord bot, maintains bridge-owned logs/downloads, and gives Pi-compatible agents a stable channel binding and a small bridge-owned harness for repeatable behavior.

The bridge must enforce routing, reactions, logging, and state in runtime code, not by prompting agents to self-police.

---

## Plan by spec component

### 1. `DiscordBridgeService`

**Build target**
- startup config loader
- daemon/service lifecycle
- bot connection manager
- bridge-owned storage root setup

**Implementation steps**
- Add a built-in bridge config with sensible defaults.
- Load user overrides before accepting any agent joins.
- Merge overrides onto defaults.
- Validate resolved keys:
  - `enabled`
  - `botToken`
  - `bridgeId`
  - `host`
  - `port`
  - `storageRoot`
- Create the storage root recursively if it does not exist.
- Keep the Discord bot connection alive.
- Refuse to start if the bridge is enabled but the resolved config is invalid.

**Done when**
- the bridge can start with defaults only, but still fails on malformed resolved config.

---

### 2. `DiscordBridgeConfig`

**Build target**
- config resolution and defaulting

**Implementation steps**
- Define a bridge config shape with defaults for:
  - bot token source
  - bridge id
  - host/port
  - storage roots
  - default guild/channel bindings
- Permit config/env overrides.
- Keep bridge config outside the agent prompt path.
- Treat the storage root as bridge-owned runtime home, not an agent workspace.

**Done when**
- a new install gets a working bridge path without extra config.

---

### 3. `DiscordAgentIdentity` and `DiscordChannelBinding`

**Build target**
- agent registry and channel ownership

**Implementation steps**
- Generate stable agent/bridge/channel identity records.
- Persist who is bound to which Discord channel.
- Enforce one active agent per channel.
- Restore bindings on reconnect when valid.
- Reject joins for channels already owned by another active agent.

**Done when**
- the bridge can track multiple agents without cross-channel leakage.

---

### 4. `DiscordBridgeJoin`

**Build target**
- explicit join handshake
- scoped permissions

**Implementation steps**
- Accept an explicit join call with agent id, creds ref, and requested guild/channel.
- Resolve scope in the bridge, not in the agent.
- Deny joins with missing credentials or conflicting bindings.
- Make join idempotent so reconnects are stable.

**Done when**
- an approved agent can join and resume the same binding reliably.

---

### 5. `DiscordSharedStore`

**Build target**
- bridge-owned logs/downloads store

**Implementation steps**
- Create bridge-owned roots for logs and attachments/downloads.
- Make logs append-only.
- Persist attachment metadata and downloaded files.
- Expose history lookup and file fetch through bridge RPC.
- Keep agent access scoped through bridge policy.

**Done when**
- the bridge can independently retain and serve past chats and attachments.

---

### 6. `DiscordInboundMessage` and `DiscordOutboundDelivery`

**Build target**
- event normalization and outbound send path

**Implementation steps**
- Normalize inbound Discord events into stable internal records.
- Preserve author, timestamps, message ids, thread/reply metadata, and attachments.
- Route only to the bound agent.
- Send outbound replies, attachments, and thread-aware responses back to the bound channel.
- Persist every delivery in the audit trail.

**Done when**
- inbound and outbound traffic follow one deterministic bridge route.

---

### 7. `DiscordBridgeReactiveHarness`

**Build target**
- bridge-owned repeatable UX behavior

**Implementation steps**
- Add an acknowledgement reaction when a message is accepted.
- Update the reaction to reflect LLM/agent progress.
- Expose a constrained final emoji palette for the agent to choose from.
- Keep the reaction protocol bridge-owned so it behaves consistently regardless of agent/model.
- Reserve room for other deterministic bridge behaviors such as read receipts, progress markers, routing banners, retry notices, and attachment lifecycle signals.

**Done when**
- the bridge can provide consistent structural behavior across agents and models.

---

### 8. `DiscordBridgeWholesaleMemory`

**Build target**
- bridge-wide durable memory for chats and attachments

**Implementation steps**
- Log every inbound and outbound chat locally.
- Store attachment metadata and file references locally.
- Keep these logs bridge-owned and independent of any single agent session.
- Make logs available for replay, search, audit, and delayed agent join.
- Preserve ordering and channel context.

**Done when**
- the bridge can retain a complete local record of channel activity regardless of which agent is active.

---

### 9. `DiscordBridgeAuditLog`

**Build target**
- append-only operational audit trail

**Implementation steps**
- Append join/leave, binding, routing, downloads, and message fanout events.
- Keep the audit log separate from agent session history.
- Preserve records across restarts.
- Keep audit writes deterministic and durable.

**Done when**
- the bridge can explain what happened even after a restart.

---

### 10. `DiscordBridgeEnforcement`

**Build target**
- runtime guarantees independent of LLM behavior

**Implementation steps**
- Load the bridge during startup.
- Register hooks for inbound events, outbound actions, reactions, logging, and shutdown.
- Fail startup if enabled bridge wiring is missing.
- Keep all bridge decisions in runtime code only.

**Done when**
- the LLM is not part of the bridge decision path.

---

## Suggested system shape

### Runtime layers

1. **CLI/bootstrap layer**
   - loads defaults and user overrides
   - creates the bridge envelope
   - installs bridge hooks before agent work begins

2. **Discord bridge layer**
   - connects to Discord
   - normalizes inbound events
   - applies reactive harness behavior
   - logs chats and attachments
   - routes to bound agents

3. **Agent runtime**
   - joins the bridge and produces content
   - remains decoupled from channel ownership details

4. **Bridge persistence**
   - stores logs, downloads, audit, and binding state
   - keeps bridge-owned memory separate from agent sessions

### Enforced boundaries

- Discord may feed agents.
- Agents may not decide whether bridge persistence exists.
- The bridge owns channel structure and durable memory.
- The bridge owns the reaction workflow and logs.

---

## Suggested build order

### Phase 1 — config + identity
- bridge defaults
- config merge/resolution
- service startup validation
- bridge and agent identity records

### Phase 2 — channel routing + persistence
- join/binding flow
- inbound normalization
- outbound delivery
- logs and attachments store

### Phase 3 — harness behavior
- ack reaction
- progress reaction updates
- final reaction choice palette
- repeatable bridge UX behavior

### Phase 4 — enforcement and durability
- audit log
- dedupe/idempotency
- restart recovery
- shutdown cleanup

---

## Recommended first milestone

The bridge starts with default settings, connects to a Discord bot, allows an approved agent to join one channel, automatically logs all chats and attachments locally, adds an acknowledgement reaction on message receipt, updates reactions during processing, lets the agent choose from a final reaction palette, and persists enough state to avoid duplicate handling after restart.

---

## Follow-up work

- add replay tooling for logs and attachments
- add richer routing modes and thread policies
- add per-agent reaction templates
- add tests for binding conflicts, dedupe, and restart recovery
- add a local dashboard for bridge status and history
