# ADR 0008: Pluggable BYOK Providers, Local Embeddings, and Host CLI Bridge

## Status
Accepted

## Date
2026-09-08

## Context
ContextForge users range from self-hosted developers who want 100% offline or free local execution, to developers with existing coding agent subscriptions (such as OpenCode Go, Claude Code, or Gemini CLI), to enterprise teams utilizing cloud AI endpoints (OpenAI, Anthropic, Google Gemini).

Key requirements:
1. **BYOK (Bring Your Own Key)**: Users can supply their own cloud API keys stored with AES-256-GCM encryption.
2. **Free Local Embeddings**: Users can run local embeddings via Ollama (e.g. `nomic-embed-text`) or in-process models without recurring cloud API fees.
3. **Host CLI Subscription Bridge**: Users with active CLI agent subscriptions (specifically OpenCode Go via `opencode`, Claude Code via `claude`, or Gemini CLI) should be able to leverage their existing subscriptions directly from the ContextForge app without paying for additional API tokens.
4. **Clean Abstraction**: Application services must not be coupled to specific provider SDKs.

## Decision
We implemented clean Go provider interfaces (`LLMProvider` and `EmbeddingProvider`) with three supported runtime drivers:

```
                      ┌────────────────────────────────────────┐
                      │        Core Service Application        │
                      └───────────────────┬────────────────────┘
                                          │
                  ┌───────────────────────┴───────────────────────┐
                  ▼                                               ▼
         [ LLMProvider Interface ]                    [ EmbeddingProvider Interface ]
                  │                                               │
      ┌───────────┼───────────┐                       ┌───────────┼───────────┐
      ▼           ▼           ▼                       ▼           ▼           ▼
[ Cloud BYOK ] [ Host CLI ] [ Ollama ]          [ Cloud BYOK ] [ Ollama ]   [ Local/In-Process ]
(OpenAI,       (opencode,   (Local LLM)         (OpenAI,       (nomic-embed)
 Anthropic,     claude,                          Gemini)
 Gemini)        gemini)
```

### 1. Provider Interfaces
- `LLMProvider`: Defines `GenerateCompletion(ctx, req)` and `StreamCompletion(ctx, req, chunkCallback)`.
- `EmbeddingProvider`: Defines `EmbedDocuments(ctx, texts)` and `EmbedQuery(ctx, text)`.

### 2. Host CLI Bridge (`cli_opencode`, `cli_claude`, `cli_gemini`)
- When configured to use a host CLI, the Go service invokes the binary using secure subprocess pipelines (`exec.CommandContext`), injecting formatted prompt payloads via stdin and parsing streamed or final responses from stdout.
- Subprocesses are guarded with strict timeouts, non-zero exit code monitoring, stderr capturing, and clean process group termination (`SIGKILL` fallback).
- This enables users with an **OpenCode Go subscription** to route RAG generation queries through their authenticated local `opencode` binary with zero per-token cloud costs.

### 3. Local Embedding Strategy
- Defaults to Ollama `nomic-embed-text` (768 dimensions) or OpenAI `text-embedding-3-small` (1536 dimensions).
- Projects specify their embedding dimension at creation time to configure the target `pgvector` column.

## Consequences

### Positive
- Maximum economic flexibility: users can run 100% free locally (Ollama + local CLI), use their existing paid agent subscriptions (OpenCode Go, Claude Code), or connect enterprise cloud keys.
- Decoupled business logic: adding a new provider requires only implementing the interface and registering it in the provider factory.
- Robust security: BYOK keys are encrypted at rest with AES-256-GCM and decrypted only in-memory when making client requests.

### Negative
- Local CLI execution is sensitive to host binary paths and local authentication states.
- Running host CLIs requires the Hybrid Local Development profile (Go API running on host machine) rather than inside an unprivileged Docker container.

## References
- SPEC.md: Section 27 (LLM Provider Interface), Section 28 (Local & BYOK Embeddings Strategy), Section 29 (Local CLI Coding Agent Bridge)
- [OpenCode Documentation](https://opencode.ai)
