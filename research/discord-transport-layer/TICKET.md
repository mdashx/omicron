# Coding Agent Prompt: Pi Discord Transport Layer

## Context

Implement the Pi Discord transport layer described in:

- `research/discord-transport-layer/SPEC.md`
- `research/discord-transport-layer/IMPLEMENTATION-PLAN.md`
- `research/discord-transport-layer/README.md`

Also use the simple Discord bot harness as a concrete baseline for the transport shape:

- `/home/easter/general-agent-harness/projects/simple-discord-bot-harness`

## Goal

Add a startup-loaded Discord transport layer to Pi so Discord messages, reactions, and lifecycle events are normalized into agent-friendly events, and agent outputs are turned back into Discord replies, reactions, and thread actions.

The feature must be enforced by runtime code, not by prompting the model to remember Discord behavior.

## What to optimize for

- correct startup wiring
- clear runtime boundaries
- fail-closed enforcement when Discord wiring is invalid
- no LLM involvement in deciding whether Discord transport happens
- minimal but durable defaults
- onboarding clarity for future readers without overconstraining the implementation

## Where to look first in the codebase

Start with these files and concepts:

- `packages/coding-agent/src/main.ts`
  - CLI/bootstrap entry point
  - best place to resolve defaults and wire startup behavior

- `packages/coding-agent/src/core/agent-session-runtime.ts`
  - runtime replacement/new-session flow
  - good place to ensure transport state follows session lifecycle changes

- `packages/coding-agent/src/core/agent-session-services.ts`
  - service creation boundary
  - good place to pass resolved transport config into session creation

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
  - structured runtime event philosophy that can inform transport events too

- `/home/easter/general-agent-harness/projects/simple-discord-bot-harness/src/index.mjs`
  - baseline Discord event → directive → action flow

- `/home/easter/general-agent-harness/projects/simple-discord-bot-harness/src/state.mjs`
  - baseline idempotency state shape

## Suggested implementation shape

This is a suggested shape, not a prescription.

1. Resolve built-in Discord transport defaults.
2. Merge user overrides onto the defaults.
3. Validate the resolved config.
4. Create the state directory if missing.
5. Generate a Discord session envelope (`sessionId`, timestamps, host, bot IDs, transport mode).
6. Install runtime hooks or a plugin that normalizes inbound gateway events.
7. Parse a tiny prefix command set into directives.
8. Convert directives into pure action plans.
9. Execute or dry-run replies/reactions/threads.
10. Persist processed event IDs and action IDs.
11. Fail closed if transport wiring is required but broken.

## Example code direction

The concrete implementation will likely need something like this shape:

```ts
const defaults = {
  enabled: true,
  tokenSource: "env.DISCORD_BOT_TOKEN",
  commandPrefix: "!",
  guildAllowlist: ["123456789012345678"],
  channelAllowlist: ["234567890123456789"],
  ownerAllowlist: [],
  dryRun: true,
  statePath: "~/.pi/discord-transport/state.json",
  threadMode: "off",
  replyMode: "reply-to-message",
  maxActionsPerTurn: 8,
};

const resolved = resolveDiscordTransportConfig(defaults, userConfig);
await mkdir(dirname(resolved.statePath), { recursive: true });
const transport = createDiscordTransport(resolved, envelope);
await transport.start();
```

A good implementation should still make its own judgment about the final API shape. This snippet is just orientation.

## What to change if the design wants it

If the cleanest solution is a core runtime feature rather than a traditional extension, choose the runtime boundary that makes startup enforcement clearest.

The important thing is not the packaging label. The important thing is:

- startup-loaded
- deterministic
- append-only/idempotent where needed
- no LLM involvement
- default configuration that works immediately
- directory creation handled automatically

## Acceptance criteria

- Pi starts with Discord transport defaults and no custom config.
- The state directory is created automatically if it does not exist.
- A Discord session envelope is created at startup.
- Inbound Discord events are normalized into stable internal shapes.
- A tiny prefix command set is parsed deterministically.
- Actions are planned separately from execution.
- Replies/reactions can be dry-run or sent.
- Processed event IDs are persisted to prevent duplicates.
- The implementation is enforced by runtime code, not by prompt text.
- The implementation stays readable enough that an onlooker can understand where the feature lives and why.

## Notes for the coding agent

- Read the existing docs and the nearby runtime code before changing anything.
- Prefer the smallest coherent implementation that proves the feature.
- Preserve room for future thread handling, attachment routing, and replay work.
- Do not overfit the implementation to this prompt; use judgment.
