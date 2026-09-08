# Security Policy

ContextForge takes the security of our platform and user data seriously. We welcome responsible security vulnerability disclosures.

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

If you discover a potential security vulnerability in ContextForge, please do NOT disclose it publicly via GitHub issues or discussions.

Instead, please send a report to:
`security@contextforge.dev` (or open a private GitHub Security Advisory).

Please include:
- A detailed description of the vulnerability.
- Proof of concept or reproduction steps.
- Potential impact and affected components.

## Security Invariants

ContextForge guarantees the following security boundaries:
1. **Hard Project Isolation**: Every retrieval query enforces `WHERE project_id = $project_id` during vector index scan. Accidental cross-tenant leakage is strictly blocked.
2. **Encryption at Rest**: Stored GitHub OAuth tokens, refresh tokens, PATs, and credentials are encrypted using AES-256-GCM.
3. **Zero Plaintext Sessions**: Sessions are stored exclusively as SHA-256 hashes in the database.
4. **SSRF Protection**: URL crawling blocks private RFC1918 subnets, loopback interfaces, and cloud metadata IPs (`169.254.169.254`).
5. **Secret Redaction**: Middleware automatically redacts Authorization headers, session cookies, and API keys from logs.
