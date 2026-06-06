# OpenClaw Gateway Spec

## Intent

OpenClaw centers on a single always-on **Gateway** process that acts as the control plane for sessions, channels, tools, UI, and node/device connections.

This behavior is enforced by runtime wiring, not by prompting the model to remember it.

---

## 1. `GatewayProcess`

### Prose Spec

The Gateway is a long-lived daemon that owns the main runtime state for OpenClaw. It multiplexes WebSocket control traffic, HTTP APIs, plugin routes, health/readiness, and UI surfaces on one port.

### Z Spec

```text
GatewayProcess
  mode: seq CHAR
  port: ℕ
  bind: seq CHAR
  bindHost: seq CHAR
  authMode: seq CHAR
where
  mode = "local"
  port > 0
  bind ≠ ⟨⟩
  bindHost ≠ ⟨⟩
  authMode ≠ ⟨⟩
```

### Data examples

```json
{
  "mode": "local",
  "port": 18789,
  "bind": "loopback",
  "bindHost": "127.0.0.1",
  "authMode": "token"
}
```

### Implementation suggestions / specifics

- The Gateway startup builds runtime state, method registries, HTTP and WS surfaces, config reload hooks, and shutdown/restart logic.
- It loads channel plugins, gateway methods, plugin HTTP routes, and Control UI assets.
- It watches config and can hot-reload or restart depending on the change type.

---

## 2. `GatewayTransport`

### Prose Spec

The Gateway exposes a **single multiplexed port** for WebSocket control/RPC, HTTP APIs, plugin routes, Control UI, hooks, and readiness/health probes.

### Z Spec

```text
GatewayTransport
  port: ℕ
  surfaces: ℙ seq CHAR
where
  port > 0
  "websocket" ∈ surfaces
  "http-api" ∈ surfaces
```

### Data examples

```json
{
  "port": 18789,
  "surfaces": ["websocket", "http-api", "plugin-http", "control-ui", "health"]
}
```

### Implementation suggestions / specifics

- HTTP and WS share the same server/port.
- The Gateway uses a WebSocket upgrade path plus regular HTTP routing.
- OpenAI-compatible endpoints are first-class compatibility surfaces.

---

## 3. `GatewayHandshake`

### Prose Spec

All WebSocket clients begin with a mandatory `connect` handshake.

The Gateway sends a pre-connect challenge, the client replies with identity, role, scopes, auth, and device metadata, and the Gateway returns a `hello-ok` payload if accepted.

### Z Spec

```text
ROLE ::= operator | node
GatewayHandshake
  role: ROLE
  clientId: seq CHAR
  authMode: seq CHAR
  deviceId?: seq CHAR
where
  role ∈ ROLE
  clientId ≠ ⟨⟩
  authMode ≠ ⟨⟩
```

### Data examples

```json
{
  "role": "operator",
  "clientId": "cli",
  "authMode": "token"
}
```

### Implementation suggestions / specifics

- First frame must be `connect`.
- Non-JSON or pre-connect junk is rejected.
- Side-effecting methods use idempotency protections.
- Some internal same-host backend callers may omit device identity on loopback-only shared-secret connections.

---

## 4. `GatewayAuth`

### Prose Spec

Gateway auth is required by default and depends on bind mode and deployment mode.

Supported auth modes include `token`, `password`, `trusted-proxy`, and `none`.

### Z Spec

```text
AUTHMODE ::= token | password | trusted-proxy | none
GatewayAuth
  mode: AUTHMODE
  sharedSecret?: seq CHAR
where
  mode ≠ ⟨⟩
```

### Data examples

```json
{
  "mode": "token",
  "sharedSecret": "..."
}
```

### Implementation suggestions / specifics

- Non-loopback binds require auth.
- `trusted-proxy` delegates identity to a reverse proxy.
- Tailscale Serve can also satisfy auth in supported setups.
- Failed auth may be rate-limited per client IP/scope.

---

## 5. `GatewayChannels`

### Prose Spec

Channels are provider-specific adapters that connect through the Gateway. They are not the control plane themselves.

A channel may be a bot, webhook transport, bridge, or native device integration.

### Z Spec

