# Chat Log Reader Research

## Goal

Build a reusable component for a harness that spawns an agent in a PTY and can:

- detect which native log/session files belong to that spawned agent
- tail them in real time
- extract useful structured output from them
- fall back to PTY capture when native logs are missing or insufficient

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

## Bottom line

Yes, all three are usable as real-time sources, but in different ways:

- `Pi`: best option. Native JSONL session files plus explicit `--mode json` and `--mode rpc` streaming interfaces.
- `Claude`: local JSONL session/project logs appear to exist and are tail-able, but the format is less stable-looking and should be treated as reverse-engineered.
- `Codex`: local JSONL session logs clearly exist and are rich; there are also auxiliary SQLite/history files.

If you control launch, prefer this order:

1. native machine interface (`rpc`, `json`, SSE, websocket)
2. native session log tailing
3. PTY stdout capture
4. cleaned PTY transcript heuristics

## What exists today

### Pi

Evidence in repo:

- Pi stores sessions as JSONL under `~/.pi/agent/sessions/` per `packages/coding-agent/README.md` and `packages/coding-agent/docs/session-format.md`.
- Session files are append-only JSONL with a `session` header followed by `message`, `compaction`, `branch_summary`, etc.
- `research/discord-bridge-client/pi_logs.go` already implements a working detector:
  - walks the Pi session root
  - filters `.jsonl`
  - matches the first line header `{"type":"session","cwd":"..."}` against the launched cwd
  - picks the most recent matching file
  - tails new bytes and extracts latest assistant text
- Pi also exposes better-than-PTY interfaces:
  - `pi --mode json` streams JSON events to stdout
  - `pi --mode rpc` provides bidirectional JSONL control/events
  - SDK embedding exists and avoids PTY entirely

Important Pi caveat from `packages/coding-agent/src/core/session-manager.ts`:

- the session file may not be flushed to disk until the first assistant message exists
- so detection must handle "no file yet" for some time after launch

This exactly matches the prototype logic in `pi_logs.go`, which polls until the file appears.

### Claude

Observed locally:

- `~/.claude/projects/-home-easter-tomhyndman-site/a4d82999-71bc-447b-97b3-1167fe94ff93.jsonl`
- `~/.claude/projects/-tmp-basicwebtheme-jinja/22934c4b-2ccf-41b8-b322-8f9db5898e47.jsonl`
- `~/.claude/projects/-home-easter--openclaw-workspace/a51cd066-8c0d-4e3d-8809-f5baaa216a22.jsonl`

Observed format characteristics:

- append-only JSONL
- includes entries like:
  - `queue-operation`
  - `user`
  - `assistant`
  - `ai-title`
  - `attachment`
- assistant entries contain `message.role == "assistant"` and `message.content[]`
- content items may be:
  - `text`
  - `thinking`
  - `tool_use`
- tool results appear as later `user` entries containing `tool_result` payloads

Practical conclusion:

- Claude appears to have real-time session/project logs on disk
- the most useful discovery path is probably `~/.claude/projects/<cwd-encoded>/<session-id>.jsonl`
- `~/.claude/sessions/` exists here but is empty; in this installation, `projects/` looks like the important store
- schema should be treated as implementation detail, not stable contract

### Codex

Observed locally:

- session JSONL files under `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`
- coarse prompt history in `~/.codex/history.jsonl`
- additional internal state/logging in:
  - `~/.codex/logs_2.sqlite`
  - `~/.codex/state_5.sqlite`
  - `~/.codex/log/codex-tui.log`

Observed session JSONL characteristics:

- clearly append-only JSONL
- includes rich event types:
  - `session_meta`
  - `event_msg`
  - `response_item`
  - `turn_context`
- contains user prompts, developer/system context, task lifecycle events, and response items
- much richer than `history.jsonl`

Practical conclusion:

- Codex definitely has real-time session logs on disk
- `~/.codex/sessions/.../rollout-*.jsonl` is the primary target
- `history.jsonl` is useful only as a secondary index, not as the real stream
- SQLite files may be useful for future indexing, but JSONL is enough for a first reusable component

## Recommended reusable component

## Shape

Build a `chatlog-reader` library with four layers:

1. `AgentDetector`
- identifies agent kind from command, argv, env, cwd, and known home dirs

2. `SessionLocator`
- finds candidate session/log files for that agent instance
- correlates by launch time, cwd, and optional session id

3. `SessionTailer`
- tails bytes safely from a growing file
- handles partial lines, rotation, truncation, delayed file creation

4. `EventParser`
- converts raw JSONL lines into normalized events

Normalized event model:

```text
SessionStarted
TurnStarted
AssistantTextDelta
AssistantMessageComplete
ToolCallStarted
ToolCallFinished
UserMessage
Status
SessionFinished
RawLine
```

Keep a `RawLine` escape hatch so unknown schema changes do not break ingestion.

## Source preference policy

For each agent, define ordered sources:

### Pi

1. `--mode rpc`
2. `--mode json`
3. native session JSONL
4. PTY transcript

### Claude

1. any official machine-readable mode if you launch with one
2. native JSONL in `~/.claude/projects/`
3. PTY transcript

### Codex

