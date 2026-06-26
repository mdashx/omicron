# Pi Reverse-Engineered Low-Level Spec

## Intent

This document describes the current Pi implementation at a low level, close to
the code that exists today.

It is intentionally lighter on examples and explanatory prose than the sibling
research specs. The goal of this pass is to expose the runtime seams, ownership
boundaries, and state transitions visible in the implementation.

---

## 1. Monorepo Partition

### Prose Spec

The Pi repo is a four-package monorepo with a layered dependency shape:

- `packages/ai`
  - provider registry, model metadata, streaming APIs, OAuth, image APIs
- `packages/agent`
  - provider-agnostic turn engine and harness/session abstractions
- `packages/tui`
  - terminal rendering, components, keyboard/input handling, overlays
- `packages/coding-agent`
  - product assembly layer that binds CLI, sessions, tools, resources, modes,
    extensions, auth, model resolution, compaction, archive, and optional
    Discord transport

The `coding-agent` package is the effective application runtime. The lower
packages are libraries it composes.

### Structural Spec

```text
PiRepo
  packages: { ai, agent, tui, coding-agent }
where
  coding-agent -> agent
  coding-agent -> ai
  coding-agent -> tui
  agent -> ai
```

### Code Anchors

- `package.json`
- `packages/coding-agent/src/index.ts`
- `packages/agent/src/index.ts`
- `packages/ai/src/index.ts`

---

## 2. CLI Bootstrap and Mode Selection

### Prose Spec

`packages/coding-agent/src/main.ts` is the product entrypoint. It:

- parses CLI args
- reads piped stdin when non-interactive
- resolves the app mode
- resolves session selection/fork/new behavior
- resolves models and session cwd issues
- creates cwd-bound services
- creates the session runtime
- dispatches to interactive, print/json, or rpc mode

Mode selection is implementation-driven:

- `rpc` when explicitly requested
- `json` when explicitly requested
- `print` when `--print` is set or stdin is non-TTY
- `interactive` otherwise

### Structural Spec

```text
AppMode ::= interactive | print | json | rpc

resolveAppMode(parsed, stdinIsTTY) =
  rpc          if parsed.mode = rpc
  json         if parsed.mode = json
  print        if parsed.print = true or stdinIsTTY = false
  interactive  otherwise
```

### Code Anchors

- `packages/coding-agent/src/main.ts`
- `packages/coding-agent/src/cli/args.ts`
- `packages/coding-agent/src/modes/index.ts`

---

## 3. Cwd-Bound Service Graph

### Prose Spec

Before an `AgentSession` exists, Pi creates a cwd-bound service bundle. That
bundle is recreated when the effective session cwd changes.

`createAgentSessionServices()` currently owns:

- canonical `cwd`
- `agentDir`
- auth storage
- settings manager
- model registry
- resource loader
- runtime diagnostics

This service layer is intentionally separate from session creation so model,
tool, and resource decisions can be resolved against the target cwd first.

### Structural Spec

```text
AgentSessionServices
  cwd: Path
  agentDir: Path
  authStorage: AuthStorage
  settingsManager: SettingsManager
  modelRegistry: ModelRegistry
  resourceLoader: ResourceLoader
  diagnostics: seq Diagnostic
```

### Code Anchors

- `packages/coding-agent/src/core/agent-session-services.ts`
- `packages/coding-agent/src/core/resource-loader.ts`
- `packages/coding-agent/src/core/settings-manager.ts`
- `packages/coding-agent/src/core/model-registry.ts`

---

## 4. Session Journal Model

### Prose Spec

Pi persists sessions as JSONL files managed by `SessionManager`.

The file format is append-oriented and tree-aware:

- one header entry defines session identity and cwd
- subsequent entries carry messages and session metadata
- every entry has `id`, `parentId`, and `timestamp`
- branch/fork structure is represented inside the session graph, not only by
  separate files

The current entry types include:

