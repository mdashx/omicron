# SPIKE Ticket: Manual Backend + Human Fallback Chat

## Context

The Discord bridge already acts as a control plane for Discord ingress/egress, channel bindings, reactions, logs, and agent lifecycle.

This spike explores a fallback mode that keeps the bridge usable even when the LLM/agent path is unavailable, stalled, or not assigned.

This follows the core harness rule:

- the harness must still work if the LLM fails
- a human or script must be able to step in and drive the harness

## Goal

Add a prototype "manual" backend option to the bridge UI and routing model.

When a channel or managed agent is configured with backend = `manual`:

- the bridge should continue to own logging, reactions, bindings, and activity history
- the bridge should still accept inbound Discord messages normally
- reactions should reflect status even if no agent is assigned
- reactions should reflect status even if an assigned agent is not responding
- a human operator can reply through a very barebones web chat UI
- replies sent from the web UI should go back out through the bridge to Discord
- the rest of the system should look and behave the same where possible
- the only visible backend difference should be that the backend is listed as `manual`

## What to build

### 1. Manual backend option

Add `manual` as a valid backend choice in the bridge UI/config model.

This should be treated as a first-class supported mode, not a workaround.

### 2. Reaction fallback behavior

The bridge reaction workflow should still function when:

- no agent is assigned to the channel
- an assigned agent is not responding
- the channel is operating in `manual` mode

Use emoji/status to communicate this clearly.

Example states:

- no agent assigned
- manual operator active
- agent stalled / not responding
- message accepted / in progress / completed

### 3. Barebones manual web chat

Create a new operator screen with an oldschool HTML chat layout.

Requirements:

- very minimal UI
- plain server-rendered HTML is fine
- live updates via JS when a Discord message arrives into the bridge
- operator can type a reply and send it back out
- should feel like a simple control surface, not a new product

### 4. Preserve existing bridge behavior

Do not redesign the rest of the bridge for this spike.

Keep existing behavior for:

- logging
- attachments
- activity/audit history
- agent management
- bindings
- runtime state

The only special handling is that the backend is labeled `manual` and the operator can drive the conversation directly.

## Why this matters

This proves the bridge can be operated as a harness even without a working model path.

It also gives us a safe fallback for:

- agent outages
- model/provider failures
- stalled sessions
- operator takeover
- scripted intervention

## Out of scope

- redesigning the full UI
- rebuilding agent lifecycle management
- changing Discord transport contracts
- adding richer chat features
- full RBAC/admin auth
- fancy frontend frameworks
- replacing the Pi-backed path

## Acceptance criteria

- `manual` appears as a valid backend choice in the UI/model.
- A channel can be operated without an assigned agent.
- Reactions still show meaningful status when no agent is present.
- Reactions still show meaningful status when an agent is stalled or not responding.
- The new manual chat screen receives live inbound Discord messages.
- A human can send a reply from the manual chat screen.
- The reply is delivered back to Discord through the bridge.
- Logging, attachments, and audit behavior remain intact.
- Agent management still works the same for non-manual backends.

## Notes

- Keep the implementation small and prototype-friendly.
- Prefer the simplest mechanism that proves the fallback flow.
- The point is to validate harness operability, not polish.
- The manual backend should be a real operational mode, not a hidden debug path.
