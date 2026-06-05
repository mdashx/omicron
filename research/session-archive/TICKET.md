# Coding Agent Prompt: Pi Session Archive Feature

## Context

Implement the Pi session archive feature in the Pi codebase.

Research docs:
- `research/session-archive/SPEC.md`
- `research/session-archive/IMPLEMENTATION-PLAN.md`
- `research/session-archive/USER-MANUAL.md`

## Goal

Add a startup-loaded session archive feature to Pi so every session is captured as append-only JSONL in a separate archive repository, enforced by runtime hooks rather than LLM prompting.

## New requirement

Pi should ship with defaults for the archive feature.

The user should be able to start Pi with no special archive config and still get a working archive path. If the archive directory does not exist, create it recursively. Users can still override the defaults with config.

## What to optimize for

- correct startup wiring
- clear runtime boundaries
- fail-closed enforcement when archive wiring is invalid
- no LLM involvement in deciding whether logging happens
- minimal but durable defaults
- onboarding clarity for future readers without constraining the implementation too tightly

## Where to look first in the codebase

Start with these files and concepts:

- `packages/coding-agent/src/main.ts`
  - CLI/bootstrap entry point
  - best place to resolve defaults and wire startup behavior

- `packages/coding-agent/src/core/agent-session-runtime.ts`
  - runtime replacement/new-session flow
  - good place to ensure archive state follows session lifecycle changes

- `packages/coding-agent/src/core/agent-session-services.ts`
  - service creation boundary
  - good place to pass resolved archive config into session creation

- `packages/coding-agent/src/core/session-manager.ts`
  - session file model and durable session state
  - useful for understanding how Pi persists session data already

- `packages/coding-agent/src/core/extensions/types.ts`
  - session_start / session_shutdown extension events
  - useful if the implementation becomes an extension or plugin-style hook

- `packages/coding-agent/src/core/extensions/runner.ts`
  - extension lifecycle and event dispatch
  - useful if hooks need to fire at startup and shutdown

- `packages/agent/docs/hooks.md`
  - hook semantics and event flow

- `packages/agent/docs/durable-harness.md`
  - durable runtime/session design notes

- `packages/agent/docs/observability.md`
  - structured runtime event philosophy that can inform archive events too

- `packages/coding-agent/docs/session-format.md`
  - current JSONL session structure, useful as a reference point

## Suggested implementation shape

This is a suggested shape, not a prescription.

1. Resolve a built-in archive default config.
2. Merge user overrides onto the defaults.
3. Validate the resolved config.
4. Create the archive directory if missing.
5. Generate a session envelope (`sessionId`, timestamps, cwd, etc.).
6. Install runtime hooks or a plugin that writes JSONL.
7. Emit `session_start` before the model session begins.
8. Append `message`, `tool_call`, `tool_result`, `session_end`, and `error` records.
9. Fail closed if archive wiring is required but broken.

## Example code direction

The concrete implementation will likely need something like this shape:

```ts
const defaults = {
  enabled: true,
  repoPath: "~/.pi/agent/session-archive",
  fileLayout: "yyyy/mm/dd/sessionId.jsonl",
  outputFormat: "jsonl",
  captureEvents: ["session_start", "message", "tool_call", "tool_result", "session_end", "error"],
  redactMode: "minimal",
  failClosed: true,
};

const resolved = resolveArchiveConfig(defaults, userConfig);
await mkdir(resolved.repoPath, { recursive: true });
const archive = createArchiveWriter(resolved, envelope);
await archive.writeSessionStart();
```

A good implementation should still make its own judgment about the final API shape. This snippet is just orientation.

## What to change if the design wants it

If the cleanest solution is a core runtime feature rather than a traditional extension, choose the runtime boundary that makes startup enforcement clearest.

The important thing is not the packaging label. The important thing is:
- startup-loaded
- deterministic
- append-only
- no LLM involvement
- default configuration that works immediately
- directory creation handled automatically

## Acceptance criteria

- Pi starts with archive defaults and no custom config.
- The archive directory is created automatically if it does not exist.
- A session archive file is created at startup.
- The archive file contains JSONL records.
- `session_start` appears first and `session_end` appears on clean shutdown.
- Inbound and outbound runtime events are captured.
- The implementation is enforced by runtime code, not by prompt text.
- The implementation stays readable enough that an onlooker can understand where the feature lives and why.

## Notes for the coding agent

- Read the existing docs and the nearby runtime code before changing anything.
- Prefer the smallest coherent implementation that proves the feature.
- Preserve room for future recovery, redaction, and replay work.
- Do not overfit the implementation to this prompt; use judgment.
