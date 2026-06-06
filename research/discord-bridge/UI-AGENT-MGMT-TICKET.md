# Coding Agent Prompt: Discord Bridge UI Rebuild + Bridge-Owned Agent Management

## Context

Implement the next iteration of the Discord bridge prototype described in:

- `research/discord-bridge/SCOPE.md`
- `research/discord-bridge/SPEC.md`
- `research/discord-bridge/IMPLEMENTATION-PLAN.md`
- `research/discord-bridge/README.md`

The current Discord bridge backend proved the basic control-plane ideas, but the web UI is still too debug-heavy and the current agent launch/stop model is not strong enough to be the primary operational surface.

This ticket is specifically about rebuilding the bridge UI and clarifying bridge-owned agent management from within the Discord bridge itself.

---

## Goal

Turn the Discord bridge from a prototype dashboard into a usable operator console.

The resulting system should:

- present high-signal bridge state first
- make managed-agent lifecycle state explicit
- make launch / stop / restart behavior more trustworthy
- treat logs and transcripts as drill-down debugging tools, not the homepage
- keep the bridge as the owner of agent lifecycle and channel binding state

---

## Strong implementation preference

Prefer a normal traditional web app architecture:

- server-rendered HTML pages
- Go `html/template` for page generation
- standard HTTP handlers and forms
- HTMX for live data refresh, inline actions, and partial updates

Do **not** default to a large client-side SPA or a JSON-API-first frontend architecture for this scope.

The default mental model should be:

- the server renders complete pages
- the browser progressively enhances interactions with HTMX
- fragments/partials are rendered on the server too
- JavaScript remains minimal and optional beyond HTMX-level behavior

This should feel like a conventional operations/admin web app, not a front-end app shell.

---

## Why this rendering approach

This project is a Go daemon with a compact operational surface.

Server-side templates plus HTMX are a good fit because they:

- keep UI state close to bridge runtime state
- reduce front-end complexity
- make the HTML easy to inspect and reason about
- keep the UI easy to evolve alongside Go structs and handlers
- support incremental live updates without introducing a heavy client framework
- fit well with forms, tables, detail pages, and operator actions

Optimize for clarity, maintainability, and speed of iteration.

---

## In scope

### 1. Rebuild the bridge UI

Replace the current dashboard with a server-rendered operator UI built from Go templates.

The new UI should emphasize:

- bridge health
- managed agents
- bindings/channels
- recent activity
- clear operator actions

### 2. Introduce a stronger bridge-owned managed-agent model

The bridge should move from transient “launched agent” records toward stronger managed-agent records that distinguish at least:

- desired state
- observed process state
- observed bridge/join state
- work state

### 3. Add server-rendered partials for live views

Use HTMX for things like:

- refreshing overview cards
- refreshing the managed-agents table
- refreshing activity feeds
- inline start / stop / restart actions
- bind / rebind actions
- expandable detail sections

### 4. Demote raw debug views

Logs, transcripts, audit tails, session artifacts, and similar raw data should move into drill-down pages or collapsible detail sections.

These are useful, but not primary.

---

## Out of scope

- building a client-side SPA
- introducing React/Vue/Svelte as the default UI architecture
- redesigning the Discord message/event contract
- replacing Pi RPC as the upstream harness path
- full auth/RBAC/multi-tenant admin systems
- production-grade orchestration or autoscaling
- broad replay/search tooling for all historical data

---

## Required UI shape

The exact HTML is flexible, but the UI should likely include the following server-rendered views.

### 1. Overview page

High-signal summary only.

Include items such as:

- Discord connection status
- bridge uptime
- total managed agents
- healthy joined agents
- queued work count
- agents needing attention

### 2. Managed Agents page

This should be the main operations page.

Show a server-rendered table/list with columns such as:

- agent id
- desired state
- observed process state
- observed bridge state
- work state
- assigned channel
- queue depth
- last activity
- last completion
- last error
- actions

### 3. Bindings / Channels page

Show:

- assignable channels
- active bindings
- dormant bindings
- conflicts / blocked ownership
- channel-to-agent mapping

### 4. Activity page or panel

Show a concise human-readable operational feed, not a raw audit dump.

Examples:

- agent launched
- agent joined
- agent stopped
- binding changed
- completion sent
- bridge error observed

