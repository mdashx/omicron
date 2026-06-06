# Pi RPC Discord Harness Spec

## Intent

Add a Pi RPC harness that connects Pi to the existing Discord bridge without using PTY output scraping as the primary control or output path.

The harness should launch Pi in `--mode rpc`, maintain session continuity, consume Pi's structured RPC events, and project status and final replies through the Discord bridge.

Pi RPC must be the only upstream agent interface for this feature.

The Discord bridge must be the only downstream output target for this feature.

This must be enforced by harness runtime wiring, not by prompting the model to remember transport rules.

---

## 1. `PiRpcHarness`

### Prose Spec

The harness is a long-lived local supervisor process.

It launches Pi in RPC mode, owns the Pi process lifecycle, tracks Pi session state, and mediates all communication between Pi and the Discord bridge.

### Z Spec

```text
PiRpcHarness
  enabled: 𝔹
  bridgeUrl: seq CHAR
  agentId: seq CHAR
  cwd: seq CHAR
  upstream: seq CHAR
  downstream: seq CHAR
where
  enabled = true
  bridgeUrl ≠ ⟨⟩
  agentId ≠ ⟨⟩
  cwd ≠ ⟨⟩
  upstream = "pi-rpc"
  downstream = "discord-bridge"
```

### Data examples

```json
{
  "enabled": true,
  "bridgeUrl": "http://127.0.0.1:19444",
  "agentId": "main",
  "cwd": "/home/easter/omicron",
  "upstream": "pi-rpc",
  "downstream": "discord-bridge"
}
```

### Implementation suggestions / specifics

- Start as a local process or service.
- Treat Pi RPC as the only supported upstream for the first implementation.
- Treat the Discord bridge as the only supported downstream for the first implementation.
- Keep transport ownership in harness code, not in the model prompt path.

---

## 2. `PiRpcHarnessConfig`

### Prose Spec

The harness should have built-in defaults and support config or environment overrides.

It should resolve bridge connectivity, Pi launch arguments, working directory, and session persistence behavior.

### Z Spec

```text
PiRpcHarnessConfig
  enabled?: 𝔹
  bridgeUrl?: seq CHAR
  agentId?: seq CHAR
  credsRef?: seq CHAR
  guildId?: seq CHAR
  channelId?: seq CHAR
  piCommand?: seq CHAR
  piArgs?: seq CHAR
  cwd?: seq CHAR
  sessionDir?: seq CHAR
  noSession?: 𝔹
  pollIntervalMs?: ℕ
  idleTimeoutMs?: ℕ
where
  piCommand = "pi" ∨ piCommand ≠ ⟨⟩
```

### Data examples

```json
{
  "bridgeUrl": "http://127.0.0.1:19444",
  "agentId": "main",
  "credsRef": "local-session",
  "guildId": "1478102509330497721",
  "channelId": "1504560627325079642",
  "piCommand": "pi",
  "piArgs": ["--mode", "rpc"],
  "cwd": "/home/easter/omicron",
  "pollIntervalMs": 1500,
  "idleTimeoutMs": 2500
}
```

### Implementation suggestions / specifics

- Default Pi launch to `pi --mode rpc`.
- Keep session persistence configurable through normal Pi flags.
- Keep bridge config outside the prompt path.
- Allow environment variable overrides for local development.

---

## 3. `BridgeJoinSession`

### Prose Spec

The harness must explicitly join the Discord bridge as one logical agent before processing inbound Discord work.

The bridge join is a runtime fact owned by the harness, not by Pi.

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
  "joinedAt": "2026-06-06T05:30:00Z"
}
```

### Implementation suggestions / specifics

- Perform `POST /join` before sending any Pi prompt.
- Rejoin idempotently on reconnect.
- Refuse to process bridge events until the join succeeds.

---

## 4. `PiRpcProcess`

### Prose Spec

The harness launches Pi as a subprocess in RPC mode and communicates with it over JSONL on stdin/stdout.

Pi RPC is the authoritative upstream control and event channel.

### Z Spec

```text
PiRpcProcess
  pid: ℕ
  command: seq CHAR
  args: seq CHAR
  mode: seq CHAR
  alive: 𝔹
where
  pid > 0
  command ≠ ⟨⟩
  mode = "rpc"
