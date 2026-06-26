# Pi Specification

## Intent

This specifies Pi as it currently exists in the codebase: a layered agent
runtime and product shell built around persistent session journals, provider-
agnostic turn execution, a resource and extension overlay, and multiple host
surfaces over one shared session abstraction.

The model here is conceptual rather than file-by-file, but it is still reverse
engineered from the current implementation.

---

## 1. `PiSystem`

### Prose Spec

Pi is not one package. It is a composed system with four main layers:

- an AI/provider layer
- a generic agent loop and harness layer
- a terminal UI layer
- a coding-agent application layer

The coding-agent layer is the control plane that assembles the other three into
the user-facing product.

### Z Spec

```text
PiSystem
  ai: PiAI
  agentCore: PiAgentCore
  tui: PiTUI
  app: PiCodingAgent
where
  app depends on ai
  app depends on agentCore
  app depends on tui
  agentCore depends on ai
```

### Implementation specifics

- `packages/coding-agent` is the composition root.
- The lower layers are reusable libraries, not just internal folders.

---

## 2. `PiCodingAgent`

### Prose Spec

`PiCodingAgent` is the product runtime that interprets CLI arguments, resolves
resources and configuration, selects or creates a session, and runs one of the
host modes.

Its job is not to implement the language-model protocol itself. Its job is to
assemble a coherent runtime around the lower layers.

### Z Spec

```text
APPMODE ::= interactive | print | json | rpc

PiCodingAgent
  mode: APPMODE
  cwd: seq CHAR
  agentDir: seq CHAR
  services: AgentSessionServices
  runtime: AgentSessionRuntime
where
  cwd ≠ ⟨⟩
  agentDir ≠ ⟨⟩
```

### Implementation specifics

- Bootstrap currently lives in `packages/coding-agent/src/main.ts`.
- Mode selection depends on explicit flags plus TTY detection.

---

## 3. `AgentSessionServices`

### Prose Spec

Pi creates a cwd-bound service bundle before it creates the session runtime.

This bundle exists so the application can resolve settings, models, auth,
resources, and extensions against the effective cwd first, then build the
session on top of that resolved environment.

### Z Spec

```text
AgentSessionServices
  cwd: seq CHAR
  agentDir: seq CHAR
  authStorage: AuthStorage
  settingsManager: SettingsManager
  modelRegistry: ModelRegistry
  resourceLoader: ResourceLoader
  diagnostics: seq Diagnostic
```

### Implementation specifics

- This service layer is recreated when Pi switches to a session rooted in a
  different cwd.
- Diagnostics are collected here and returned upward rather than printed at the
  point of construction.

---

## 4. `PiSessionJournal`

### Prose Spec

Pi sessions are durable journals, not transient chat buffers.

A session file begins with a header and then accumulates append-oriented entries
that describe messages, model changes, thinking changes, compaction summaries,
labels, custom extension state, and other session metadata.

The journal is also branch-aware. Entry IDs and parent IDs encode tree
structure, so Pi can support branching, forks, and compaction-aware history
operations.

### Z Spec

```text
ENTRYTYPE ::= message | thinking_level_change | model_change | compaction
           | branch_summary | custom | custom_message | label | session_info

PiSessionJournal
  header: SessionHeader
  entries: seq SessionEntry
where
  header.type = "session"
  ∀ entry in entries : entry.id ≠ ⟨⟩
```

### Data example

```json
{
  "type": "session",
  "id": "01K0...",
  "cwd": "/home/easter/omicron"
}
```

### Implementation specifics

- The persistence model lives in `SessionManager`.
- Session storage is JSONL and append-oriented.
- Session metadata is part of the same journal model as the transcript.

---

## 5. `AgentSession`

### Prose Spec

`AgentSession` is the shared runtime object across all Pi host modes.

It binds the generic agent engine to product concerns such as:

- session persistence
- extension execution
- tool exposure
- model and thinking selection
- compaction
- export and branch operations

This is the stable object that interactive, print, and rpc modes all operate
through.

### Z Spec

```text
AgentSession
  agent: Agent
  sessionManager: SessionManager
  settingsManager: SettingsManager
  extensionRunner: ExtensionRunner
  modelRegistry: ModelRegistry
  resourceLoader: ResourceLoader
```

### Implementation specifics

- `AgentSession` is mode-independent by design.
- Host modes add I/O and transport, not a different core agent abstraction.

---

## 6. `AgentSessionRuntime`

### Prose Spec

Pi treats the runtime as replaceable.

The host process may keep running while the active `AgentSession`, its cwd-bound
services, and optional archive runtime are torn down and rebuilt. This happens
for session switching, new sessions, forks, and similar transitions.

