# RFC: Prefer `any-llm-go` for Go-native multi-provider AI integration

## Status

Draft

## Intent

When we need a reusable backend AI layer in Go, we should prefer adopting
`any-llm-go` rather than importing a TypeScript-only stack or rebuilding a
provider abstraction from scratch.

This RFC is specifically about the lower AI/provider layer:

- model/provider abstraction
- streaming
- tool-calling support
- normalized backend integration

It is not a proposal to adopt a full upstream agent runtime.

## Context

We previously considered using Pi's standalone AI package:

- `@earendil-works/pi-ai`

That package is structurally good, but it shapes the implementation toward a
TypeScript backend.

If the surrounding system is Go-native, that creates an avoidable language split
in the core server/runtime layer. A mixed Go + TypeScript architecture may still
be justified in some cases, but it should not be the default when the main need
is simply "use multiple model providers through one backend interface."

`any-llm-go` appears to provide a closer fit for that requirement:

- Go-native
- unified provider abstraction
- streaming support
- multi-provider orientation

## Proposal

For Go-based systems that need multi-provider LLM support, we should standardize
on `any-llm-go` as the first candidate abstraction layer.

This means:

- keep the host system in Go
- use `any-llm-go` for provider/model access
- build our own product-specific agent/runtime behavior on top
- avoid importing larger agent frameworks unless we specifically need them

## Why this makes sense

### 1. It preserves language coherence

If the main application is already Go, a Go-native AI layer keeps:

- build tooling simpler
- deployment simpler
- operational ownership clearer
- contributor expectations more coherent

### 2. It reuses the expensive part

The most expensive layer to rebuild is the provider abstraction:

- OpenAI / Anthropic / Gemini / other provider differences
- model configuration
- streaming behavior
- tool-calling request/response normalization

That is exactly the layer we want to borrow.

### 3. It avoids premature framework capture

The goal is not to inherit someone else's full agent architecture.

The goal is to adopt a thin, reusable substrate and then implement our own:

- browser interaction model
- orchestration semantics
- session model
- tool model
- product behavior

### 4. It leaves the architecture open

If we later need:

- a custom agent runtime
- a browser-specific interaction model
- file- or workspace-aware orchestration
- application-specific tool semantics

we can build those ourselves without fighting a larger imported framework.

## Recommended usage pattern

The intended shape is:

1. Use `any-llm-go` only as the LLM/provider layer.
2. Keep session, orchestration, and product semantics local to our codebase.
3. Define our own internal abstraction around:
   - prompts/messages
   - tool definitions
   - tool execution
   - streamed events
   - retries / cancellation / observability
4. Treat `any-llm-go` as a boundary dependency, not the center of the system.

## Non-goals

This RFC does not propose:

- adopting a full external agent framework
- changing application architecture around an upstream runtime
- mirroring another product's session format
- coupling product semantics to upstream queue or transcript behavior

## Risks

### 1. Abstraction mismatch

`any-llm-go` may not normalize every provider capability the way we want.

That is acceptable if we keep our own internal boundary above it and treat it as
replaceable infrastructure rather than as a core domain model.

### 2. Upstream maturity and drift

As with any upstream dependency, we inherit its release cadence, provider support
choices, and API evolution.

We should isolate it behind our own small integration layer.

### 3. Hidden product assumptions

If we let the provider library define too much of our runtime shape, we risk
accidentally outsourcing product design to the dependency.

This RFC explicitly rejects that.

## Decision

Tentative recommendation: **yes**.

If we want a Go-native equivalent of a reusable multi-provider AI layer, we
should start with `any-llm-go` and keep everything above that boundary our own.
