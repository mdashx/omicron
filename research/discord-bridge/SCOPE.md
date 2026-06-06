# Discord Bridge UI + Agent Management Scope

## Intent

Refocus the next iteration of the Discord bridge prototype on two areas only:

1. a complete rebuild of the bridge web UI
2. a clearer, safer model for agent management owned by the bridge itself

This scope exists because the current dashboard is not a good operational surface. Most panels are low-signal, the UI is too close to raw logs, and the launch/stop behavior is not yet trustworthy enough to act as the primary way to manage bridge-connected agents.

---

## Why this scope change

The current bridge prototype proved several backend ideas:

- Discord ingress and outbound delivery
- one-agent-per-channel binding
- bridge-owned reactions
- append-only logs and audit data
- local HTTP control plane
- basic spawning/stopping of bridge clients

But the current UI is still prototype-shaped:

- it exposes too many raw internals
- it does not present the bridge as a coherent control plane
- it mixes useful operational status with debug-only transcript panels
- it does not make agent lifecycle state clear enough
- it does not yet inspire confidence for launch / stop / recover workflows

The next step should therefore optimize for operator trust and bridge-owned control, not more debug surface area.

---

## In scope

### 1. Rebuild the bridge web UI from scratch

The UI should stop being a log dashboard with a launch form attached.

Instead it should become an operator-facing control surface for:

- bridge health
- Discord connection state
- channel bindings
- queued work
- agent presence and lifecycle
- recent completions and failures
- explicit operator actions

The rebuilt UI should emphasize:

- current state over raw transcripts
- actionable controls over passive debug panels
- confidence and clarity over exhaustiveness
- stable objects and workflow states over append-only noise

### 2. Redesign bridge-owned agent management

The bridge should own the lifecycle model for managed agents more explicitly.

This includes clarifying:

- what it means for an agent to be known vs launched vs joined vs bound vs healthy
- whether the bridge is managing a configured agent slot or an arbitrary spawned process
- what “stop” guarantees and what it does not
- how operator intent differs from observed process state
- how launch failures, reconnects, and stale bindings are surfaced
- how the bridge should recover after restart

The goal is not just to spawn processes, but to model agent lifecycle in a way the bridge can reason about safely.

### 3. Define bridge-first UI/API objects for managed agents

The bridge likely needs stronger first-class concepts such as:

- managed agent
- desired state
- observed state
- process record
- join record
- binding record
- health record
- last activity / last completion / last error

This may require evolving the current launch/stop endpoints into a more explicit management API.

### 4. Preserve useful debug access without making it the main UI

Raw logs, PTY traces, session paths, and audit tails may still exist, but they should move behind secondary drill-down views.

They are useful for diagnosis, but should not dominate the default bridge UI.

---

## Out of scope

The following should not be the focus of this scope item:

- redesigning the Discord event contract
- changing the bridge reaction protocol
- adding new model providers
- generalizing beyond Discord
- replay/search tooling for all historical logs
- advanced RBAC or multi-user admin auth
- replacing the underlying Pi RPC harness design
- a full production scheduler/orchestrator

Those may follow later, but they are not required to fix the current operator experience.

---

## Problems to solve

### UI problems

- The most visible panels are not the most useful ones.
- The current homepage is too transcript-heavy.
- The operator cannot quickly answer:
  - is the bridge healthy?
  - which channels are bound?
  - which agents are alive?
  - which agents are actually joined?
  - what is stuck?
  - what failed recently?
- Launch and stop actions do not communicate enough state before or after the action.

### Agent management problems

- Launching a process is not the same as having a usable agent.
- A running PID is not enough evidence of bridge health.
- Join state and process state are currently too loosely related.
- Stale bindings and half-dead agents need clearer treatment.
- The bridge should know whether it is trying to keep an agent running, merely observing one, or temporarily launching one.

---

## Desired operator experience

A bridge operator should be able to open the UI and immediately understand:

- whether the Discord bridge is connected and healthy
- which channels are available, assigned, or conflicted
- which managed agents exist
- for each managed agent:
  - desired state
  - observed runtime state
  - joined/bound state
  - assigned channel
  - last work time
  - last completion status
  - last error
