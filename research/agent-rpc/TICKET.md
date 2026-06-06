# Coding Agent Prompt: Agent RPC Harness

## Context

Implement the narrow first version of the agent RPC harness described in:

- `research/agent-rpc/SCOPE.md`
- `research/agent-rpc/SPEC.md`
- `research/agent-rpc/IMPLEMENTATION-PLAN.md`

The current scope is intentionally narrow:

- upstream: `Pi` via `pi --mode rpc`
- downstream: a very simple local CLI tool only

Do not build Discord integration yet.

## Goal

Add a small harness prototype that launches Pi in RPC mode, preserves session continuity, consumes Pi’s structured RPC events, and exposes a very simple local CLI on the other end.

The implementation must be enforced by runtime code, not by prompt text.

## What to optimize for

- correct RPC startup wiring
- clear runtime boundaries
- minimal but durable defaults
- no PTY dependency for normal success
- no log-scraping dependency for normal success
- readable structure that makes session continuity and event flow obvious
- the smallest coherent vertical slice that proves the architecture

## Where to build it

Build the prototype as a self-contained project in `research/agent-rpc/` alongside this `TICKET.md`.

That directory should be the working home for:

- design docs
- implementation scaffolding
- any small runnable prototype code
- local CLI entry point
- test fixtures or logs if needed

If the code later graduates into a package or core integration, keep the prototype source tree here first.

## Where to look first in the codebase

Start with these docs and nearby code:

- `packages/coding-agent/docs/rpc.md`
  - primary reference for Pi RPC commands, responses, and events

- `packages/coding-agent/docs/json.md`
  - helpful comparison point for Pi event streaming

- `packages/coding-agent/docs/session-format.md`
  - useful for understanding session identity and continuity

- `research/discord-bridge-client/pi_logs.go`
  - prior structured-output work; useful as contrast and fallback thinking, not the primary path

- `research/discord-bridge-client/pty_agent.go`
  - prior PTY fallback work; useful mainly as anti-goal for the first path

- `packages/coding-agent/src/core/session-manager.ts`
  - useful for understanding how Pi persists sessions

- `packages/coding-agent/src/main.ts`
  - useful context for how Pi is launched/configured in normal usage

## Suggested implementation shape

This is a suggested shape, not a prescription.

1. Resolve a built-in harness default config.
2. Launch Pi with `--mode rpc`.
3. Implement strict JSONL framing for stdin/stdout.
4. Send one `prompt` command.
5. Read responses and events.
6. Capture final assistant text.
7. Print minimal CLI status and final output.
8. Preserve `sessionId` and `sessionFile` using `get_state`.
9. Reuse the same Pi session across repeated prompts.
10. Add basic busy handling using `steer` when Pi is already streaming.

## Example code direction

The concrete implementation will likely want something like this shape:

```ts
const harness = await createHarness({
  command: "pi",
  args: ["--mode", "rpc"],
  cwd: process.cwd(),
});

await harness.start();
const state = await harness.getState();
console.log("session:", state.sessionId, state.sessionFile);

const result = await harness.prompt("List files in the current directory");
console.log(result.text);
```

A richer internal design is fine if it makes the event flow clearer. The important thing is to keep the first working slice small and coherent.

## CLI expectations

The local CLI can be extremely simple.

Acceptable first versions include:

- one-shot mode that takes a prompt and prints the final answer
- a tiny REPL loop that accepts repeated prompts
- a line-oriented stdin/stdout interface

The CLI does not need:

- fancy rendering
- ncurses/TUI behavior
- syntax highlighting
- tool streaming display
- visible thinking display

A plain, reliable CLI is preferred.

## What not to do yet

Do not add these in the first implementation:

- Discord bridge integration
- Claude integration
- Codex integration
- PTY-first execution
- log-tailing-first execution
- rich token streaming UI
- visible thinking display by default
- generalized multi-backend abstraction beyond what the current scope needs

## Acceptance criteria

- The harness can start with Pi as the upstream using `--mode rpc`.
- The harness can send a `prompt` and receive a final assistant reply.
- The CLI can show simple started/working/completed or error output.
- The harness can query and retain Pi session state such as `sessionId` and `sessionFile`.
- Repeated prompts in the same harness instance reuse the same Pi session.
- The harness does not require PTY scraping or log tailing for normal success.
- Tool lifecycle and thinking events are captured internally when available, even if not displayed.
- The code stays readable enough that an onlooker can understand where the control path lives and why.

## Notes for the coding agent

- Read the docs first, especially `packages/coding-agent/docs/rpc.md`.
- Prefer the smallest coherent implementation that proves the architecture.
- Use runtime code for behavior; do not rely on prompt conventions.
- Keep room for future downstreams, but do not generalize prematurely.
- If a clean first slice is one-shot CLI before REPL, that is acceptable.
