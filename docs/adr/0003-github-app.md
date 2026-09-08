# ADR 0003: GitHub App with OAuth Web Flow & Token Encryption (plus PAT Fallback)

## Status
Accepted

## Date
2026-09-08

## Context
ContextForge requires programmatic access to GitHub repositories across public and private codebases for cloning, syncing, polling commit trees, and receiving webhook events. 

Authentication must balance:
1. High API rate limits and fine-grained repository permissions in production.
2. Zero-friction setup for local single-user developers who may not want to configure a GitHub App registration just to test the software.
3. Cryptographic protection for stored authentication credentials.

## Decision
We chose a dual-tier GitHub authentication model:
1. **Production Standard: GitHub App with OAuth 2.0 Web Flow**:
   - Short-lived user access tokens (8 hours) refreshed via refresh tokens.
   - Installation access tokens (1 hour) with precise repository-level access control.
   - Dedicated webhook secret signature verification (`X-Hub-Signature-256` via HMAC-SHA256).
   - Elevated GitHub API rate limits (up to 15,000 requests/hour).
2. **Local Experimentation: Personal Access Token (PAT) Fallback**:
   - Developers can supply `CF_GITHUB_PAT_FALLBACK` in `.env` to bypass GitHub App registration during local development.
3. **Mandatory Token Encryption**:
   - All stored tokens (OAuth access tokens, refresh tokens, PATs) are encrypted at rest using authenticated **AES-256-GCM** with a unique 12-byte cryptographic nonce per record. No plaintext credentials ever touch the database.

## Consequences

### Positive
- Enterprise-grade security posture with fine-grained scoping and token rotation.
- Frictionless local onboarding via the PAT fallback environment variable.
- Protection against database dump credential harvesting via AES-256-GCM authenticated encryption.
- Webhook-driven real-time repository updates via HMAC-SHA256 verified GitHub events.

### Negative
- Setting up a GitHub App for production requires registering an application in GitHub settings with webhook endpoints and private keys.
- Managing token refresh cycles adds state machine complexity in the authentication service.

## Alternatives Considered
- **Classic OAuth App Only**: Rejected because OAuth apps have broad account-wide permissions, no fine-grained repository selection, lower rate limits, and lack modern installation token isolation.
- **PAT Only**: Rejected because requiring every production user to generate, copy, and paste PATs offers poor UX and lacks automated token rotation.

## References
- SPEC.md: Section 12 (GitHub App Integration), Section 13 (Personal Access Token Fallback), Section 58 (Encryption at Rest)
- [GitHub Apps Documentation](https://docs.github.com/en/apps)