1. any official machine-readable mode if available in your launch mode
2. native JSONL in `~/.codex/sessions/`
3. PTY transcript
4. optional SQLite inspection later

## Detection strategy

Use a layered detector:

1. explicit config wins
- agent type
- session dir override
- session id override

2. launch metadata
- executable basename: `pi`, `claude`, `codex`
- argv flags
- cwd
- launch timestamp

3. known home roots
- Pi: `~/.pi/agent/sessions` or configured session dir
- Claude: `~/.claude/projects`
- Codex: `~/.codex/sessions`

4. schema verification
- read first line or first few lines
- verify expected JSON keys before binding

5. time window filter
- only consider files modified within a configurable window around launch time

6. cwd correlation
- Pi: exact header `cwd`
- Claude: entry `cwd` fields and/or cwd-encoded project dir name
- Codex: `session_meta.payload.cwd` or `turn_context.payload.cwd`

## Tailing requirements

The tailer should:

- keep `(device, inode, offset)` state
- buffer partial lines across reads
- reopen if file is rotated or truncated
- poll at short interval if fs events are unreliable
- support "file not created yet"
- tolerate malformed lines and continue

JSONL tailing is enough for all three observed systems.

## Agent-specific parsing notes

### Pi parser

Parse:

- header: `type == "session"`
- conversation entries: `type == "message"`
- assistant text from `message.role == "assistant"`
- text blocks from `message.content[]` with `type == "text"`

The existing `parsePiAssistantLine()` and `extractPiAssistantReply()` in `research/discord-bridge-client/pi_logs.go` are a good starting point.

### Claude parser

Parse:

- `type == "assistant"` entries for assistant output
- assistant text from `message.content[]` items with `type == "text"`
- optional commentary/progress from earlier assistant text entries
- tool lifecycle from `tool_use` and later `tool_result`

Do not assume every assistant message is final on first appearance; treat the file as an event log.

### Codex parser

Parse:

- `type == "response_item"` with assistant/user payloads
- `type == "event_msg"` for task lifecycle
- `type == "session_meta"` and `turn_context` for correlation metadata

Codex logs are event-richer than Pi/Claude, so normalization matters.

## PTY fallback recommendations

If you must rely on PTY output, use these techniques.

### 1. Capture raw and cleaned streams separately

Store:

- raw bytes for forensics
- cleaned text for heuristics

The existing `cleanPTYTranscriptChunk()` in `research/discord-bridge-client/pty_agent.go` is a reasonable baseline:

- strip `\r`
- strip ANSI CSI/OSC escapes

### 2. Do not treat idle as correctness

Idle timeout is only a fallback. It is not a reliable semantic "turn finished" signal.

### 3. Add your own transcript logs

Even if native logs exist, keep harness-owned:

- PTY input log
- PTY output log
- launch metadata log

This is already done in `pty_agent.go`.

### 4. Prefer non-TUI/headless modes when available

PTY + full-screen TUI is the hardest case.

If the agent supports a headless mode, use it.

For Pi, `--mode json` or `--mode rpc` is far better than PTY scraping.

### 5. Correlate PTY and file logs

When both exist, bind them together with:

- launch time
- cwd
- command line
- optional session id

Then trust native log semantics and use PTY only for:

- early boot text
- crashes before session file creation
- debugging mismatches

### 6. Consider sentinel strategies

If you own the interaction protocol, inject recognizable markers around user prompts or turns. This helps align PTY text with native log events. Only do this if it does not pollute the user-visible workflow.

## Reliability ranking

Best to worst:

1. embedded SDK/API integration
2. explicit RPC/JSON event stream
3. native session JSONL tailing
4. native auxiliary DB/log inspection
5. PTY stdout capture
6. PTY cleaned-text inference

## Suggested first implementation

### v1

Implement:

- `PiReader`
- `ClaudeReader`
- `CodexReader`
- shared JSONL tailer
- normalized event bus

Use simple polling rather than filesystem notifications.

### v1 heuristics

- Pi: match most recent session file by cwd + modtime
- Claude: match newest `~/.claude/projects/<cwd-encoded>/*.jsonl`, then verify inner `cwd`
- Codex: match newest `~/.codex/sessions/**/rollout-*.jsonl`, then verify `session_meta.payload.cwd`

### v2

Add:

- SQLite index readers for Codex if needed
- file rotation handling tests
- replay mode from saved transcripts
- confidence scoring per detected source

## Design recommendation

The reusable component should not be "a PTY parser". It should be a `multi-source session observer` with PTY as the last fallback.

That architecture fits the evidence:

- Pi already has first-class structured streams and session files
- Claude appears to persist rich JSONL project/session logs
- Codex clearly persists rich JSONL rollout logs and additional local state

The right abstraction is:

- detect agent
- locate native source
- tail and normalize
- fall back to PTY only when necessary

## Most useful repo artifacts for implementation

- `research/discord-bridge-client/pi_logs.go`
- `research/discord-bridge-client/pty_agent.go`
- `packages/coding-agent/docs/session-format.md`
- `packages/coding-agent/docs/json.md`
- `packages/coding-agent/docs/rpc.md`
- local samples in `~/.claude/projects/`
- local samples in `~/.codex/sessions/`
