# Coding Agent Prompt: Discord Bridge Interactive Model Picker

## Context

Implement a bridge-owned interactive model picker for the Discord bridge described in:

- `research/discord-bridge/README.md`
- `research/discord-bridge/SCOPE.md`
- `research/discord-bridge/ADMIN-COMMANDS-PROPOSAL.md`
- `research/discord-bridge/SPEC.md`
- `research/discord-bridge/IMPLEMENTATION-PLAN.md`

The bridge already owns Discord connectivity, managed agents, channel bindings, and a slash-command admin layer.

This ticket adds a Discord-native interactive control surface for model selection, using Discord interactions/components rather than the bridge web UI.

---

## Goal

Let an authorized operator change a room agent’s active model from Discord using a bridge-owned interactive picker.

The bridge should:

- own the interaction lifecycle
- render/select from available models
- permission-gate the action
- route the final model change through the bridge control plane
- audit the whole flow

This should feel similar to the existing OpenClaw Discord UX, but remain explicitly bridge-owned.

---

## Strong product preference

Prefer Discord-native interactions such as:

- slash command entrypoint
- select menus
- buttons if helpful
- ephemeral or bridge-authored replies as appropriate

Do not rely on the web UI for the actual picking interaction.

The primary operator workflow should happen inside Discord itself.

---

## Suggested interaction shape

A good first shape is:

### Entry

A slash command such as:

```text
/model
```

or:

```text
/agent <id> model
```

The bridge intercepts it as an admin command.

### Resolution

The bridge determines the target managed agent by one of:

- explicit agent id argument
- current bound room agent for the channel
- a follow-up selection step if ambiguous

### Model picker UI

The bridge responds with a Discord select menu listing available models.

Each option should include enough information to be useful, for example:

- alias / human-friendly label
- provider/model id
- maybe a short note like current/default/fallback

### Optional second step

If useful, a second menu or buttons can set:

- thinking level
- whether the change is temporary or desired/persisted

This second step is optional for the first milestone.

### Completion

After the user chooses a model, the bridge:

- validates authorization
- validates the target agent
- applies the change through the bridge-owned control path
- replies with a bridge-authored success/failure message
- logs the action in the audit trail

---

## First milestone scope

Implement only the smallest coherent slice:

- slash command entrypoint for model picker
- target the bound room agent by default
- render one Discord select menu of available models
- apply the selected model to the target agent
- bridge-authored confirmation response
- permission checks
- audit logging

Optional later work:

- thinking-level picker
- explicit agent selection step
- persistent desired-model vs temporary active-model choice
- model groups/favorites/recent choices

---

## In scope

### 1. Discord interaction handling in the bridge

Add support in the bridge for Discord `InteractionCreate` events and message-component interactions.

The bridge should be able to:

- recognize the model-picker slash command
- send a select menu
- receive the menu selection
- correlate it to the original command context

### 2. Model listing

The bridge must decide where available models come from.

A practical first approach is:

- ask the target room agent / Pi RPC session for available models if possible
- or maintain a bridge-side view of configured models if that is simpler for the first version

Prefer correctness over ideal abstraction.

### 3. Model application

When a model is selected, the bridge should apply it using a bridge-owned operation.

A likely path is:

- bridge sends a targeted control event to the room agent
- room agent maps that to Pi RPC `set_model`

This should follow the same architectural pattern as bridge-controlled slash-command passthrough.

### 4. Authorization

Only authorized users should be able to open or use the picker.

Authorization should be enforced by bridge runtime code, not by Discord UI obscurity.

### 5. Auditability

All model-picker steps should be recorded in bridge audit logs.

---

## Out of scope

- a general-purpose full interaction framework for every bridge feature
- large visual Discord workflows with multiple deep branching menus
- persistent per-user model preferences
- full parity with every OpenClaw Discord UX feature
- web-UI-first model selection

This ticket is specifically about a Discord-native model picker.

---

## Required command/interaction behavior

A likely interaction contract:

### Slash command

Examples:

```text
/model
/model room-1504560627325079642
```

Exact syntax is flexible.

### Response

The bridge sends a select menu with model choices.

### Selection

The user selects one model.

### Bridge action

The bridge applies the model change to the target managed agent.

### Confirmation

The bridge replies with something like:

```text
[discord-bridge]
Agent: room-1504560627325079642
Model changed to: openai/gpt-5.4
```

---

## Data-model direction

The bridge may need a small pending-interaction state model, for example:

- interaction id
- requesting user id
- target agent id
- channel id
- guild id
- created at / expires at
- interaction type = model-picker
- allowed options

This can be in-memory initially if simple and bounded, as long as expiry is handled safely.

---

## Bridge/runtime alignment

The model picker should align with the bridge-controlled slash-command/admin model already proposed.

That means:

- slash command enters the bridge admin layer first
- bridge decides whether to open a picker
- bridge owns aliasing / authorization / audit
- bridge routes the final selected action to the room agent in a controlled form

Do not bypass the bridge and talk directly to Pi from Discord interaction handlers in an ad hoc way.

---

## Suggested implementation shape

### Bridge side

- add `InteractionCreate` handler registration
- add slash command recognition for `/model`
- add component custom-id format for model picker state
- add select-menu rendering helper
- add interaction execution path for model selection

### Agent side

If needed, extend `agent-rpc --bridge` control-event handling so the bridge can send model-change directives, for example:

- `kind: "set_model"`
- payload: provider/model id

The room agent then maps this to Pi RPC `set_model`.

### Audit events

Examples:

- `bridge.model_picker.opened`
- `bridge.model_picker.selected`
- `bridge.model_picker.applied`
- `bridge.model_picker.denied`
- `bridge.model_picker.failed`

---

## Acceptance criteria

- an authorized user can trigger a model picker from Discord
- the bridge responds with a Discord-native selection UI
- the bridge applies the selected model to the intended room agent
- the bridge sends a bridge-authored confirmation or error reply
- unauthorized users are denied cleanly
- the action is logged in the bridge audit trail
- the implementation remains bridge-owned and consistent with the slash-command admin model

---

## Notes for the coding agent

- keep the first implementation narrow and reliable
- prefer one clean model-picker flow over a partially generalized interaction framework
- preserve room for future thinking-level and agent-selection extensions
- align command naming and permissions with the existing bridge admin-command work
- keep the Discord UX understandable and minimal

---

## Summary

Add a bridge-owned interactive model picker in Discord.

Use slash commands plus Discord select menus.
Keep the bridge in control.
Apply the selected model through the bridge’s managed-agent control path.
