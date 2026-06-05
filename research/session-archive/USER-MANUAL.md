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

Pi ships with defaults, so the user can start with no special archive config.

At launch Pi:
1. reads archive defaults
2. applies any user overrides
3. validates the resolved settings
4. creates the archive directory if it does not exist
5. creates a session envelope
6. installs runtime hooks
7. opens the session archive file
8. appends events as the session runs

Each line in the archive is one JSON object.

## Files in this research set

- `SPEC.md` — runtime contract for the Pi feature
- `IMPLEMENTATION-PLAN.md` — build plan for Pi code changes
- `USER-MANUAL.md` — how to use the feature once built
- `TICKET.md` — coding-agent prompt for implementation

## How to start a session

### Proposed flow

1. Start Pi normally.
2. Pi uses its built-in archive defaults unless the user overrides them.
3. Pi initializes the archive plugin before the model session begins.
4. Use Pi as usual.
5. Stop the session cleanly when finished.

### Example

```bash
pi
```

To override the defaults, point Pi at a config file or equivalent environment setting.

If the archive feature is configured as fail-closed and startup wiring cannot be completed, Pi should refuse to start.

## What you will see

On startup, Pi should create or announce the session archive file path.

Example:

```text
/home/easter/.pi/agent/session-archive/2026/06/05/2026-06-05T19-45-12Z_8f3a.jsonl
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

- If the resolved archive config is invalid, startup fails.
- If archive mode is enabled, logging is mandatory.
- The model does not decide whether logging happens.
- Pi runtime and the archive plugin enforce it.
- The archive directory is created automatically if it is missing.

## Minimal example

```bash
pi
```

Then run a normal Pi session. The archive plugin should capture the session automatically using defaults.