### 5. Agent detail page

Show richer server-rendered detail for a single managed agent:

- launch config
- working directory
- command / args
- process metadata
- join/binding state
- recent bridge activity
- recent errors
- links to logs/session artifacts

---

## Required interaction model

Prefer ordinary HTML forms/buttons enhanced by HTMX.

Examples of good interaction patterns:

- POST form to start an agent, returning an updated row or flash message
- POST form to stop an agent, returning updated state
- POST form to restart an agent
- POST form to change desired state
- POST form to bind or rebind a channel
- HTMX polling/triggered refresh for overview cards and activity snippets

Avoid building a thick custom front-end state machine unless clearly necessary.

---

## Suggested route shape

The exact routes may differ, but prefer a split like this:

### HTML pages
- `GET /`
- `GET /agents`
- `GET /agents/{agentId}`
- `GET /bindings`
- `GET /activity`

### HTML partials / HTMX targets
- `GET /partials/overview`
- `GET /partials/agents-table`
- `GET /partials/agents/{agentId}/summary`
- `GET /partials/activity`
- `GET /partials/bindings-table`

### Actions
- `POST /agents`
- `POST /agents/{agentId}/start`
- `POST /agents/{agentId}/stop`
- `POST /agents/{agentId}/restart`
- `POST /agents/{agentId}/desired-state`
- `POST /agents/{agentId}/bind`

This is a suggested shape, not a locked API.

---

## Suggested code organization

Keep the UI implementation simple and idiomatic for Go.

For example:

- `templates/`
  - base layout
  - page templates
  - partial templates
- `ui/` or `web/`
  - page handlers
  - form parsing
  - view models
  - template rendering helpers
- bridge/domain code remains separate from UI rendering concerns

Try to keep:

- domain state computation
- process management
- bridge lifecycle management
- template rendering

as distinct layers.

---

## Agent-management requirements

The bridge should define stronger managed-agent behavior.

At minimum, introduce a concept like a persisted managed-agent record containing:

- agent id
- creds ref
- desired state
- launch command
- launch args
- working directory
- channel binding intent
- auto-launch policy
- observed process info
- observed bridge join/binding info
- last activity
- last completion
- last error

Clarify bridge semantics for:

- launch requested
- launch succeeded
- process running
- process exited
- join observed
- binding confirmed
- stop requested
- stop confirmed
- stale state after restart

A running process must not automatically be treated as a healthy agent.

---

## Data-model direction

Prefer explicit state fields over one overloaded string.

For example, distinguish:

### Desired state
- `running`
- `stopped`
- `disabled`

### Process state
- `not_started`
- `starting`
- `running`
- `stopping`
- `exited`
- `failed`
- `unknown`

### Bridge state
- `never_joined`
- `joining`
- `joined`
- `bound`
- `degraded`
- `stale`

### Work state
- `idle`
- `queued`
- `processing`
- `completing`
- `errored`

Compute readable health/status badges for the UI from these explicit fields.

---

## Acceptance criteria

- The bridge UI is rebuilt primarily as server-rendered HTML pages.
- Go templates are the default rendering mechanism.
- HTMX is used for live refresh and interactions, not as a SPA framework substitute.
- The default homepage is high-signal and not dominated by raw logs.
- Managed agents are modeled as first-class bridge-owned records.
- Agent lifecycle state is clearer than simple spawn/kill status.
- Start / stop / restart behavior is represented explicitly in the UI.
- Bindings/channels have a clearer operator-facing view.
- Debug logs remain available, but secondary.
- The code stays readable and idiomatic for a Go server-side web app.

---

## Notes for the coding agent

- Read `SCOPE.md` first and respect its boundaries.
- Prefer small, coherent page/partial abstractions over a large front-end architecture.
- Keep templates boring and conventional.
- Prefer HTML forms and server redirects/responses where possible.
- Use HTMX where it clearly improves operator experience.
- Avoid overengineering the visual design; prioritize structure and clarity.
- Keep room for later refinement of styling and richer drill-down views.

---

## Summary

Build the next Discord bridge UI as a traditional Go web app.

Render pages on the server.
Render partials on the server.
Use HTMX for live updates and inline actions.

At the same time, strengthen bridge-owned agent management so the UI reflects real lifecycle and health state instead of a thin wrapper around process spawn/kill.
