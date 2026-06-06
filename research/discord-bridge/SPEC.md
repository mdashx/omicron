# Discord Bridge Service Spec

## Intent

Add an always-on Discord bridge service that connects to a Discord bot and acts as a shared ingress/egress layer for Pi-compatible agents and other clients.

An agent with valid credentials/config should be able to join the bridge, bind to one Discord channel, read shared logs/downloads, and actively listen/respond on that assigned channel.

This must be enforced by the bridge runtime, not by prompting agents to self-police.

---

## 1. `DiscordBridgeService`

### Prose Spec

The bridge is a long-lived daemon that maintains the Discord bot connection, agent registry, shared storage, and channel routing.

It is the stable control plane for all Discord-mediated agent traffic.

### Z Spec

```text
DiscordBridgeService
  enabled: 𝔹
  botToken: seq CHAR
  bridgeId: seq CHAR
  host: seq CHAR
  port: ℕ
  storageRoot: seq CHAR
where
  enabled = true
  botToken ≠ ⟨⟩
  bridgeId ≠ ⟨⟩
  host ≠ ⟨⟩
  port > 0
  storageRoot ≠ ⟨⟩
```

### Data examples

```json
{
  "enabled": true,
  "bridgeId": "discord-bridge-main",
  "host": "127.0.0.1",
  "port": 19444,
  "storageRoot": "~/.pi/discord-bridge",
  "botToken": "***"
}
```

### Implementation suggestions / specifics

- Start automatically as a daemon/service.
- Keep the Discord bot connection alive.
- Expose local RPC/HTTP for agent join, leave, and status.
- Load configuration before accepting agent joins.
- Fail closed if the bot token or storage root is missing.

---

## 2. `DiscordBridgeConfig`

### Prose Spec

The bridge should have a built-in default configuration, with overrides from config or environment variables.

It should support a single shared Discord bot plus per-agent bindings.

### Z Spec

```text
DiscordBridgeConfig
  enabled?: 𝔹
  botToken?: seq CHAR
  bridgeId?: seq CHAR
  host?: seq CHAR
  port?: ℕ
  storageRoot?: seq CHAR
  defaultChannelId?: seq CHAR
  defaultGuildId?: seq CHAR
```

### Data examples

```json5
{
  discordBridge: {
    enabled: true,
    botToken: "...",
    bridgeId: "discord-bridge-main",
    storageRoot: "~/.pi/discord-bridge",
    defaultGuildId: "1234567890",
    defaultChannelId: "9876543210"
  }
}
```

### Implementation suggestions / specifics

- Merge user overrides onto safe defaults.
- Treat the storage root as a separate runtime home, not the agent workspace.
- Permit environment-variable auth injection for deployment convenience.
- Keep Discord bridge config outside the prompt path.

---

## 3. `DiscordAgentIdentity`

### Prose Spec

Every agent joining the bridge must have a stable identity.

The bridge must know which agent is connecting, which credentials it used, and which Discord channel it is bound to.

### Z Spec

```text
DiscordAgentIdentity
  agentId: seq CHAR
  bridgeId: seq CHAR
  discordUserId: seq CHAR
  guildId: seq CHAR
  channelId: seq CHAR
  joinedAt: seq CHAR
where
  agentId ≠ ⟨⟩
  bridgeId ≠ ⟨⟩
  discordUserId ≠ ⟨⟩
  guildId ≠ ⟨⟩
  channelId ≠ ⟨⟩
  joinedAt ≠ ⟨⟩
```

### Data examples

```json
{
  "agentId": "main",
  "bridgeId": "discord-bridge-main",
  "discordUserId": "112233445566",
  "guildId": "1234567890",
  "channelId": "9876543210",
  "joinedAt": "2026-06-06T10:15:00Z"
}
```

### Implementation suggestions / specifics

- Bind each agent to exactly one active Discord channel.
- Persist the identity mapping so reconnects restore the prior binding.
- Reject joins that lack proper credentials or refer to an unrecognized agent.
- Allow the bridge to track multiple agents, but keep each agent channel-bound.

---

## 4. `DiscordChannelBinding`

### Prose Spec

A binding connects exactly one agent instance to exactly one Discord channel.

