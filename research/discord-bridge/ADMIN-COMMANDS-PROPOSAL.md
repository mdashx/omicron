# Discord Bridge Proposal: Bridge-Controlled Slash Commands

## Intent

Add a bridge-controlled administrative command surface for Discord using slash-style commands.

The key requirement is:

- slash commands in Discord should not go straight to Pi by default
- they should first enter a bridge-controlled admin command layer
- the bridge can then:
  - interpret bridge-native commands
  - resolve aliases
  - permission-gate access
  - optionally pass through selected commands to Pi
  - audit the result as bridge-owned behavior

This preserves the familiar slash-command UX while keeping the bridge as the control plane.

---

## Problem

Today, Discord messages are wrapped as normal agent prompts before being sent into `agent-rpc`.

That means a Discord message like:

```text
/new
```

is currently treated as ordinary user text, not as a Pi slash command.

That is too limited for operator control, but sending slash commands directly to Pi without bridge mediation would also be incomplete because it would remove an important control point.

We want the bridge to own the decision about what a slash command means.

---

## Proposal

Treat leading slash commands as input to a bridge-controlled admin command process.

When the bridge sees a Discord message beginning with `/`, it should:

1. recognize it as a command candidate
2. route it into a bridge-owned command parser/executor
3. decide whether it is:
   - a bridge-native command
   - an alias for another command
   - an allowed passthrough command
   - a denied or unknown command
4. enforce permissions
5. execute or forward accordingly
6. emit a bridge-authored response
7. write an audit record

So the slash UX is preserved, but the bridge remains in charge.

---

## Core model

### Ordinary messages

Messages that do **not** begin with `/` continue to follow the current path:

- normalized by the bridge
- wrapped as bridge context
- sent to the bound Pi agent as normal prompt content

### Slash commands

Messages that **do** begin with `/` are intercepted by the bridge admin command layer first.

They are not treated as ordinary user prompts by default.

---

## Why this is the right compromise

This design combines the benefits of both positions:

### What it keeps

- natural Discord slash-command UX
- compatibility with OpenClaw-style expectations
- the possibility of Pi slash-command passthrough

### What it adds

- bridge-owned aliasing
- bridge-native commands
- permission gates
- explicit auditing
- runtime control over what is and is not allowed

This is better than both extremes:

- better than treating `/...` as plain wrapped text
- better than blindly forwarding every `/...` command straight to Pi

---

## Command classification

A slash command received by the bridge should be classified into one of several categories.

### 1. Bridge-native commands

These are handled entirely by the bridge runtime.

Examples:

```text
/status
/agents
/bindings
/activity
/agent room-1504560627325079642 status
/agent room-1504560627325079642 restart
/reconcile
```

These do not need Pi involvement.

### 2. Aliases

The bridge may rewrite or expand friendly commands into other bridge-native or passthrough commands.

Examples:

```text
/rooms           -> /agents
/room restart x  -> /agent x restart
/health          -> /status
```

This gives the bridge a stable UX layer even if the underlying implementation changes.

### 3. Allowed passthrough commands

These are slash commands that the bridge is willing to forward to Pi after permission checks.

Examples might include:

```text
/new
/compact
/state
/model
```

But importantly, these commands would only be forwarded after bridge review.

The bridge may:

- allow some
- deny some
- rewrite some
- scope some to particular users/channels/agents

### 4. Denied or unknown commands

If a slash command is not known, not allowed, or not authorized, the bridge should reject it explicitly with a bridge-authored response.

---

## Suggested routing behavior

When a Discord message arrives:

1. if it does not start with `/`, use the normal prompt-routing path
2. if it starts with `/`, route it to the bridge command layer
3. parse the command
4. check authorization
5. classify it as:
   - bridge-native
   - alias
   - allowed passthrough
   - denied/unknown
6. execute the corresponding action
7. send a bridge-authored response
8. log the action in the audit trail

This should be mutually exclusive with ordinary prompt routing.

So:

- slash commands should not also be forwarded as normal chat content

---

## Bridge-controlled admin process

The phrase “admin process” does not need to imply a separate OS process immediately.

It can begin as a bridge-owned command handler in the bridge runtime.

Conceptually, though, it should act like a separate control-plane subsystem with:

- parsing
- classification
- authorization
- dispatch
- reply formatting
- audit logging

Later, this could become a more distinct internal module or service boundary if useful.

---

## Suggested syntax

Use direct slash syntax as the visible user-facing command language.

Examples:

```text
/help
/status
/agents
/bindings
/agent room-1504560627325079642 status
/agent room-1504560627325079642 restart
/new
/state
```

The bridge decides what each means.

This preserves the UX you want while still keeping interpretation under bridge control.

---

## Permissions model

Slash commands should be permission-gated in bridge runtime code.

The bridge should not assume every user can invoke every command.

Possible permission dimensions:

- Discord user id
- channel id
- guild id
- DM vs guild channel
- command class
- target agent

### Suggested first cut

A simple first pass could classify commands into permission tiers:

#### Read-only bridge commands
Examples:

