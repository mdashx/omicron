# Agent RPC Research

## Goal

Build a reusable harness component that can launch agents through their native machine interfaces instead of depending primarily on PTY scraping.

Primary targets:

- `Pi` via `--mode rpc` and `--mode json`
- `Claude` via `--print --verbose --output-format stream-json`

Secondary targets:

- local session logs when native streaming interfaces are unavailable
- PTY capture as a final fallback

The larger objective is not just "read logs", but to create a reusable RPC/stream harness that can:

- connect agents to chatbots
- connect agents to Discord/Slack/CLI bridges
- connect agents to any other process that can speak the harness protocol
- preserve session identity and continuity where supported
- surface assistant text, tool calls, tool results, status, and thinking visibility when available

## Research inputs

This research used:

- `research/discord-bridge-client/pi_logs.go`
- `research/discord-bridge-client/pty_agent.go`
- `research/discord-bridge-client/README.md`
- `packages/coding-agent/README.md`
- `packages/coding-agent/docs/session-format.md`
- `packages/coding-agent/docs/json.md`
- `packages/coding-agent/docs/rpc.md`
- local Claude data under `~/.claude/`
- local Codex data under `~/.codex/`
- `https://github.com/Khan/format-claude-stream`

## Bottom line

The main direction should shift from log-tailing-first to RPC/stream-first.

Preferred source order when the harness controls launch:

1. native bidirectional RPC interface
2. native structured stdout event stream
3. native session log tailing
4. PTY stdout capture
5. cleaned PTY transcript heuristics

That means:

- `Pi`: use `--mode rpc` first
- `Claude`: use `--output-format stream-json` first
- logs remain useful for auditing, recovery, and fallback
- PTY remains useful mostly for TUI-only launches and debugging

## Main findings

### Pi is already close to the ideal harness target

Pi exposes:

- persisted JSONL sessions
- `--mode json` event streaming
- `--mode rpc` bidirectional JSONL control and event streaming
- an SDK for embedding without subprocesses

From `packages/coding-agent/docs/rpc.md`, Pi RPC supports:

- prompting
- steering/follow-up queues
- abort
- model changes
- thinking level changes
- session switching
- session stats
- HTML export
- extension UI requests/responses

Important harness implications:

- Pi RPC preserves session identity via `sessionId` and `sessionFile`
- Pi RPC is close to the normal agent behavior, but not the same as the TUI experience
- thinking can be streamed through `message_update` events when the provider/model exposes it
- TUI-only affordances must be reimplemented by the harness client if needed

This makes Pi the strongest candidate for a reusable agent harness backend.

### Claude appears to have a good structured stream interface

Evidence from `Khan/format-claude-stream`:

- Claude supports:
  - `claude --print --verbose --output-format stream-json`
- stream line types include:
  - `assistant`
  - `user`
  - `result`
  - `stream_event`
  - `system`
  - `rate_limit_event`
- assistant content can include:
  - `text`
  - `thinking`
  - `tool_use`
- user content can include:
  - `tool_result`

This is not bidirectional RPC in the same shape as Pi RPC, but it is a structured live stream and is much better than PTY scraping.

Practical conclusion:

- when launching Claude for harness use, prefer `stream-json`
- build a stream adapter that normalizes the event stream into the harness event model
- if bidirectional control is needed beyond one-shot prompt/print workflows, the harness may need its own request/response wrapper around Claude process launches

### Claude local logs still matter, but as secondary sources

Observed locally:

- `~/.claude/projects/-home-easter-tomhyndman-site/a4d82999-71bc-447b-97b3-1167fe94ff93.jsonl`
- `~/.claude/projects/-tmp-basicwebtheme-jinja/22934c4b-2ccf-41b8-b322-8f9db5898e47.jsonl`
- `~/.claude/projects/-home-easter--openclaw-workspace/a51cd066-8c0d-4e3d-8809-f5baaa216a22.jsonl`

Observed characteristics:

- append-only JSONL
- event-like records including:
  - `queue-operation`
  - `user`
  - `assistant`
  - `ai-title`
  - `attachment`
- assistant content includes:
  - `text`
  - `thinking`
  - `tool_use`
- tool results later appear in user entries as `tool_result`

Practical conclusion:

- Claude has useful local logs for replay and fallback
- but these should be treated as reverse-engineered implementation details
- the harness should prefer `stream-json` over file-tailing when launch is under our control