The bridge routes inbound channel messages to the bound agent and publishes agent output back to that same channel.

### Z Spec

```text
DiscordChannelBinding
  bindingId: seq CHAR
  agentId: seq CHAR
  guildId: seq CHAR
  channelId: seq CHAR
  active: 𝔹
where
  bindingId ≠ ⟨⟩
  agentId ≠ ⟨⟩
  guildId ≠ ⟨⟩
  channelId ≠ ⟨⟩
```

### Data examples

```json
{
  "bindingId": "bind_001",
  "agentId": "main",
  "guildId": "1234567890",
  "channelId": "9876543210",
  "active": true
}
```

### Implementation suggestions / specifics

- Only one active agent may own a channel binding at a time.
- If an agent disconnects, the binding may be retained as dormant or released.
- Route all inbound messages for the channel to the bound agent.
- Route all outbound replies for that agent back to the same channel unless explicitly overridden.

---

## 5. `DiscordBridgeJoin`

### Prose Spec

An agent joins the bridge by presenting credentials and a desired binding/config.

The bridge verifies the agent, loads its scoped permissions, restores its storage context, and starts message delivery.

### Z Spec

```text
DiscordBridgeJoin
  agentId: seq CHAR
  credsRef: seq CHAR
  requestedGuildId?: seq CHAR
  requestedChannelId?: seq CHAR
  scope: ℙ seq CHAR
where
  agentId ≠ ⟨⟩
  credsRef ≠ ⟨⟩
```

### Data examples

```json
{
  "agentId": "research-bot",
  "credsRef": "file:~/.pi/discord-bridge/creds/research-bot.json",
  "requestedGuildId": "1234567890",
  "requestedChannelId": "9876543210",
  "scope": ["read.logs", "read.downloads", "channel.listen", "channel.reply"]
}
```

### Implementation suggestions / specifics

- Join should be explicit and idempotent.
- A join may restore a prior binding or create a new one.
- Scope should be resolved by the bridge, not assumed by the agent.
- Deny joins that request a channel already owned by another active agent.

---

## 6. `DiscordSharedStore`

### Prose Spec

The bridge owns shared logs and downloads.

Agents with the right scope can read from those stores through the bridge.

### Z Spec

```text
DiscordSharedStore
  logsRoot: seq CHAR
  downloadsRoot: seq CHAR
  historyLimit: ℕ
where
  logsRoot ≠ ⟨⟩
  downloadsRoot ≠ ⟨⟩
  historyLimit > 0
```

### Data examples

```json
{
  "logsRoot": "~/.pi/discord-bridge/logs",
  "downloadsRoot": "~/.pi/discord-bridge/downloads",
  "historyLimit": 5000
}
```

### Implementation suggestions / specifics

- Store channel logs as append-only records.
- Store downloaded files in a bridge-owned directory tree.
- Expose history lookup and file retrieval through bridge RPC.
- Keep agent access scoped to bridge policy, not raw filesystem access.

---

## 7. `DiscordInboundMessage`

### Prose Spec

Inbound Discord messages are normalized by the bridge before delivery to an agent.

The bridge must preserve the author, channel, message id, timestamps, attachments, and reply/thread context.

### Z Spec

```text
DiscordInboundMessage
  messageId: seq CHAR
  channelId: seq CHAR
  authorId: seq CHAR
  content: seq CHAR
  timestamp: seq CHAR
where
  messageId ≠ ⟨⟩
  channelId ≠ ⟨⟩
  authorId ≠ ⟨⟩
  timestamp ≠ ⟨⟩
```

### Data examples

```json
{
  "messageId": "discord_msg_1",
  "channelId": "9876543210",
  "authorId": "112233445566",
  "content": "summarize the latest log",
  "timestamp": "2026-06-06T10:16:20Z"
}
```

### Implementation suggestions / specifics

- Normalize Discord message shapes into a stable internal event format.
- Preserve thread and reply metadata when available.
- Attachments should be downloaded or proxied into the bridge store.
- Route only to the agent bound to the channel.

---

## 8. `DiscordOutboundDelivery`

### Prose Spec

The bridge publishes agent responses back to the assigned Discord channel.

