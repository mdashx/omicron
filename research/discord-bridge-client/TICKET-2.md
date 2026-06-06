# Coding Agent Prompt: Discord Bridge Client Structured Pi Output

## Context

The Discord bridge client spec has been updated so Pi-backed agents still receive inbound work through PTY input, but outbound assistant replies should come from Pi's structured JSONL logs instead of PTY screen scraping.

Primary spec:
- `research/discord-bridge-client/SPEC.md`

This ticket exists only to explain the spec change and the intended implementation direction. It does not change the implementation plan or the original ticket.

## What changed in the spec

The spec now makes these runtime boundaries explicit:

- the real agent process still runs in a PTY
- bridge-originated prompts are still injected through PTY input
- the agent should still feel like it is talking only to a normal terminal session
- for Pi-backed agents, outbound reply extraction should come from Pi JSONL logs
- PTY output is now a fallback/debug surface, not the preferred authoritative output source for Pi replies

## New spec concepts

### `BridgeClientConfig`
The config shape now allows structured Pi log discovery settings such as:
- `outputMode`
- `piSessionRoot`
- `piSessionArchiveRoot`
- `piLogPreference`

### `PiStructuredOutputSource`
The spec now introduces a harness-owned registration concept for the active Pi log source. The harness should discover and register the JSONL file(s) associated with the launched Pi process.

This can point at:
- native Pi session logs under `~/.pi/agent/sessions`
- session archive logs under `~/.pi/agent/session-archive`

The preferred order is:
1. session-archive JSONL
2. native Pi session JSONL
3. PTY fallback only if structured logs are unavailable or the command is not Pi

## Why this change exists

PTY scraping is a poor primary output source for Pi because terminal output contains:
- ANSI escape sequences
- cursor movement
- UI redraw noise
- prompt/footer text
- ambiguous reply boundaries

Pi's JSONL logs are a better integration boundary because they provide structured assistant and tool events that can be tailed in real time.

## Expected implementation direction

A future implementation following this spec should:

1. launch Pi inside a PTY
2. inject Discord-originated work through PTY input
3. discover the newly created Pi JSONL session/archive file after launch
4. tail that JSONL file in real time
5. derive assistant reply text from structured events
6. send final content back through the bridge
7. use PTY output only for liveness, debugging, or fallback behavior

## Important invariant

Do not change the illusion from the agent's perspective:
- Pi should still behave as though it is attached to a normal terminal
- the harness owns the relay semantics outside the agent prompt path

## Acceptance focus for future work

A future implementation should be considered aligned with this spec update when:
- prompts still enter through PTY
- Pi still behaves like a normal terminal app
- replies are sourced from JSONL logs instead of terminal scraping
- the harness can discover and register the correct active Pi log source deterministically