### Z Spec

```text
AgentSessionRuntime
  session: AgentSession
  services: AgentSessionServices
  archive?: SessionArchiveRuntime
  diagnostics: seq Diagnostic
```

### Implementation specifics

- Runtime replacement is explicit, not incidental.
- Session shutdown hooks run before invalidation.
- The host mode can rebind to the new session after replacement.

---

## 7. `PiAgentLoop`

### Prose Spec

The agent loop is provider-agnostic and message-centric.

It operates in `AgentMessage` space, emits lifecycle events, streams one
assistant turn at a time, executes tool calls, appends tool results, and
continues until stop conditions are met.

It also distinguishes two queue types:

- steering messages for in-flight intervention
- follow-up messages for post-turn continuation

### Z Spec

```text
PiAgentLoop
  context: AgentContext
  config: AgentLoopConfig
  steeringQueue: seq AgentMessage
  followUpQueue: seq AgentMessage
where
  tool results follow assistant tool calls
```

### Implementation specifics

- The generic loop lives in `packages/agent/src/agent-loop.ts`.
- Product features build on this loop instead of bypassing it.

---

## 8. `PiResourceOverlay`

### Prose Spec

Pi assembles behavior from a runtime overlay of resources rather than from one
hardcoded configuration source.

This overlay includes:

- settings
- context files
- prompt templates
- skills
- extensions
- built-in tools
- custom tools

Extensions can affect both agent behavior and host behavior. They are not only
tool plugins; they can also contribute providers, lifecycle hooks, commands,
keybindings, and UI pieces.

### Z Spec

```text
PiResourceOverlay
  settings: Settings
  contextFiles: seq ContextFile
  promptTemplates: seq PromptTemplate
  skills: seq Skill
  extensions: seq Extension
  tools: seq ToolDefinition
```

### Implementation specifics

- Resource discovery is cwd-sensitive.
- The coding-agent layer is definition-first for tool exposure.
- Extension runtime state is part of the live session environment.

---

## 9. `PiAI`

### Prose Spec

Pi’s AI layer is a typed provider registry and streaming abstraction.

Upper layers work with typed models and generic stream calls. Provider-specific
logic is pushed downward into registered API providers.

This makes OpenAI-, Anthropic-, Google-, and other backends implementation
details of the `ai` package rather than direct dependencies of the application
layer.

### Z Spec

```text
PiAI
  providers: Map<Api, ApiProvider>
  models: seq Model
where
  ∀ provider : provider.api matches registered key
```

### Implementation specifics

- `stream()` and `streamSimple()` resolve providers by `api`.
- Environment API key lookup is applied unless an explicit key is provided.

---

## 10. `PiHostModes`

### Prose Spec

Pi exposes multiple host surfaces over the same session runtime:

- interactive terminal mode
- print/json mode
- rpc mode

The interactive surface is built on a TUI system with differential rendering,
focus handling, overlays, and terminal-specific cursor/image behavior.

### Z Spec

```text
HOSTMODE ::= interactive | print | json | rpc

PiHostModes
  activeMode: HOSTMODE
  session: AgentSession
```

### Implementation specifics

- Modes differ mainly in transport and presentation.
- They do not each own separate core session semantics.

---

## 11. `PiAdjunctServices`

### Prose Spec

Pi includes adjunct runtime services that are orthogonal to the core turn loop
but still architecturally important:

- session archive
- Discord transport integration
- compaction and branch summarization
- HTML export
- project trust
- telemetry/timings

These services should be modeled as first-class product capabilities rather than
as accidental utilities.

### Z Spec

```text
PiAdjunctServices
  archive?: SessionArchiveRuntime
  discord?: DiscordTransport
  compaction: CompactionSubsystem
  exportHtml: HtmlExportSubsystem
  trust: TrustSubsystem
```

### Implementation specifics

- These concerns live mostly in `packages/coding-agent/src/core`.

---

## 12. Architectural Facts

### Prose Spec

The current implementation implies the following stable architectural facts:

- Pi is layered, not monolithic.
- The coding-agent package is the application composition root.
- Sessions are durable journals with branch-aware metadata.
- One shared `AgentSession` abstraction spans all host modes.
- The generic agent loop is provider-agnostic and tool-call-aware.
- Provider calls are always mediated through the `ai` registry layer.
- Resources and extensions are runtime overlays.
- Session runtimes are replaceable without killing the host process.

### Implementation specifics

- Any future spec or redesign should preserve or explicitly reject these facts,
  rather than accidentally drifting away from them.