```

### Data examples

```json
{
  "pid": 44123,
  "command": "pi",
  "args": ["--mode", "rpc"],
  "mode": "rpc",
  "alive": true
}
```

### Implementation suggestions / specifics

- Use plain pipes for RPC stdin/stdout.
- Use strict LF-delimited JSONL framing.
- Treat RPC parse failure as a protocol/process error.
- Do not depend on PTY output for normal control flow.

---

## 5. `PiRpcSessionState`

### Prose Spec

The harness should track Pi session state so it can preserve continuity across multiple Discord interactions.

The harness should treat Pi's reported session identifiers as authoritative.

### Z Spec

```text
PiRpcSessionState
  agentId: seq CHAR
  cwd: seq CHAR
  sessionId?: seq CHAR
  sessionFile?: seq CHAR
  sessionName?: seq CHAR
  isStreaming: 𝔹
  startedAt: seq CHAR
where
  agentId ≠ ⟨⟩
  cwd ≠ ⟨⟩
  startedAt ≠ ⟨⟩
```

### Data examples

```json
{
  "agentId": "main",
  "cwd": "/home/easter/omicron",
  "sessionId": "019ea1f0-1234-7890-abcd-ef1234567890",
  "sessionFile": "/home/easter/.pi/agent/sessions/--home-easter-omicron--/2026-06-06T05-31-02-111Z_019ea1f0-1234-7890-abcd-ef1234567890.jsonl",
  "sessionName": "discord main",
  "isStreaming": false,
  "startedAt": "2026-06-06T05:31:02Z"
}
```

### Implementation suggestions / specifics

- Read Pi state through `get_state`.
- Preserve `sessionId` and `sessionFile` for logging and continuity.
- Reuse one Pi session for a bound Discord agent unless explicitly reset.

---

## 6. `BridgeEventPollLoop`

### Prose Spec

The harness polls the Discord bridge for inbound work assigned to its bound agent id.

Each inbound event becomes harness-owned work that is then translated into Pi RPC commands.

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
  "queueDepth": 2
}
```

### Implementation suggestions / specifics

- Poll `GET /agents/{agentId}/events`.
- Deduplicate as needed on the harness side.
- Keep bridge event handling outside Pi prompts.

---

## 7. `DiscordToPiRpcMapping`

### Prose Spec

The harness maps Discord-originated work into Pi RPC commands.

The mapping is owned by the harness and must be deterministic.

### Z Spec

```text
DiscordToPiRpcMapping
  eventId: seq CHAR
  command: seq CHAR
  message: seq CHAR
  streamingBehavior?: seq CHAR
where
  eventId ≠ ⟨⟩
  command ∈ {"prompt", "steer", "follow_up", "abort"}
  message ≠ ⟨⟩ ∨ command = "abort"
```

### Data examples

```json
{
  "eventId": "evt_123",
  "command": "prompt",
  "message": "[discord-bridge]\nAuthor: easter\nMessage: summarize the latest log"
}
```

### Implementation suggestions / specifics

- Default idle inbound work to `prompt`.
- Default busy inbound work to `steer` for the first implementation.
- Keep injected prompt formatting stable and minimal.
- Make bridge origin explicit in the message wrapper.

---

## 8. `PiRpcEventProjection`

### Prose Spec

The harness consumes Pi RPC events and projects them into a smaller internal event model suitable for downstream bridge behavior.

Pi RPC is the authoritative source for assistant output, queue state, tool lifecycle, and thinking visibility.

### Z Spec

```text
PiRpcEventProjection
  source: seq CHAR
  mappedType: seq CHAR
  timestamp: seq CHAR
  payload: seq CHAR
where
  source = "pi-rpc"
  mappedType ≠ ⟨⟩
  timestamp ≠ ⟨⟩
```

### Data examples

```json
{
  "source": "pi-rpc",
  "mappedType": "assistant_message_complete",
  "timestamp": "2026-06-06T05:31:30Z",
  "payload": "{\"text\":\"Here is the summary.\"}"
}
```

### Implementation suggestions / specifics

- Map Pi events such as:
  - `agent_start`
  - `agent_end`
  - `message_update`
  - `message_end`
  - `tool_execution_start`
  - `tool_execution_end`
  - `queue_update`
- Preserve an unknown-event fallback instead of failing hard.
- Treat Pi RPC, not logs or PTY, as authoritative for normal flow.

---

## 9. `BridgeStatusProjection`

### Prose Spec

While Pi is working, the harness reports progress back to the Discord bridge so the bridge can update reactions and status indicators.

The harness is the status publisher.

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

