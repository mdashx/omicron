# Agent RPC Harness Spec

## Intent

Add a harness that sits between a single RPC-capable upstream agent process and a single simple local CLI downstream consumer.

The harness should launch the upstream agent through its native RPC interface, maintain session continuity, consume structured RPC events, and project useful status and final replies to the CLI.

This spec intentionally describes the architecture at a conceptual level rather than binding it to a named upstream product or a named downstream transport.

The narrowed scope still applies:

- one upstream RPC agent only
- one simple local CLI downstream only

This must be enforced by harness runtime wiring, not by prompting the model to remember transport rules.

---

## 1. `AgentRpcHarness`

### Prose Spec

The harness is a long-lived local supervisor process.

It launches the upstream agent in RPC mode, owns the agent process lifecycle, tracks session state, and mediates all communication between the upstream RPC interface and the downstream CLI.

### Z Spec

```text
AgentRpcHarness
  enabled: 𝔹
  agentId: seq CHAR
  cwd: seq CHAR
  upstream: seq CHAR
  downstream: seq CHAR
where
  enabled = true
  agentId ≠ ⟨⟩
  cwd ≠ ⟨⟩
  upstream = "rpc-agent"
  downstream = "simple-cli"
```

### Data examples

```json
{
  "enabled": true,
  "agentId": "main",
  "cwd": "/home/easter/omicron",
  "upstream": "rpc-agent",
  "downstream": "simple-cli"
}
```

### Implementation suggestions / specifics

- Start as a local process or service.
- Treat the upstream RPC interface as authoritative.
- Treat the CLI as the only downstream consumer in the first implementation.
- Keep transport ownership in harness code, not in the prompt path.

---

## 2. `AgentRpcHarnessConfig`

### Prose Spec

The harness should have built-in defaults and support config or environment overrides.

It should resolve upstream launch arguments, working directory, and session persistence behavior, plus basic CLI behavior.

### Z Spec

```text
AgentRpcHarnessConfig
  enabled?: 𝔹
  agentId?: seq CHAR
  command?: seq CHAR
  args?: seq CHAR
  cwd?: seq CHAR
  sessionDir?: seq CHAR
  noSession?: 𝔹
  idleTimeoutMs?: ℕ
  debug?: 𝔹
```

### Data examples

```json
{
  "agentId": "main",
  "command": "agent-command",
  "args": ["--mode", "rpc"],
  "cwd": "/home/easter/omicron",
  "idleTimeoutMs": 2500,
  "debug": true
}
```

### Implementation suggestions / specifics

- Default upstream launch to RPC mode.
- Keep session persistence configurable through normal upstream flags.
- Allow environment variable overrides for development.
- Keep downstream CLI concerns minimal.

---

## 3. `UpstreamRpcProcess`

### Prose Spec

The harness launches the upstream agent as a subprocess in RPC mode and communicates with it over JSONL on stdin/stdout.

The upstream RPC channel is the authoritative control and event path.

### Z Spec

```text
UpstreamRpcProcess
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
  "command": "agent-command",
  "args": ["--mode", "rpc"],
  "mode": "rpc",
  "alive": true
}
```

### Implementation suggestions / specifics

- Use plain pipes for RPC stdin/stdout.
- Use strict LF-delimited JSONL framing.
- Treat parse failure as a protocol/process error.
- Do not depend on PTY output for normal control flow.

---

## 4. `UpstreamSessionState`

### Prose Spec

The harness should track upstream session state so it can preserve continuity across multiple downstream requests.

The harness should treat upstream-reported session identifiers as authoritative when they exist.

### Z Spec

```text
UpstreamSessionState
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
  "sessionFile": "/tmp/session.jsonl",
  "sessionName": "main session",
  "isStreaming": false,
  "startedAt": "2026-06-06T05:31:02Z"
}
```

### Implementation suggestions / specifics

- Read upstream state from the RPC interface where possible.
- Preserve `sessionId` and `sessionFile` for logging and continuity.
- Reuse one upstream session for a single downstream conversation unless explicitly reset.

---

## 5. `DownstreamCliRequest`

### Prose Spec

The harness accepts simple local CLI-originated requests and maps them to upstream RPC commands.

The mapping is owned by the harness and must be deterministic.

### Z Spec

```text
DownstreamCliRequest
  requestId: seq CHAR
  command: seq CHAR
  message: seq CHAR
  streamingBehavior?: seq CHAR
where
  requestId ≠ ⟨⟩
  command ∈ {"prompt", "steer", "follow_up", "abort"}
  message ≠ ⟨⟩ ∨ command = "abort"
```

### Data examples

```json
{
  "requestId": "req_123",
  "command": "prompt",
  "message": "summarize the latest log"
}
```

### Implementation suggestions / specifics

- Default idle inbound work to `prompt`.
- Default busy inbound work to `steer` for the first implementation.
- Keep request formatting stable and minimal.
- Preserve a request id for completion matching and logging.

---

## 6. `UpstreamRpcEventProjection`

### Prose Spec

The harness consumes upstream RPC events and projects them into a smaller internal event model suitable for downstream CLI behavior.

The upstream RPC stream is the authoritative source for assistant output, queue state, tool lifecycle, and thinking visibility.

### Z Spec

```text
UpstreamRpcEventProjection
  source: seq CHAR
  mappedType: seq CHAR
  timestamp: seq CHAR
  payload: seq CHAR
where
  source = "rpc-agent"
  mappedType ≠ ⟨⟩
  timestamp ≠ ⟨⟩
```

