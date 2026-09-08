# ContextForge RAG Architecture

ContextForge implements a project-isolated, explainable RAG pipeline designed specifically for source code repositories and technical documentation.

## Core Principles

1. **Source Independence**: Raw data from GitHub (code, PRs, issues), URLs, and uploaded files is converted into a standardized `Document` model before chunking.
2. **Deterministic Chunking**: Code and markdown are chunked respecting structural boundaries (functions, classes, markdown headings) rather than arbitrary byte boundaries.
3. **Structured Attribution**: The generative LLM is constrained to produce structured citations that directly map to specific line ranges and commit SHAs.

## Pipeline Sequence

```
User Query
    │
    ▼
Embedder (OpenAI / Gemini / Ollama) ──► Query Vector [dim]
    │
    ▼
VectorRepository.SearchSimilar ────────► PostgreSQL (pgvector cosine <=> query)
    │                                    [STRICT WHERE project_id = $project_id]
    ▼
Relevance Filtering & Re-ranking ──────► Top K Chunks (with file, commit, lines)
    │
    ▼
Prompt Construction ───────────────────► System Prompt + Reference Context
    │
    ▼
Generator (BYOK or opencode CLI) ──────► SSE Stream Output
    │
    ▼
Citation Extraction & Verification ────► Structured Citation Array (JSON)
```