- Publish started/in-progress/completed/error transitions.
- Use bridge-approved reactions only.
- Keep status updates deterministic and separate from model prose.

---

## 10. `BridgeCompletionDelivery`

### Prose Spec

When Pi finishes a bridge-originated turn, the harness sends the final response back through the Discord bridge.

Pi RPC-derived assistant output is the authoritative completion source.

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
- Send completion only once per bridge event.
- Prefer the final assistant text from Pi RPC state/events.
- Use a fallback reply only if Pi completes without extractable final text.

---

## 11. `PiThinkingCapture`

### Prose Spec

The harness should capture Pi thinking events when the provider/model exposes them.

Thinking capture is internal observability by default and need not be shown to Discord users in the first implementation.

### Z Spec

```text
PiThinkingCapture
  visibleToHarness: 𝔹
  visibleToDiscord: 𝔹
  source: seq CHAR
where
  visibleToHarness = true
  source = "pi-rpc"
```

### Data examples

```json
{
  "visibleToHarness": true,
  "visibleToDiscord": false,
  "source": "pi-rpc"
}
```

### Implementation suggestions / specifics

- Ingest `thinking_start`, `thinking_delta`, and `thinking_end` when present.
- Do not assume every provider/model exposes visible thinking.
- Keep internal logs or event trails for future use.

---

## 12. `PiToolLifecycleCapture`

### Prose Spec

The harness should capture Pi tool execution lifecycle events for observability and debugging.

Tool execution should not be sent to Discord users by default in the first implementation.

### Z Spec

```text
PiToolLifecycleCapture
  visibleToHarness: 𝔹
  visibleToDiscord: 𝔹
  source: seq CHAR
where
  visibleToHarness = true
  visibleToDiscord = false
  source = "pi-rpc"
```

### Data examples

```json
{
  "visibleToHarness": true,
  "visibleToDiscord": false,
  "source": "pi-rpc"
}
```

### Implementation suggestions / specifics

- Capture:
  - `tool_execution_start`
  - `tool_execution_update`
  - `tool_execution_end`
- Use these for internal state, debugging, and future UI work.
- Keep bridge output minimal for the first implementation.

---

## 13. `HarnessAuditState`

### Prose Spec

The harness should keep enough local state to avoid duplicate replies and to debug session continuity issues.

### Z Spec

```text
HarnessAuditState
  processedEventIds: ℙ seq CHAR
  activeMessageId?: seq CHAR
  lastJoinAt?: seq CHAR
  piAlive: 𝔹
  sessionId?: seq CHAR
where
  piAlive ∈ {true, false}
```

### Data examples

```json
{
  "processedEventIds": ["evt_123", "evt_124"],
  "activeMessageId": "discord_msg_1",
  "lastJoinAt": "2026-06-06T05:30:00Z",
  "piAlive": true,
  "sessionId": "019ea1f0-1234-7890-abcd-ef1234567890"
}
```

### Implementation suggestions / specifics

- Persist processed bridge event ids locally.
- Record Pi `sessionId` and `sessionFile` when available.
- Log raw RPC lines on parse failure.
- Distinguish bridge state from Pi session state.

---

## 14. `PiRpcDiscordHarnessInvariants`

### Prose Spec

- The harness joins the Discord bridge before handling Discord work.
- Pi is launched only through `--mode rpc`.
- Pi RPC is the authoritative upstream control and event path.
- The Discord bridge is the only downstream output target.
- Session continuity is preserved through Pi session state when enabled.
- Thinking and tool events are captured internally even if not shown to Discord users.
- Duplicate completion should be prevented by harness and bridge state.
- Session logs and PTY output may exist for debugging or fallback, but they are not the primary architecture.

### Z Spec

```text
PiRpcDiscordHarnessInvariants
  bridgeFirst: 𝔹
  rpcRequired: 𝔹
  piOnlyUpstream: 𝔹
  discordOnlyDownstream: 𝔹
where
  bridgeFirst = true
  rpcRequired = true
  piOnlyUpstream = true
  discordOnlyDownstream = true
```

---

## Summary

This design makes Pi RPC the single high-trust upstream and the Discord bridge the single downstream transport.

The harness owns:

- Pi RPC process lifecycle
- bridge join and polling
- Pi session continuity
- deterministic Discord-to-Pi command mapping
- status projection
- final completion delivery
- internal thinking/tool observability

Pi provides the agent semantics.

The Discord bridge provides the user-facing transport.

The harness is the stable control layer between them.
