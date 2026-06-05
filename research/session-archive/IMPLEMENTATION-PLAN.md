# Pi Session Archive Feature Implementation Plan

## Goal

Add a startup-loaded session archive feature to Pi that records each session as append-only JSONL in a separate archive repository.

The feature must be enforced by Pi runtime code, not by LLM prompting.

---

## Plan by spec component

### 1. `SessionArchiveConfig`

**Build target**
- startup config loader
- strict validation
- fail-closed boot behavior

**Implementation steps**
- Add a small archive config format, JSON first.
- Load config before the Pi agent loop starts.
- Validate required keys:
  - `enabled`
  - `repoPath`
  - `fileLayout`
  - `outputFormat=jsonl`
  - `failClosed=true`
- Allow env-var overrides only for sensitive paths if needed.
- Refuse to start if archiving is enabled but config is missing or invalid.

**Done when**
- Pi will not start a session without a valid archive config when archiving is enabled.

---

### 2. `PiSessionEnvelope`

**Build target**
- runtime session identity and metadata

**Implementation steps**
- Generate `sessionId` at startup using ISO timestamp + short random suffix.
- Capture `startedAt`, `source`, `host`, `runtimeVersion`, `packageVersion`, `mode`, and `cwd`.
- Store the envelope in agent runtime state.
- Emit a `session_start` record as the first archive line.

**Done when**
- every Pi session has a stable ID before the first user turn.

---

### 3. `PiArchiveEvent`

**Build target**
- unified JSONL event record

**Implementation steps**
- Define one event schema for:
  - `session_start`
  - `message`
  - `tool_call`
  - `tool_result`
  - `session_end`
  - `error`
- Normalize all Pi runtime activity into that shape.
- Write one JSON object per line.
- Keep `metadata` compact and structured.
- Validate every event before write.

**Done when**
- all session activity can be captured as valid JSONL records.

---

### 4. `PiArchiveWriter`

**Build target**
- append-only file writer

**Implementation steps**
- Resolve a session file path from `fileLayout`.
- Create the archive path outside the Pi code tree.
- Open files in append mode only.
- Write each validated event immediately.
- Flush after every write or tiny batch.
- Append `session_end` on clean shutdown.
- Do not create any update or rewrite path.

**Done when**
- the archive survives crashes with minimal loss.

---

### 5. `PiArchiveEnforcement`

**Build target**
- runtime guarantees that bypass the LLM entirely

**Implementation steps**
- Register the archive plugin during Pi startup.
- Hook archive writes into session lifecycle, user input, tool use, model output, and shutdown.
- Make logging synchronous or strongly ordered.
- Validate records before storage.
- Block startup if archive mode is enabled but plugin wiring fails.
- Keep archive decisions outside prompt text and model output.

**Done when**
- archiving still happens even if the model ignores it.

---

## Proposed Pi system shape

### Runtime layers

1. **CLI/bootstrap layer**
   - loads archive config
   - creates the session envelope
   - installs hooks before the model loop starts

2. **Agent runtime**
   - emits user, assistant, tool, and lifecycle events

3. **Archive plugin**
   - validates and writes JSONL
   - enforces append-only behavior

4. **Archive repository sync**
   - later git commit/push process, outside the runtime path

### Enforced boundaries

- The LLM may generate messages.
- The LLM may not decide whether logging happens.
- Pi runtime owns archive state.
- The archive plugin owns persistence.

---

## Suggested build order

### Phase 1 — config + envelope
- archive config loader
- session ID generation
- startup validation
- session_start event

### Phase 2 — event capture
- normalize lifecycle and tool events
- JSONL event schema
- append-only writer

### Phase 3 — enforcement
- plugin registration
- hook wiring
- startup failure modes
- schema validation

### Phase 4 — hardening
- redaction rules
- crash recovery
- manifest/index files
- repo sync process

---

## Recommended first milestone

Pi starts a session, creates a session archive file, logs every inbound and outbound event, and closes with `session_end` while the archive remains entirely outside the LLM path.

---

## Follow-up work

- add a daily manifest/index
- add optional redaction rules
- add replay tooling
- add tests for startup failure, append-only behavior, and hook enforcement
- add a repo sync path for publishing the archive repo