### Codex still looks log-first from current evidence

Observed locally:

- session JSONL files under `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`
- coarse prompt history in `~/.codex/history.jsonl`
- additional internal state/logging in:
  - `~/.codex/logs_2.sqlite`
  - `~/.codex/state_5.sqlite`
  - `~/.codex/log/codex-tui.log`

Observed session JSONL characteristics:

- append-only JSONL
- event types like:
  - `session_meta`
  - `event_msg`
  - `response_item`
  - `turn_context`
- rich metadata including cwd, task lifecycle, and assistant/user payloads

Practical conclusion:

- Codex definitely has usable real-time local logs
- JSONL tailing is enough for a first-pass harness adapter
- if Codex later exposes a stronger official RPC/stream interface, it should replace file-tailing in the priority order

## Pi RPC answers relevant harness questions

### Does Pi RPC maintain session identity?

Yes.

From the Pi docs and RPC behavior:

- session continuity exists in RPC mode
- state can expose:
  - `sessionId`
  - `sessionFile`
  - `sessionName`
- launch modes still support:
  - persisted sessions
  - `--no-session`
  - `--session <path|id>`
  - `--session-dir <path>`

So a harness can maintain session identity back and forth, as long as it stores and reuses the appropriate Pi session settings.

### Is the experience the same as normal Pi?

Mostly in agent behavior, not in presentation.

Same or close:

- same agent/session model
- same tools
- same persisted session format
- same message queue concepts
- same model/thinking/session controls

Not the same:

- no built-in TUI
- harness must render events itself
- extension UI requests must be handled by the harness if used
- TUI-specific affordances are not automatic

So the right framing is:

- Pi RPC gives nearly the same agent semantics
- Pi TUI gives a richer local UX
- the harness can reproduce much of that experience if it consumes the RPC event stream well

### Will Pi RPC report all thinking steps?

It will report thinking when the underlying provider/model exposes them.

Pi RPC emits `message_update` events that may include:

- `thinking_start`
- `thinking_delta`
- `thinking_end`
- text deltas
- tool call deltas

Caveat:

- thinking visibility depends on provider/model behavior and selected thinking level
- some providers summarize or omit thinking
- some models may provide none

So the harness should support thinking deltas, but must not assume they always exist.

## Recommended reusable component

Build `agent-rpc` as a reusable harness layer, not a one-off Pi adapter and not just a log reader.

## Core shape

1. `AgentLauncher`
- launches the agent in one of several modes:
  - rpc
  - structured stream
  - session-log mode
  - PTY fallback

2. `AgentAdapter`
- agent-specific knowledge for Pi, Claude, Codex
- knows launch flags, event parsing, session correlation, and capability detection

3. `SessionTracker`
- preserves session identity when available
- tracks session id, session file, cwd, launch time, and user-visible name

4. `EventNormalizer`
- converts native events into a common harness event model

5. `TransportBridge`
- connects normalized events to downstream systems:
  - chatbots
  - Discord bridges
  - Slack bridges
  - other CLIs
  - web services

6. `FallbackObserver`
- tails local logs or PTY transcripts when the native interface is absent or insufficient

## Normalized event model

```text
SessionStarted
SessionState
TurnStarted
AssistantTextDelta
AssistantThinkingDelta
AssistantMessageComplete
ToolCallStarted
ToolCallDelta
ToolCallFinished
ToolResult
UserMessage
Status
QueueUpdate
SessionFinished
RawLine
UnknownEvent
```

Keep `RawLine` and `UnknownEvent` escape hatches so schema drift does not break the harness.

## Adapter priority policy

### Pi

1. `pi --mode rpc`
2. `pi --mode json`
3. native session JSONL
4. PTY transcript

### Claude

1. `claude --print --verbose --output-format stream-json`
2. native JSONL in `~/.claude/projects/`
3. PTY transcript

### Codex

1. official stream/RPC mode if available in chosen launch path
2. native JSONL in `~/.codex/sessions/`
3. PTY transcript
4. optional SQLite inspection later

## Proposed agent adapters

### `PiRpcAdapter`

Responsibilities:

- spawn `pi --mode rpc`
- speak JSONL over stdin/stdout
- map Pi responses and events to normalized harness events
- preserve `sessionId` and `sessionFile`
- surface thinking deltas when present
- optionally handle Pi extension UI protocol

This should be the primary implementation target.

### `ClaudeStreamAdapter`

