# Pi Reverse-Engineered Spec

This directory reverse engineers the current Pi codebase into specification documents.

It is derived from the implementation, not from a greenfield design.

Documents:

- `LOW-LEVEL-SPEC.md`
  - code-close model of the current runtime and package boundaries
  - optimized for implementation fidelity over presentation
- `SPEC.md`
  - higher-level conceptual model
  - shaped after the style used in `/home/easter/element-sketchpad/research/SPEC.md`

Primary source files inspected for this pass:

- `packages/coding-agent/src/main.ts`
- `packages/coding-agent/src/core/agent-session.ts`
- `packages/coding-agent/src/core/agent-session-runtime.ts`
- `packages/coding-agent/src/core/agent-session-services.ts`
- `packages/coding-agent/src/core/session-manager.ts`
- `packages/coding-agent/src/core/sdk.ts`
- `packages/agent/src/agent-loop.ts`
- `packages/agent/src/harness/agent-harness.ts`
- `packages/ai/src/stream.ts`
- `packages/ai/src/api-registry.ts`
- `packages/tui/src/tui.ts`