```text
GatewayChannels
  channels: ℙ seq CHAR
where
  channels ≠ ∅
```

### Data examples

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "botToken": "123:abc",
      "dmPolicy": "pairing"
    }
  }
}
```

### Implementation suggestions / specifics

- Channels are loaded from config and plugin registry state.
- Channels can be bundled or external plugins.
- DM/group access is controlled by per-channel policy.
- Some channels have startup readiness checks and health monitoring.
- The Gateway may restart channels when config or plugin state changes.

---

## 6. `ChannelPolicy`

### Prose Spec

Channel access is fail-closed by default.

Policies include:

- `dmPolicy`: `pairing`, `allowlist`, `open`, `disabled`
- `groupPolicy`: `open`, `allowlist`, `disabled`
- mention gating and allowlists
- channel/account-specific overrides

### Z Spec

```text
DMPolicy ::= pairing | allowlist | open | disabled
GroupPolicy ::= open | allowlist | disabled
ChannelPolicy
  dmPolicy: DMPolicy
  groupPolicy?: GroupPolicy
```

### Data examples

```json
{
  "channels": {
    "discord": {
      "dmPolicy": "pairing",
      "groupPolicy": "allowlist",
      "allowFrom": ["*"]
    }
  }
}
```

### Implementation suggestions / specifics

- Unknown DM senders usually get pairing flow.
- Group traffic often requires explicit allowlist or mention.
- Some channels have stronger defaults when config is missing.
- Safety rules are enforced by core, not by model behavior.

---

## 7. `GatewayConfig`

### Prose Spec

OpenClaw loads JSON5 config from `~/.openclaw/openclaw.json`.

The Gateway uses strict schema validation and refuses to start on invalid config.

### Z Spec

```text
GatewayConfig
  gateway?: seq CHAR
  channels?: seq CHAR
  agents?: seq CHAR
where
  gateway ≠ ⟨⟩ ∨ channels ≠ ⟨⟩ ∨ agents ≠ ⟨⟩
```

### Data example

```json5
{
  gateway: {
    port: 18789,
    bind: "loopback",
    auth: { mode: "token", token: "..." }
  },
  channels: {
    telegram: { enabled: true }
  }
}
```

### Implementation suggestions / specifics

- Missing config is allowed; defaults are safe.
- Invalid config prevents startup.
- The Gateway keeps a last-known-good snapshot after successful startup.
- Config reload watches the active file and applies changes atomically when safe.

---

## 8. `GatewayHealth`

### Prose Spec

The Gateway exposes health/readiness and service probes for both local operators and automation.

### Z Spec

```text
GatewayHealth
  live: 𝔹
  ready: 𝔹
  rpcReachable: 𝔹
where
  live = ready ∨ ¬ready
```

### Implementation suggestions / specifics

- `openclaw gateway status` summarizes service state and connection health.
- `openclaw channels status --probe` checks per-channel readiness.
- Health and readiness are distinct.
- Startup may delay or retry while sidecars/providers initialize.

---

## 9. `GatewayLifecycle`

### Prose Spec

The Gateway can be run foreground or installed as a service.

Supported service managers include launchd, systemd, and schtasks.

### Z Spec

```text
GatewayLifecycle
  serviceManager: seq CHAR
  running: 𝔹
where
  serviceManager ≠ ⟨⟩
```

### Implementation suggestions / specifics

- `openclaw gateway install`
- `openclaw gateway start`
- `openclaw gateway stop`
- `openclaw gateway restart`
- `openclaw gateway uninstall`
- `openclaw gateway --force` can kill old listeners on the target port

---

## 10. `GatewayInvariants`

### Prose Spec

- One gateway controls one host-side runtime boundary
- Handshake is mandatory
- Auth applies to all connections
- Channel traffic is mediated through the Gateway
- Events are not replayed; clients refresh on gaps
- Fail closed on invalid config or missing required auth

### Z Spec

```text
GatewayInvariants
  singleGatewayPerHost: 𝔹
  handshakeRequired: 𝔹
  authRequiredByDefault: 𝔹
  eventReplay: 𝔹
where
  singleGatewayPerHost = true
  handshakeRequired = true
  authRequiredByDefault = true
  eventReplay = false
```