Responsibilities:

- spawn Claude with `--output-format stream-json`
- parse discriminated line types
- normalize:
  - assistant text
  - assistant thinking
  - tool use
  - tool result
- ignore or pass through less useful event types
- optionally write raw JSONL audit logs

This should be the second major implementation target.

### `PiLogAdapter`

Responsibilities:

- reuse the logic from `research/discord-bridge-client/pi_logs.go`
- locate the most recent matching Pi session file by cwd and launch time
- tail and extract assistant/tool events when RPC is not used

### `ClaudeLogAdapter`

Responsibilities:

- locate project log file in `~/.claude/projects/<cwd-encoded>/`
- verify inner cwd/session records
- tail append-only JSONL
- normalize assistant/tool events

### `PtyFallbackAdapter`

Responsibilities:

- capture raw PTY bytes
- capture cleaned PTY text
- provide weak fallback extraction heuristics
- remain debug-oriented, not semantic-primary

## Harness protocol recommendation

The harness itself should expose its own small stable protocol regardless of the upstream agent.

Suggested commands:

- `start_session`
- `resume_session`
- `send_user_message`
- `steer`
- `follow_up`
- `abort`
- `get_state`
- `list_sessions`
- `switch_model`
- `set_thinking`
- `stop_session`

Suggested outbound events:

- `session_started`
- `session_state`
- `assistant_text_delta`
- `assistant_thinking_delta`
- `assistant_message_complete`
- `tool_call_started`
- `tool_call_finished`
- `tool_result`
- `queue_update`
- `status`
- `session_finished`
- `error`

This lets downstream systems avoid caring whether the upstream source is Pi RPC, Claude stream-json, or a file tailer.

## Chatbot/process integration targets

The harness should be designed to sit between agents and other systems.

Examples:

- Discord bot receives a message, forwards it to `agent-rpc`, streams reply back to Discord
- Slack bot does the same
- CLI process uses harness stdin/stdout for headless local automation
- web service connects to harness over websocket/SSE and renders live assistant/tool/thinking output
- another orchestration process uses harness sessions as durable agent workers

This is more valuable than a log reader because it enables:

- interactive control
- session continuity
- live streaming UX
- multi-platform downstream integrations
- agent swapping behind one stable abstraction

## Log tailing still matters

Even with an RPC-first design, keep log support for:

- audit/replay
- crash recovery
- agents that only expose logs
- debugging mismatches between native interface and visible behavior

That means the older log-reader work is still useful, but it should become a fallback subsystem inside `agent-rpc`, not the primary abstraction.

## PTY fallback guidance

If PTY must be used:

1. capture raw and cleaned streams separately
2. never treat idle timeout as true semantic completion
3. keep harness-owned input/output transcript logs
4. correlate PTY with session ids, cwd, and launch time
5. trust native machine interfaces or native logs over PTY-derived guesses

The existing cleaning logic in `research/discord-bridge-client/pty_agent.go` remains a useful fallback baseline.

## Suggested implementation order

### v1

Build:

- `PiRpcAdapter`
- `ClaudeStreamAdapter`
- normalized event bus
- simple harness protocol
- one downstream bridge target, likely the existing Discord bridge path

### v2

Add:

- `PiLogAdapter`
- `ClaudeLogAdapter`
- persistent session registry in the harness
- replay/audit logging
- thinking-aware UI rendering in downstream bridges

### v3

Add:

- Codex adapter
- confidence scoring for multiple concurrent sources
- web transport support
- test fixtures for stream replay and session restoration

## Design recommendation

The right abstraction is no longer "chat log reader".

It is:

- an `agent-rpc` harness
- with native RPC/stream integrations first
- session-log observation second
- PTY parsing last

That architecture best fits the evidence:

- Pi already has a strong native RPC interface
- Claude already has a useful structured stream interface
- logs still provide durable audit and fallback value
- downstream consumers want a stable session and event protocol more than they want raw logs

## Most useful artifacts for implementation

- `packages/coding-agent/docs/rpc.md`
- `packages/coding-agent/docs/json.md`
- `packages/coding-agent/docs/session-format.md`
- `research/discord-bridge-client/pi_logs.go`
- `research/discord-bridge-client/pty_agent.go`
- `https://github.com/Khan/format-claude-stream`
- local Claude samples in `~/.claude/projects/`
- local Codex samples in `~/.codex/sessions/`
