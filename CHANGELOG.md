# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Comprehensive engineering specification ([SPEC.md](SPEC.md)) updated with all 20 production requirements.
- Architecture documentation, STRIDE threat model, and RAG design specifications.
- Architecture Decision Records (ADR-0001 through ADR-0010) establishing foundational technical choices.
- Standardized Ent ORM data access layer with versioned SQL migrations.
- Support for Bring Your Own Key (BYOK) cloud providers (OpenAI, Gemini, Anthropic) and local developer CLI subscription bridges (`opencode`, `claudecode`, `gemini cli`, `codex`).
- Decoupled zero-cost local embedding architecture (Ollama / in-process).
- GitHub Personal Access Token (PAT) fallback mode for rapid local experimentation alongside GitHub App.
- Docker Compose development environment supporting both Full-Container and Host-CLI Bridge profiles.
- Automated CI and CD GitHub Actions workflow configurations.
- Granular implementation task backlog tracking `CF-001` through `CF-036`.