- what operator actions are available next

The operator should be able to:

- create or register an agent entry
- assign or reassign a channel
- launch an agent
- stop an agent
- restart an agent
- see whether the agent actually joined the bridge
- detect when a process is alive but not healthy
- inspect details only when necessary

---

## Proposed UI shape

The rebuilt UI should likely center around a few primary views.

### 1. Overview

High-signal summary cards only:

- Discord connection status
- bridge uptime
- number of managed agents
- number of healthy joined agents
- number of queued events
- number of agents needing attention

### 2. Managed Agents

A table or list of bridge-managed agents with explicit columns for:

- agent id
- desired state
- observed state
- process pid/status
- join state
- binding/channel
- queue depth
- last activity
- last completion
- last error
- actions

This should be the main operational surface.

### 3. Bindings / Channels

A clear view of:

- assignable channels
- currently bound channels
- dormant bindings
- conflicts or blocked assignments
- channel-to-agent ownership

### 4. Event / Activity Feed

A concise operational feed:

- joined
- launched
- stopped
- completion sent
- status updated
- error encountered
- binding changed

This should be a readable activity stream, not a raw audit dump.

### 5. Agent Detail

A drill-down page or panel for one agent showing:

- config / launch command
- process history
- join history
- recent bridge events
- recent completions
- last errors
- links to raw logs and session artifacts

---

## Proposed agent-management model

The bridge should shift from “launched agents” as an ad hoc runtime map to a stronger managed-agent model.

### Candidate state split

#### Desired state
What the operator or bridge wants:

- stopped
- running
- disabled

#### Observed process state
What the OS/runtime shows:

- not started
- starting
- running
- stopping
- exited
- failed
- unknown

#### Observed bridge state
What the bridge protocol shows:

- never joined
- joining
- joined
- bound
- degraded
- stale

#### Work state
What the queue/turn system shows:

- idle
- queued
- processing
- completing
- errored

This split should replace vague single-string status where possible.

### Managed-agent record

A managed agent record should likely include:

- stable agent id
- launch command / args / cwd
- whether the bridge is allowed to auto-launch it
- desired channel or binding policy
- creds ref
- process metadata
- bridge join metadata
- last heartbeat / last activity
- last completion metadata
- last error metadata

### Bridge guarantees to clarify

The bridge should explicitly define:

- when it considers launch successful
- when it considers an agent available for work
- when stop is best-effort vs confirmed
- how it marks stale processes after bridge restart
- whether it reattaches to existing processes or only tracks ones it launched

---

## API implications

The current APIs may need to move toward managed resources instead of one-off commands.

Possible future shape:

- `GET /api/agents`
- `POST /api/agents`
- `GET /api/agents/{agentId}`
- `POST /api/agents/{agentId}/start`
- `POST /api/agents/{agentId}/stop`
- `POST /api/agents/{agentId}/restart`
- `POST /api/agents/{agentId}/bind`
- `GET /api/activity`

This is directional, not a locked contract.

---

## Acceptance criteria for this scope

This scope is satisfied when the project has a clear implementation direction such that:

- the bridge UI is explicitly rebuilt around high-signal operational views
- agent management is modeled as a bridge-owned lifecycle system, not just spawn/kill helpers
- desired state, observed state, and binding/join state are separated conceptually
- raw logs are demoted to drill-down/debug surfaces
- the bridge project docs clearly explain the intended operator experience
- the bridge has a path toward safer launch/stop/restart behavior

---

## Recommended next docs/code work

1. Update `SPEC.md` with managed-agent concepts and a more explicit admin/control-plane surface.
2. Update `IMPLEMENTATION-PLAN.md` with a UI rebuild phase and an agent-lifecycle phase.
3. Replace the current dashboard incrementally behind the same HTTP service.
4. Introduce persisted managed-agent records distinct from transient launched-process records.
5. Add explicit health/state computation rather than relying on single status strings.

---

## Summary

The Discord bridge should now evolve from a backend prototype with a debug dashboard into a bridge-owned operations console.

The immediate priority is not more raw telemetry.

The priority is:

- clear bridge state
- trustworthy agent lifecycle management
- useful operator controls
- debug information as a secondary detail layer