- `message`
- `thinking_level_change`
- `model_change`
- `compaction`
- `branch_summary`
- `custom`
- `custom_message`
- `label`
- `session_info`

Session persistence is not just transcript storage. It is also where Pi stores:

- model/thinking changes
- compaction summaries
- extension-owned state
- user labels and display metadata

### Structural Spec

```text
SessionHeader
  type = "session"
  id: SessionId
  timestamp: ISO8601
  cwd: Path
  parentSession?: Path

SessionEntry
  type: EntryType
  id: EntryId
  parentId: EntryId | null
  timestamp: ISO8601
```

### Code Anchors

- `packages/coding-agent/src/core/session-manager.ts`

---

## 5. AgentSession as Product Runtime Object

### Prose Spec

`AgentSession` is the mode-independent runtime abstraction used by interactive,
print, and rpc modes.

It owns or mediates:

- current `Agent`
- current session manager
- current settings manager
- model + thinking level state
- tool definition registry
- extension runner
- prompt execution
- event subscription
- session statistics
- compaction
- export and branch operations

This object is higher-level than the generic `agent` package loop. It is where
Pi-specific product behavior is attached to the lower-level agent engine.

### Structural Spec

```text
AgentSession
  agent: Agent
  sessionManager: SessionManager
  settingsManager: SettingsManager
  modelRegistry: ModelRegistry
  resourceLoader: ResourceLoader
  toolDefinitions: Map<ToolName, ToolDefinition>
  extensionRunner: ExtensionRunner
```

### Code Anchors

- `packages/coding-agent/src/core/agent-session.ts`
- `packages/coding-agent/src/core/sdk.ts`

---

## 6. Runtime Replacement and Session Switching

### Prose Spec

Pi does not treat a session as a static singleton. `AgentSessionRuntime`
supports replacing the active runtime when the user:

- resumes another session
- creates a new session
- forks a session
- imports a session into a new runtime

Replacement semantics are explicit:

1. emit pre-switch hooks
2. emit `session_shutdown`
3. dispose archive runtime if present
4. invalidate the old session
5. create a fresh cwd-bound runtime
6. bind the host mode to the new session

This is a major architectural fact: the host UI/mode survives, while the
underlying session runtime can be swapped out.

### Structural Spec

```text
AgentSessionRuntime
  session: AgentSession
  services: AgentSessionServices
  diagnostics: seq Diagnostic
  archive?: SessionArchiveRuntime
```

### Code Anchors

- `packages/coding-agent/src/core/agent-session-runtime.ts`

---

## 7. Agent Turn Engine

### Prose Spec

The lower-level turn engine lives in `packages/agent/src/agent-loop.ts`.

Its execution model is:

- start with prompt messages or continue from an existing context
- emit lifecycle events
- call the provider through `streamSimple`
- collect assistant output
- execute tool calls in batches
- append tool result messages into context
- continue until stop conditions, termination, or follow-up/steering rules end
  the run

The loop supports two queue classes:

- steering messages
  - injected while work is ongoing
- follow-up messages
  - injected after the agent would otherwise stop

The product-level `AgentSession` and the harness layer build on this loop rather
than reimplementing turn execution.

### Structural Spec

```text
AgentLoop
  context: AgentContext
  config: AgentLoopConfig
  pendingSteering: seq AgentMessage
  pendingFollowUp: seq AgentMessage
where
  assistant turns may yield tool calls
  tool results are appended before the next assistant turn
```

### Code Anchors

- `packages/agent/src/agent-loop.ts`
- `packages/agent/src/agent.ts`

---

## 8. Harness Layer

### Prose Spec

`AgentHarness` is the generic higher-order runtime in `packages/agent`. It
wraps:

- session state
- resources
- tools
- model selection
- hook/event registration
- compaction hooks
- prompt-template and skill expansion

This sits conceptually between the raw loop and the full coding-agent product.
It is a reusable agent runtime, whereas `coding-agent` adds the CLI/TUI/product
features on top.

### Code Anchors