It should support plain text, attachments, and simple reply threading.

### Z Spec

```text
DiscordOutboundDelivery
  agentId: seq CHAR
  channelId: seq CHAR
  content: seq CHAR
  sentAt: seq CHAR
where
  agentId ≠ ⟨⟩
  channelId ≠ ⟨⟩
  sentAt ≠ ⟨⟩
```

### Data examples

```json
{
  "agentId": "research-bot",
  "channelId": "9876543210",
  "content": "Here’s the summary.",
  "sentAt": "2026-06-06T10:17:02Z"
}
```

### Implementation suggestions / specifics

- Respect channel binding on all outbound sends.
- Preserve reply/thread context if supported.
- Queue outbound messages when Discord rate limits or reconnects happen.
- Record every outbound delivery in the audit log.

---

## 9. `DiscordBridgeReactiveHarness`

### Prose Spec

The bridge may provide repeatable, model-independent structure around a message before and after the LLM turn.

A common default behavior is:

- add a basic acknowledgement reaction when a message is first accepted
- update that reaction to reflect LLM progress/status while the turn is running
- offer the LLM a constrained choice of final emoji reactions to leave when the final reply is ready

This makes the bridge behave a bit like a harness: the bridge provides durable interaction structure, while the agent/model provides the content.

### Z Spec

```text
DiscordBridgeReactiveHarness
  ackReaction: seq CHAR
  statusReactions: ℙ seq CHAR
  finalReactionChoices: ℙ seq CHAR
  enabled: 𝔹
where
  enabled = true
  ackReaction ≠ ⟨⟩
  statusReactions ≠ ∅
  finalReactionChoices ≠ ∅
```

### Data examples

```json
{
  "enabled": true,
  "ackReaction": "✅",
  "statusReactions": ["⏳", "🤖", "💭", "✅", "⚠️"],
  "finalReactionChoices": ["✅", "👍", "👀", "🧠", "❤️"]
}
```

### Implementation suggestions / specifics

- Add an acknowledgement reaction as soon as the bridge accepts a post for processing.
- Update the active reaction as the agent transitions through queued, thinking, tool use, waiting, or finished states.
- When the final reply is ready, expose a small bridge-defined reaction palette the LLM can choose from.
- Keep the reaction protocol bridge-owned so behavior stays consistent across agents and models.
- This pattern may extend to other repeatable bridge behaviors, such as read receipts, progress markers, routing banners, retry notices, and attachment lifecycle indicators.

---

## 10. `DiscordBridgeAuditLog`

### Prose Spec

Every join, message, file access, routing decision, and disconnect should be audit-logged.

The audit log should be append-only and recoverable across restarts.

### Z Spec

```text
DiscordBridgeAuditLog
  auditRoot: seq CHAR
  appendOnly: 𝔹
where
  auditRoot ≠ ⟨⟩
  appendOnly = true
```

### Data examples

```json
{
  "auditRoot": "~/.pi/discord-bridge/audit",
  "appendOnly": true
}
```

### Implementation suggestions / specifics

- Append one event per line.
- Include join/leave, channel binding, downloads, and message fanout.
- Keep audit records separate from agent session history.
- Preserve records after bridge restart.

---

## 11. `DiscordBridgeInvariants`

### Prose Spec

- One bridge controls one Discord bot connection
- One active agent owns one channel binding at a time
- Agent joins are explicit and credentialed
- Shared logs/downloads are bridge-owned
- Inbound messages route only to the assigned agent
- Outbound replies go back to the assigned channel
- Audit logging is append-only

### Z Spec

```text
DiscordBridgeInvariants
  alwaysOn: 𝔹
  singleBotConnection: 𝔹
  oneAgentPerChannel: 𝔹
  appendOnlyAudit: 𝔹
where
  alwaysOn = true
  singleBotConnection = true
  oneAgentPerChannel = true
  appendOnlyAudit = true
```

---

## Summary

This design makes Discord the high-trust ingress and keeps Pi/agents as clients of a separate always-on bridge.

The bridge owns:

- Discord bot connectivity
- agent join/binding
- message routing
- shared logs/downloads
- audit history

Agents only need to know how to join, listen, and respond.
