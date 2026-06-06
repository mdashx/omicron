# Coding Agent Prompt: Discord Bridge Service

## Context

Implement the Discord bridge service described in:

- `research/discord-bridge/SPEC.md`
- `research/discord-bridge/IMPLEMENTATION-PLAN.md`
- `research/discord-bridge/README.md` (if present)

The bridge should act as an always-on Discord ingress/egress layer for Pi-compatible agents. It should also provide bridge-owned structure around interactions: acknowledgement reactions, reaction progress updates, final reaction choices, and durable local logs/downloads for all chats.

## Goal

Add a startup-loaded Discord bridge service to the Pi codebase so Discord messages, reactions, and lifecycle events are normalized into a bridge-owned control plane, routed to one bound agent per channel, and persisted as bridge-wide local memory.

The bridge must be enforced by runtime code, not by prompting the model to remember Discord behavior.

## New requirements

The bridge should ship with defaults that work immediately.

The user should be able to start the bridge with no special config and still get a working storage path. If the storage directory does not exist, create it recursively. Users can still override the defaults with config.

In addition to per-agent automation, the bridge should also provide wholesale features:
- automatically keep a local log of all chats
- automatically keep a local log of all attachments
- maintain consistent bridge-owned reaction behavior
- preserve replay/search/audit data across restarts

## What to optimize for

- correct startup wiring
- clear runtime boundaries
- fail-closed enforcement when bridge wiring is invalid
- no LLM involvement in deciding whether bridge persistence or routing happens
- minimal but durable defaults
- deterministic bridge-owned behavior across agents and models

## Where to look first in the codebase

Start with these files and concepts:

- `packages/coding-agent/src/main.ts`
  - CLI/bootstrap entry point
  - best place to resolve defaults and wire startup behavior

- `packages/coding-agent/src/core/agent-session-runtime.ts`
  - runtime replacement/new-session flow
  - good place to ensure bridge state follows session lifecycle changes

- `packages/coding-agent/src/core/agent-session-services.ts`
  - service creation boundary
  - good place to pass resolved bridge config into session creation

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
  - structured runtime event philosophy that can inform bridge events too

- `research/discord-transport-layer/README.md`
  - prior transport ideas and baseline bridge-ish patterns

## Suggested implementation shape

This is a suggested shape, not a prescription.

1. Resolve built-in Discord bridge defaults.
2. Merge user overrides onto the defaults.
3. Validate the resolved config.
4. Create the storage root if missing.
5. Generate a bridge/session envelope (`bridgeId`, timestamps, host, bot IDs, transport mode).
6. Install runtime hooks or a plugin that normalizes inbound gateway events.
7. Route inbound messages to the bound agent.
8. Automatically add/update reactions as bridge-owned state.
9. Log chats and attachments locally in append-only bridge storage.
10. Convert directives or bridge rules into pure action plans.
11. Execute or dry-run replies/reactions/threads.
12. Persist processed event IDs, action IDs, and binding state.
13. Fail closed if bridge wiring is required but broken.

## Example code direction

The concrete implementation will likely need something like this shape:

```ts
const defaults = {
  enabled: true,
  botToken: "env.DISCORD_BOT_TOKEN",
  bridgeId: "discord-bridge-main",
  host: "127.0.0.1",
  port: 19444,
  storageRoot: "~/.pi/discord-bridge",
  logsRoot: "~/.pi/discord-bridge/logs",
  downloadsRoot: "~/.pi/discord-bridge/downloads",
  ackReaction: "✅",
  statusReactions: ["⏳", "🤖", "💭", "✅", "⚠️"],
  finalReactionChoices: ["✅", "👍", "👀", "🧠", "❤️"],
};

const resolved = resolveDiscordBridgeConfig(defaults, userConfig);
await mkdir(resolved.storageRoot, { recursive: true });
const bridge = createDiscordBridge(resolved, envelope);
await bridge.start();
```

A good implementation should still make its own judgment about the final API shape. This snippet is just orientation.

## What to change if the design wants it

If the cleanest solution is a core runtime feature rather than a traditional extension, choose the runtime boundary that makes startup enforcement clearest.

The important thing is not the packaging label. The important thing is:

- startup-loaded
- deterministic
- bridge-owned
- append-only/idempotent where needed
- no LLM involvement
- default configuration that works immediately
- directory creation handled automatically
- consistent ack/progress/final-reaction behavior across agents/models

## Acceptance criteria

- Pi starts with Discord bridge defaults and no custom config.
- The storage directory is created automatically if it does not exist.
- A Discord bridge envelope is created at startup.
- Inbound Discord events are normalized into stable internal shapes.
- A bound agent can join one channel per binding.
- Acknowledgement reactions are added automatically on accepted messages.
- Reaction status updates reflect bridge/LLM progress.
- The bridge can present a final reaction palette for the agent to choose from.
- All chats and attachments are logged locally by the bridge.
- Processed event IDs are persisted to prevent duplicates.
- The implementation is enforced by runtime code, not by prompt text.
- The implementation stays readable enough that an onlooker can understand where the feature lives and why.

## Notes for the coding agent

- Read the existing docs and the nearby runtime code before changing anything.
- Prefer the smallest coherent implementation that proves the feature.
- Preserve room for future replay, search, and richer harness behaviors.
- Do not overfit the implementation to this prompt; use judgment.
