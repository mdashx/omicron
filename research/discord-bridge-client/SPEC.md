# Discord Bridge Client PTY Harness Spec

## Intent

Add a Discord bridge client harness that connects to the always-on Discord bridge service, binds as one logical agent, and runs the actual agent process inside a PTY.

The harness should make the agent feel like it is running in a normal terminal session, while the harness owns Discord transport, binding, polling, status updates, and final reply delivery.

For Pi-backed agents, inbound work should still be delivered through PTY input so the agent continues to experience an ordinary terminal session. Outbound assistant replies should be derived from Pi's structured JSONL logs rather than scraped from terminal escape-coded PTY output.

This must be enforced by the harness runtime, not by prompting the model to remember Discord behavior.

---

## 1. `BridgeClientHarness`

### Prose Spec

The bridge client harness is a long-lived local supervisor process.

It connects to the bridge service, joins as one agent identity, launches the agent process inside a PTY, and mediates all bridge I/O on the agent's behalf.

### Z Spec

```text
BridgeClientHarness
  enabled: 𝔹
  bridgeUrl: seq CHAR
  agentId: seq CHAR
  command: seq CHAR
  cwd: seq CHAR
  pty: 𝔹
where
  enabled = true
  bridgeUrl ≠ ⟨⟩
  agentId ≠ ⟨⟩
  command ≠ ⟨⟩
  cwd ≠ ⟨⟩
  pty = true
```

### Data examples

```json
{
  "enabled": true,
  "bridgeUrl": "http://127.0.0.1:19444",
  "agentId": "main",
  "command": "pi",
  "cwd": "/home/easter/omicron",
  "pty": true
}
```

### Implementation suggestions / specifics

- Start as a local process or service.
- Join the bridge before the first agent turn.
- Spawn the agent inside a PTY, not a plain pipe.
- Keep the bridge client responsible for Discord-facing behavior.

---

## 2. `BridgeClientConfig`

### Prose Spec

The harness should have built-in defaults and support config/env overrides.

It should resolve bridge URL, agent identity, command, cwd, polling behavior, and PTY sizing.

### Z Spec

```text
BridgeClientConfig
  enabled?: 𝔹
  bridgeUrl?: seq CHAR
  agentId?: seq CHAR
  credsRef?: seq CHAR
  guildId?: seq CHAR
  channelId?: seq CHAR
  command?: seq CHAR
  args?: seq CHAR
  cwd?: seq CHAR
  pollIntervalMs?: ℕ
  cols?: ℕ
  rows?: ℕ
  outputMode?: seq CHAR
  piSessionRoot?: seq CHAR
  piSessionArchiveRoot?: seq CHAR
  piLogPreference?: seq CHAR
```

### Data examples

```json
{
  "bridgeUrl": "http://127.0.0.1:19444",
  "agentId": "main",
  "credsRef": "local-session",
  "guildId": "1478102509330497721",
  "channelId": "1504560627325079642",
  "command": "pi",
  "args": ["-c"],
  "cwd": "/home/easter/omicron",
  "pollIntervalMs": 1500,
  "cols": 120,
  "rows": 40,
  "outputMode": "pi-jsonl",
  "piSessionRoot": "~/.pi/agent/sessions",
  "piSessionArchiveRoot": "~/.pi/agent/session-archive",
  "piLogPreference": "session-archive"
}
```

### Implementation suggestions / specifics

- Default to PTY mode.
- Default the command to `pi`.
- Default the bridge URL to local loopback.
- Add harness-owned config for structured Pi log discovery.
- Keep config outside the agent prompt path.

---

## 3. `BridgeJoinSession`

### Prose Spec

The harness must explicitly join the bridge as an agent before processing Discord-originated work.

The bridge join is a runtime fact owned by the harness, not the model.

### Z Spec

```text
BridgeJoinSession
  agentId: seq CHAR
  credsRef: seq CHAR
  guildId: seq CHAR
  channelId: seq CHAR
  joinedAt: seq CHAR
where
  agentId ≠ ⟨⟩
  credsRef ≠ ⟨⟩
  channelId ≠ ⟨⟩
  joinedAt ≠ ⟨⟩
```

### Data examples