- `packages/agent/src/harness/agent-harness.ts`
- `packages/agent/src/harness/types.ts`

---

## 9. Provider and Model Dispatch

### Prose Spec

`packages/ai` is the provider abstraction layer.

Its current dispatch model is:

- providers register stream functions by `api`
- `stream()` / `streamSimple()` resolve the provider from the registry
- env API key lookup is applied unless an explicit key is provided
- the provider implementation owns translation to the upstream API

The important boundary is that the upper layers do not call OpenAI,
Anthropic, Google, or others directly. They call the `ai` package against
typed `Model<Api>` objects.

### Structural Spec

```text
ApiProvider
  api: Api
  stream(model, context, options) -> AssistantMessageEventStream
  streamSimple(model, context, options) -> AssistantMessageEventStream
```

### Code Anchors

- `packages/ai/src/api-registry.ts`
- `packages/ai/src/stream.ts`
- `packages/ai/src/providers/register-builtins.ts`

---

## 10. Tools, Resources, and Extension Overlay

### Prose Spec

Pi constructs a runtime overlay from several resource families:

- settings
- context files
- prompt templates
- skills
- extensions
- built-in tools
- custom tools

The resource loader discovers project-local and global assets. Extensions can
add tools, commands, providers, keybindings, prompt hooks, UI affordances, and
session lifecycle hooks.

Tool exposure is definition-first in the coding-agent layer. The runtime
supports:

- default built-ins
- allowlists
- denylists
- custom tools
- extension-provided tools

### Code Anchors

- `packages/coding-agent/src/core/resource-loader.ts`
- `packages/coding-agent/src/core/extensions/index.ts`
- `packages/coding-agent/src/core/extensions/runner.ts`
- `packages/coding-agent/src/core/tools/index.ts`
- `packages/coding-agent/src/core/system-prompt.ts`

---

## 11. Interaction Modes

### Prose Spec

Pi has three product-facing execution surfaces:

- interactive terminal mode
- print/json one-shot mode
- rpc mode

These modes share the same `AgentSession` abstraction and differ mostly in I/O,
transport, and host behavior.

The interactive surface is built on `@earendil-works/pi-tui`, whose `TUI`
implementation owns:

- component tree rendering
- differential repainting
- focus management
- overlays
- keyboard dispatch
- hardware cursor placement
- terminal image cleanup

### Code Anchors

- `packages/coding-agent/src/modes/interactive/interactive-mode.ts`
- `packages/coding-agent/src/modes/print-mode.ts`
- `packages/coding-agent/src/modes/rpc/rpc-mode.ts`
- `packages/tui/src/tui.ts`

---

## 12. Product-Specific Adjunct Subsystems

### Prose Spec

The coding-agent package already contains product-level adjunct subsystems that
sit outside the core turn loop:

- session archive runtime
- Discord transport/runtime glue
- compaction and branch summarization
- HTML export
- trust/project-trust checks
- telemetry and timings

These are optional or orthogonal services, but they are part of the current
application architecture and should be treated as first-class runtime concerns.

### Code Anchors

- `packages/coding-agent/src/core/session-archive.ts`
- `packages/coding-agent/src/core/discord-transport.ts`
- `packages/coding-agent/src/core/compaction/index.ts`
- `packages/coding-agent/src/core/export-html/index.ts`
- `packages/coding-agent/src/core/trust-manager.ts`

---

## 13. Current Architectural Invariants

### Prose Spec

The current code implies these invariants:

- `coding-agent` is the application composition root
- cwd-bound services are recreated when runtime cwd changes
- session persistence is append-oriented JSONL with tree metadata
- `AgentSession` is the shared runtime object across modes
- provider calls are always mediated through `packages/ai`
- the generic loop works in `AgentMessage` space and only crosses into provider
  message format at the LLM boundary
- interactive, print, and rpc modes are host adapters over the same session
  runtime
- extensions and resources are runtime overlays, not compile-time wiring