- `/help`
- `/status`
- `/agents`
- `/bindings`
- `/activity`

These might be allowed more broadly.

#### Mutating bridge commands
Examples:

- `/agent <id> start`
- `/agent <id> stop`
- `/agent <id> restart`
- `/bind ...`
- `/reconcile`

These should be more restricted.

#### Pi passthrough commands
Examples:

- `/new`
- `/compact`
- `/state`
- `/model`

These should likely be gated separately from bridge-native commands because they affect agent session/runtime state directly.

---

## Passthrough policy

The bridge should not automatically pass through every slash command.

Instead, it should define an explicit passthrough policy.

For example:

- allowlisted commands only
- per-channel or per-agent allowlists
- optionally per-user allowlists

Examples of policy questions:

- Is `/new` allowed remotely at all?
- Is `/compact` allowed in shared channels?
- Is `/model` allowed only for admins?
- Should `/state` be visible to everyone or only operators?

These are bridge policy decisions.

---

## Alias resolution

Alias support is one of the best reasons to keep slash commands bridge-controlled.

Examples:

```text
/restart              -> /agent <bound-agent> restart
/reset                -> /new
/health               -> /status
/queue                -> /agent <bound-agent> queue
```

This lets the bridge present a cleaner command language without exposing raw implementation details.

It also gives room for room-specific semantics later.

---

## Bridge-native command examples

A strong initial set of bridge-native slash commands could be:

- `/help`
- `/status`
- `/agents`
- `/bindings`
- `/activity`
- `/agent <id> status`
- `/agent <id> restart`
- `/agent <id> stop`
- `/agent <id> start`

These align directly with the web UI and HTTP admin actions.

---

## Pi passthrough examples

A narrow initial set of passthrough commands could be:

- `/new`
- `/compact`
- `/state`

Even here, the bridge should remain responsible for:

- deciding whether passthrough is allowed
- selecting the target agent/session
- capturing the outcome for audit
- formatting any errors or denials cleanly

---

## Response shape

All slash-command responses should be visibly bridge-authored.

Example:

```text
[discord-bridge]
Agent: room-1504560627325079642
Desired: running
Process: running
Bridge: bound
Queue: 0
Last activity: 12s ago
```

For passthrough commands, the bridge may still note that the command was forwarded.

Example:

```text
[discord-bridge]
Forwarded to Pi: /new
Result: session reset requested
```

This avoids ambiguity about whether the reply came from the bridge or from ordinary agent chat.

---

## Audit / observability

Every slash command should generate bridge audit records such as:

- `bridge.admin.received`
- `bridge.admin.alias_resolved`
- `bridge.admin.authorized`
- `bridge.admin.denied`
- `bridge.admin.executed`
- `bridge.admin.forwarded`
- `bridge.admin.failed`

Payloads should include at least:

- author id
- author name
- guild id
- channel id
- raw command text
- normalized/parsed command
- classification
- target agent
- result

This is a major advantage over naive direct passthrough.

---

## UI and API alignment

The bridge-controlled slash-command surface should map onto the same conceptual operations as the bridge UI and HTTP API.

That means:

- `/status` aligns with overview
- `/agents` aligns with managed agents
- `/bindings` aligns with bindings
- `/agent <id> restart` aligns with lifecycle controls

This keeps the operator experience coherent across:

- Discord
- web UI
- HTTP control endpoints

---

## Tradeoffs

### Pros

- preserves familiar slash-command UX
- keeps the bridge in control
- enables aliases
- enables bridge-native commands
- supports explicit passthrough policy
- improves auditability
- improves permission handling

### Cons

- requires a command parser/classifier in the bridge
- requires clear command documentation
- introduces some policy complexity around passthrough

These are acceptable tradeoffs for a trustworthy control plane.

---

## Suggested first milestone

Implement:

### Bridge-native
- `/help`
- `/status`
- `/agents`
- `/bindings`
- `/agent <id> status`
- `/agent <id> restart`

### Passthrough
- `/new`
- `/state`

### Runtime rules
- slash commands go to bridge command handling first
- passthrough uses an explicit allowlist
- permissions are checked in bridge code
- results are bridge-authored and audited

This is enough to prove the shape without needing the full command universe immediately.

---

## Acceptance criteria

This proposal is satisfied when:

- slash commands are recognized by the bridge as a separate control path
- the bridge parses and classifies slash commands before any forwarding
- bridge-native commands are executed by the bridge itself
- aliases can be resolved by the bridge
- passthrough commands are explicit and permission-gated
- slash command actions are audited in bridge runtime logs
- slash-command behavior aligns with the web UI and HTTP admin model

---

## Summary

Use slash commands directly in Discord, but do not let them bypass the bridge.

Instead:

- slash commands enter a bridge-controlled admin command layer
- the bridge decides whether each command is:
  - bridge-native
  - an alias
  - an allowed passthrough
  - denied

This gives you the familiar OpenClaw-style slash UX while preserving the bridge as the owner of permissions, aliases, policy, audit, and control-plane semantics.