```json
{
  "agentId": "main",
  "credsRef": "local-session",
  "guildId": "1478102509330497721",
  "channelId": "1504560627325079642",
  "joinedAt": "2026-06-06T03:10:00Z"
}
```

### Implementation suggestions / specifics

- Perform `POST /join` before polling events.
- Rejoin idempotently on reconnect.
- Refuse to process bridge events until the join succeeds.

---

## 4. `AgentPTYProcess`

### Prose Spec

The harness launches the agent process inside a PTY so the agent behaves as if it is attached to a normal terminal.

This preserves terminal assumptions, TUI behavior, and interactive command semantics.

### Z Spec

```text
AgentPTYProcess
  pid: ℕ
  command: seq CHAR
  args: seq CHAR
  cols: ℕ
  rows: ℕ
  alive: 𝔹
where
  pid > 0
  command ≠ ⟨⟩
  cols > 0
  rows > 0
```

### Data examples

```json
{
  "pid": 43210,
  "command": "pi",
  "args": ["-c"],
  "cols": 120,
  "rows": 40,
  "alive": true
}
```

### Implementation suggestions / specifics

- Use a real pseudo-terminal.
- Keep stdout/stderr merged through the PTY.
- Allow resize later if needed.
- Treat the PTY wrapper as the durable harness boundary.
- Keep PTY as the authoritative inbound control path even when outbound replies are sourced from structured logs.

---

## 5. `BridgeEventPollLoop`

### Prose Spec

The harness polls or subscribes to bridge events for its bound agent id.

Each inbound event is normalized into a harness-owned unit of work before it is shown to the agent.

### Z Spec

```text
BridgeEventPollLoop
  agentId: seq CHAR
  pollIntervalMs: ℕ
  queueDepth: ℕ
where
  agentId ≠ ⟨⟩
  pollIntervalMs > 0
```

### Data examples

```json
{
  "agentId": "main",
  "pollIntervalMs": 1500,
  "queueDepth": 3
}
```

### Implementation suggestions / specifics

- Poll `GET /agents/{agentId}/events`.
- Deduplicate if needed on the client side as well as the bridge side.
- Treat each event as a structured inbound request.
- Keep bridge event handling outside model text.

---

## 6. `BridgePromptInjection`

### Prose Spec

The harness translates bridge events into agent-facing input.

This may appear as terminal input, synthetic session prompts, or a structured wrapper message, but the mapping is owned by the harness.

### Z Spec

```text
BridgePromptInjection
  eventId: seq CHAR
  agentInput: seq CHAR
  source: seq CHAR
where
  eventId ≠ ⟨⟩
  agentInput ≠ ⟨⟩
  source = "discord-bridge"
```

### Data examples

```json
{
  "eventId": "evt_123",
  "agentInput": "[discord-bridge]\nAuthor: easter\nChannel: 1504560627325079642\nMessage: summarize the latest log",
  "source": "discord-bridge"
}
```

### Implementation suggestions / specifics

- Keep the formatting stable and minimal.
- Include author, channel, timestamps, and attachments when present.
- Make it obvious to the agent that the source is bridge-mediated.
- Avoid leaking transport mechanics into normal user prompt flow unless intentional.
- Write injected work through PTY so the agent continues to believe it is talking only to a terminal session.

---

## 7. `BridgeStatusProjection`

### Prose Spec

While the agent is working, the harness reports progress back to the bridge so the bridge can update reactions and status indicators.

The harness is the status publisher; the bridge is the status renderer.

### Z Spec

```text
BridgeStatusProjection
  agentId: seq CHAR
  messageId: seq CHAR
  reaction: seq CHAR
  phase: seq CHAR
where
  agentId ≠ ⟨⟩
  messageId ≠ ⟨⟩
  reaction ≠ ⟨⟩
```

### Data examples

```json
{
  "agentId": "main",
  "messageId": "discord_msg_1",
  "reaction": "💭",
  "phase": "thinking"
}
```

### Implementation suggestions / specifics

- Publish queued / thinking / tool-use / complete transitions.
- Use bridge-approved reaction values.
- Keep status updates deterministic and separate from model prose.

---

## 8. `BridgeCompletionDelivery`

### Prose Spec

When the agent finishes a bridge-originated turn, the harness sends the final response back through the bridge.

