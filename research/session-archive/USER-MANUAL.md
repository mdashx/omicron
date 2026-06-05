# Pi Session Archive Feature User Manual

## What it does

When enabled, Pi records each session as an append-only JSONL file in a separate archive repository.

It:
- creates a timestamped session ID at startup
- writes a `session_start` record
- logs user turns, tool calls, tool results, and assistant output as archive events
- writes `session_end` when the session closes
- keeps the archive logic outside the LLM path

## How it works

The archive feature is loaded at Pi startup.

At launch Pi:
1. reads archive config
2. validates archive settings
3. creates a session envelope
4. installs runtime hooks
5. opens the session archive file
6. appends events as the session runs

Each line in the archive is one JSON object.

## Files in this research set

- `SPEC.md` — runtime contract for the Pi feature
- `IMPLEMENTATION-PLAN.md` — build plan for Pi code changes
- `USER-MANUAL.md` — how to use the feature once built

## How to start a session

### Proposed flow

1. Create or point Pi at an archive config.
2. Start Pi normally.
3. Pi initializes the archive plugin before the model session begins.
4. Use Pi as usual.
5. Stop the session cleanly when finished.

### Example

```bash
export PI_SESSION_ARCHIVE_CONFIG_PATH=/home/easter/session-archive-repo/config.json
pi
```

If the archive feature is configured as fail-closed, Pi should refuse to start when the archive plugin cannot be loaded.

## What you will see

On startup, Pi should create or announce the session archive file path.

Example:

```text
/home/easter/session-archive-repo/2026/06/05/2026-06-05T19-45-12Z_8f3a.jsonl
```

## What gets written

Typical records:
- `session_start`
- `message`
- `tool_call`
- `tool_result`
- `session_end`

The file is append-only. Nothing is rewritten.

## Important behavior

- If archive config is invalid, startup fails.
- If archive mode is enabled, logging is mandatory.
- The model does not decide whether logging happens.
- Pi runtime and the archive plugin enforce it.

## Minimal example

```bash
export PI_SESSION_ARCHIVE_CONFIG_PATH=/home/easter/session-archive-repo/config.json
pi
```

Then run a normal Pi session. The archive plugin should capture the session automatically.
