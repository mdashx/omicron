# Discord Bridge Proposal: Bridge-Owned Admin Commands

## Intent

Add a bridge-owned administrative command surface for Discord that is separate from ordinary chat messages and separate from raw Pi slash-command passthrough.

This proposal corresponds to option 3 from the current discussion:

- keep ordinary Discord chat routed as normal agent prompts
- do not treat leading `/` messages as raw Pi terminal commands by default
- introduce explicit bridge-owned admin commands that the bridge interprets itself

The bridge should remain the control plane.

---

## Problem

Today, Discord messages are wrapped as normal agent prompts before being sent into `agent-rpc`.

That means:

- a Discord message like `/new` is not a Pi slash command
- it becomes ordinary user text inside a structured bridge wrapper
- Pi sees it as content, not as terminal control input

That is currently correct for safety and determinism, but it creates a gap:

- operators still need a remote way to manage the bridge and its agents from Discord
- some of those operations should be handled by the bridge itself, not by Pi
- using raw Pi slash commands over Discord would blur the control boundary

---

## Proposal

Introduce a dedicated bridge-owned admin command namespace.

These commands are parsed and executed by the Discord bridge runtime itself before normal agent routing.

They should not be forwarded to Pi as prompt text unless explicitly designed to do so.

This creates a separate control plane with properties that are:

- explicit
- bridge-enforced
- auditable
- safer than raw slash-command passthrough
- easier to reason about than magical prompt handling

---

## Command model

### Suggested syntax

Use a distinct prefix that clearly means “bridge command”, for example:

```text
!bridge ...
```

or possibly:

```text
.bridge ...
```

This proposal uses `!bridge` for examples.

### Examples

```text
!bridge help
!bridge status
!bridge agents
!bridge agent room-1504560627325079642 status
!bridge agent room-1504560627325079642 restart
!bridge bindings
!bridge channel status
!bridge logs room-1504560627325079642
```

### Why not raw `/...`

Discord users often associate `/` with app commands or command-like syntax.

Using `/...` as a hidden raw pass-through to Pi would be ambiguous and risky because:

- it is not obvious whether the bridge or Pi owns the command
- it weakens bridge/runtime boundaries
- it increases the chance of accidental terminal-like control messages being sent to agents
- it makes auditing and permissions less clear

A bridge-specific prefix keeps the control surface visibly separate.

---

## Scope of bridge-owned admin commands

These commands should manage the bridge and its managed agents, not Pi internals directly.

### Good candidates

#### Bridge status

- `!bridge help`
- `!bridge status`
- `!bridge activity`
- `!bridge bindings`
- `!bridge agents`

#### Managed agent lifecycle

- `!bridge agent <id> status`
- `!bridge agent <id> start`
- `!bridge agent <id> stop`
- `!bridge agent <id> restart`

#### Channel/binding state

- `!bridge bind <agentId> <channelId>`
- `!bridge unbind <agentId>`
- `!bridge channel status`

#### Debug / inspection

- `!bridge logs <agentId>`
- `!bridge audit tail`
- `!bridge queue <agentId>`

#### Optional future commands

- `!bridge rescan rooms`
- `!bridge autostart on`
- `!bridge autostart off`
- `!bridge reconcile`

---

## Out of scope

The bridge-owned admin surface should not initially try to expose arbitrary Pi slash commands such as:

- `/new`
- `/compact`
- `/state`
- `/model`

Those are Pi runtime concerns, not bridge control-plane concerns.

If later needed, a very narrow explicit bridge command could request some equivalent high-level lifecycle action, but the bridge should remain the owner of the policy and interpretation.

---

## Permissions model

Bridge admin commands should not be available to every Discord user by default.

The bridge should check permissions in runtime code before executing admin commands.

Possible policies:

- allow only configured Discord user ids
- allow only configured channels
- allow only DMs from paired/approved users
- optionally allow specific guild roles later

A first implementation can stay simple:

- only permit bridge admin commands from approved user ids already present in the bridge/channel config
- optionally restrict them to specific channels or DMs

The important part is that permission is bridge-owned and enforced in code.

---

## Routing behavior

When a Discord message arrives:

1. the bridge checks whether it matches the admin-command prefix
2. if not, it follows normal inbound agent routing
3. if yes, the bridge parses and authorizes it
4. the bridge executes the command itself
5. the bridge replies with a bridge-authored response
6. the bridge records the action in the audit log

This should be mutually exclusive with normal agent prompt routing for that message.

In other words:

- admin commands do not also get forwarded to the bound agent

---

## Response shape

Bridge-owned admin command replies should be visibly bridge-authored.

For example:

```text
[discord-bridge]
Agent: room-1504560627325079642
Desired: running
Process: running
Bridge: bound
Queue: 0
Last activity: 12s ago
```

This helps users understand that the response came from the bridge control plane, not from Pi.

---

## UI and API alignment

The new Discord admin command surface should align with the bridge web UI and HTTP actions.

That means bridge admin commands should map onto the same conceptual operations as the UI:

- overview/status
- managed agents
- bindings
- activity
- lifecycle actions

This reduces drift between:

- local web UI
- HTTP control actions
- Discord-side operator control

The bridge should not invent a completely separate hidden behavior model for Discord.

---

## Audit / observability

Every admin command should produce bridge audit records such as:

- `bridge.admin.received`
- `bridge.admin.authorized`
- `bridge.admin.denied`
- `bridge.admin.executed`
- `bridge.admin.failed`

Payloads should include at least:

- author id
- author name
- channel id
- guild id
- command text or parsed command
- result status

This is a strong advantage over raw Pi slash-command passthrough.

---

## Why this is preferable to raw Pi slash passthrough

### 1. Better separation of concerns

The bridge owns bridge control.
Pi owns model/session work.

### 2. Safer remote control

The bridge can whitelist exact operations instead of allowing arbitrary command-like input.

### 3. Better auditability

Bridge actions are explicit and structured.

### 4. Easier permissions

The bridge can gate admin commands independently of agent prompting.

### 5. Better UX

Users can distinguish:

- asking the agent to do work
- asking the bridge to manage infrastructure

---

## Tradeoffs

### Pros

- safer
- clearer
- easier to document
- runtime-enforced
- easier to align with the UI and API

### Cons

- not a direct substitute for every Pi slash command
- requires bridge-specific command parsing
- introduces another operator-facing command surface to maintain

These tradeoffs are acceptable if the goal is a trustworthy bridge control plane.

---

## Suggested first milestone

Implement a narrow first set of bridge-owned admin commands:

- `!bridge help`
- `!bridge status`
- `!bridge agents`
- `!bridge bindings`
- `!bridge agent <id> status`
- `!bridge agent <id> restart`

This is enough to prove the pattern without overextending the parser.

---

## Acceptance criteria

This proposal is satisfied when:

- the bridge defines a dedicated admin command prefix
- admin commands are parsed by the bridge, not forwarded as agent prompts
- permissions are enforced in bridge runtime code
- responses are bridge-authored and clearly labeled
- actions are logged in the bridge audit trail
- the initial command set maps cleanly onto existing UI/HTTP concepts

---

## Summary

Do not overload ordinary Discord chat or raw `/...` messages with hidden bridge semantics.

Instead, add a bridge-owned admin command surface.

This preserves:

- clean bridge/runtime boundaries
- safer operations
- clearer user intent
- better auditability
- future alignment between Discord control, web UI, and HTTP APIs