### Data examples

```json
{
  "source": "rpc-agent",
  "mappedType": "assistant_message_complete",
  "timestamp": "2026-06-06T05:31:30Z",
  "payload": "{\"text\":\"Here is the summary.\"}"
}
```

### Implementation suggestions / specifics

- Map upstream events such as:
  - agent lifecycle
  - message streaming lifecycle
  - tool execution lifecycle
  - queue updates
- Preserve an unknown-event fallback instead of failing hard.
- Treat RPC, not logs or PTY, as authoritative for normal flow.

---

## 7. `CliStatusProjection`

### Prose Spec

While the upstream agent is working, the harness projects simple status information to the downstream CLI.

The harness is the status publisher.

### Z Spec

```text
CliStatusProjection
  requestId: seq CHAR
  phase: seq CHAR
  message: seq CHAR
where
  requestId ≠ ⟨⟩
  phase ≠ ⟨⟩
```

### Data examples

```json
{
  "requestId": "req_123",
  "phase": "thinking",
  "message": "working"
}
```

### Implementation suggestions / specifics

- Publish started/in-progress/completed/error transitions.
- Keep status messages deterministic and separate from model prose.
- Keep CLI output minimal for the first implementation.

---

## 8. `CliCompletionDelivery`

### Prose Spec

When the upstream agent finishes a downstream-originated turn, the harness sends the final response back to the CLI.

RPC-derived assistant output is the authoritative completion source.

### Z Spec

```text
CliCompletionDelivery
  requestId: seq CHAR
  content: seq CHAR
  completedAt: seq CHAR
where
  requestId ≠ ⟨⟩
  completedAt ≠ ⟨⟩
```

### Data examples

```json
{
  "requestId": "req_123",
  "content": "Here is the summary.",
  "completedAt": "2026-06-06T05:31:30Z"
}
```

### Implementation suggestions / specifics

- Send completion only once per request.
- Prefer the final assistant text from upstream RPC state/events.
- Use a fallback reply only if the upstream finishes without extractable final text.

---

## 9. `ThinkingCapture`

### Prose Spec

The harness should capture thinking events when the upstream provider/model exposes them.

Thinking capture is internal observability by default and need not be shown to CLI users in the first implementation.

### Z Spec

```text
ThinkingCapture
  visibleToHarness: 𝔹
  visibleToCli: 𝔹
  source: seq CHAR
where
  visibleToHarness = true
  source = "rpc-agent"
```

### Data examples

```json
{
  "visibleToHarness": true,
  "visibleToCli": false,
  "source": "rpc-agent"
}
```

### Implementation suggestions / specifics

- Ingest thinking lifecycle events when present.
- Do not assume every provider/model exposes visible thinking.
- Keep internal logs or event trails for future use.

---

## 10. `ToolLifecycleCapture`

### Prose Spec

The harness should capture tool execution lifecycle events for observability and debugging.

Tool execution should not be sent to CLI users by default in the first implementation.

### Z Spec

```text
ToolLifecycleCapture
  visibleToHarness: 𝔹
  visibleToCli: 𝔹
  source: seq CHAR
where
  visibleToHarness = true
  visibleToCli = false
  source = "rpc-agent"
```

### Data examples

```json
{
  "visibleToHarness": true,
  "visibleToCli": false,
  "source": "rpc-agent"
}
```

### Implementation suggestions / specifics

- Capture tool execution start, update, and end.
- Use these for internal state, debugging, and future UI work.
- Keep CLI output minimal for the first implementation.

---

## 11. `HarnessAuditState`

### Prose Spec

The harness should keep enough local state to avoid duplicate replies and to debug session continuity issues.

### Z Spec

```text
HarnessAuditState
  processedRequestIds: ℙ seq CHAR
  activeRequestId?: seq CHAR
  upstreamAlive: 𝔹
  sessionId?: seq CHAR
where
  upstreamAlive ∈ {true, false}
```

### Data examples

```json
{
  "processedRequestIds": ["req_123", "req_124"],
  "activeRequestId": "req_123",
  "upstreamAlive": true,
  "sessionId": "019ea1f0-1234-7890-abcd-ef1234567890"
}
```

### Implementation suggestions / specifics

- Persist processed request ids locally if needed.
- Record session identifiers when available.
- Log raw RPC lines on parse failure.
- Distinguish downstream request state from upstream session state.

---

## 12. `AgentRpcHarnessInvariants`

### Prose Spec

- The upstream agent is launched only through its RPC interface.
- The RPC interface is the authoritative upstream control and event path.
- The simple local CLI is the only downstream output target.
- Session continuity is preserved through upstream session state when enabled.
- Thinking and tool events are captured internally even if not shown to CLI users.
- Duplicate completion should be prevented by harness state.
- Session logs and PTY output may exist for debugging or fallback, but they are not the primary architecture.

### Z Spec

```text
AgentRpcHarnessInvariants
  rpcRequired: 𝔹
  singleUpstream: 𝔹
  singleDownstream: 𝔹
where
  rpcRequired = true
  singleUpstream = true
  singleDownstream = true
```

---

## Summary

This design makes one RPC-capable agent process the single high-trust upstream and one simple local CLI the single downstream transport.

The harness owns:

- upstream RPC process lifecycle
- session continuity
- deterministic request mapping
- status projection
- final completion delivery
- internal thinking/tool observability

The upstream agent provides the agent semantics.

The downstream CLI provides the user-facing interaction surface.

The harness is the stable control layer between them.