For Pi-backed agents, the harness should derive reply content from Pi's structured JSONL session logs rather than from PTY screen scraping. The harness may still use PTY observations for liveness, fallback behavior, or debugging, but PTY output is not the authoritative response source when Pi logs are available.

The harness may also provide the bridge-approved final emoji choice.

### Z Spec

```text
BridgeCompletionDelivery
  agentId: seq CHAR
  messageId: seq CHAR
  content: seq CHAR
  finalReaction?: seq CHAR
where
  agentId ≠ ⟨⟩
  messageId ≠ ⟨⟩
```

### Data examples

```json
{
  "agentId": "main",
  "messageId": "discord_msg_1",
  "content": "Here is the summary.",
  "finalReaction": "✅"
}
```

### Implementation suggestions / specifics

- Use `POST /agents/{agentId}/complete`.
- Keep final reactions constrained to the bridge palette.
- Send completion only once per bridge event.
- Prefer Pi JSONL log tailing as the authoritative completion source.
- Fall back to PTY-derived completion only for non-Pi commands or when no usable Pi log source is registered.

---

## 9. `PiStructuredOutputSource`

### Prose Spec

For Pi-backed agents, the harness should resolve and register a structured JSONL output source for the launched agent session.

The harness owns this registration step. The agent process itself still runs in a PTY and does not need to know about Discord or about log relaying semantics.

### Z Spec

```text
PiStructuredOutputSource
  agentId: seq CHAR
  mode: seq CHAR
  sessionFile?: seq CHAR
  archiveFile?: seq CHAR
  registeredAt: seq CHAR
  active: 𝔹
where
  agentId ≠ ⟨⟩
  mode = "pi-jsonl"
  registeredAt ≠ ⟨⟩
  active = true
```

### Data examples

```json
{
  "agentId": "main",
  "mode": "pi-jsonl",
  "sessionFile": "/home/easter/.pi/agent/sessions/--home-easter-omicron--/2026-06-06T03-47-32-000Z_abc.jsonl",
  "archiveFile": "/home/easter/.pi/agent/session-archive/2026/06/06/2026-06-06T03-47-32Z_d388.jsonl",
  "registeredAt": "2026-06-06T03:47:33Z",
  "active": true
}
```

### Implementation suggestions / specifics

- Allow the harness to be configured with both native Pi session roots and session-archive roots.
- Resolve the active log file after agent launch using launch time, cwd, and newest matching file heuristics.
- Prefer session-archive JSONL when it is present and current.
- Fall back to native Pi session JSONL when session-archive is unavailable.
- Keep the output-source registration harness-owned and outside the prompt path.

---

## 10. `HarnessSessionState`

### Prose Spec

The harness should keep enough local state to survive restarts and avoid duplicate Discord replies.

### Z Spec

```text
HarnessSessionState
  processedEventIds: ℙ seq CHAR
  activeMessageId?: seq CHAR
  lastJoinAt?: seq CHAR
  ptyAlive: 𝔹
```

### Data examples

```json
{
  "processedEventIds": ["evt_123", "evt_124"],
  "activeMessageId": "discord_msg_1",
  "lastJoinAt": "2026-06-06T03:10:00Z",
  "ptyAlive": true
}
```

### Implementation suggestions / specifics

- Persist processed event ids locally.
- Persist enough join/binding state to recover cleanly.
- Distinguish bridge state from agent session state.

---

## 11. `HarnessInvariants`

### Prose Spec

- The harness joins the bridge before handling Discord work.
- The agent runs inside a PTY.
- The harness owns Discord/bridge I/O.
- The agent does not directly own Discord transport semantics.
- Inbound work is delivered to the agent through PTY input.
- For Pi-backed agents, outbound reply extraction should come from structured JSONL logs when available.
- PTY output may be used for liveness, fallback, and debugging, but is not the preferred authoritative output source for Pi.
- Status and completion updates go through the bridge.
- Duplicate completion should be prevented by harness and bridge state.

### Z Spec

```text
HarnessInvariants
  bridgeFirst: 𝔹
  ptyRequired: 𝔹
  harnessOwnsTransport: 𝔹
where
  bridgeFirst = true
  ptyRequired = true
  harnessOwnsTransport = true
```
