# ContextForge Security Controls & Invariants

This document outlines the concrete security controls and technical safeguards implemented in ContextForge.

## 1. Cryptography & Key Management

- **AES-256-GCM Token & Credential Encryption at Rest**:
  - All stored GitHub user OAuth tokens, refresh tokens, GitHub PATs, and external database connection strings (`encrypted_connection_url` in `database_sources`) are encrypted before insertion into PostgreSQL using AES-256-GCM.
  - Encryption key is supplied via the `CF_AUTH_TOKEN_ENCRYPTION_KEY` environment variable and never stored in the database.
  - A unique 12-byte random nonce is generated per encryption operation (`crypto/rand`) and prepended to the ciphertext.
  - Base64 encoding/decoding strictly uses `base64.StdEncoding.Strict()` to prevent ciphertext malleability.
- **Session Security**:
  - Raw session tokens are 256-bit random byte arrays formatted in hex (64 chars).
  - Database stores ONLY the SHA-256 hash of the session token. Compromise of the database does not yield valid session tokens.
  - Session cookies use `HttpOnly`, `SameSite=Lax`, and `Secure` (in HTTPS/production).

## 2. SSRF Protection Specifications

The URL ingestion connector and external database connector framework enforce strict IP and DNS validation before establishing connections:
- Normalizes IPv4-mapped IPv6 addresses (`ip.To4()`) to prevent bypasses like `::ffff:127.0.0.1`.
- Resolves DNS hostname to IP address via `net.LookupIP` and rejects unresolvable hosts or any host resolving to private ranges.
- Blocks connections if the target IP falls within:
  - `0.0.0.0/8` (Current network / unspecified)
  - `10.0.0.0/8` (Private network RFC 1918)
  - `100.64.0.0/10` (Shared address space / CGNAT)
  - `127.0.0.0/8` (Loopback)
  - `169.254.0.0/16` (Link-local / AWS & GCP Cloud Metadata `169.254.169.254`)
  - `172.16.0.0/12` (Private network RFC 1918)
  - `192.0.0.0/24` (IETF protocol assignments)
  - `192.0.2.0/24` (TEST-NET-1)
  - `192.168.0.0/16` (Private network RFC 1918)
  - `198.18.0.0/15` (Network benchmark tests)
  - `198.51.100.0/24` (TEST-NET-2)
  - `203.0.113.0/24` (TEST-NET-3)
  - `224.0.0.0/4` (Multicast)
  - `240.0.0.0/4` (Reserved)
  - `::1/128` (IPv6 Loopback)
  - `fc00::/7` (IPv6 Unique Local / ULA)
  - `fe80::/10` (IPv6 Link-local)
- Follow-redirects logic re-verifies the destination IP on every redirect.
- SQLite paths are strictly validated to block access to system and credential directories (`/root`, `/.ssh`, `/.env`, `/.git`, `/var/run/secrets`).
- Allows local/private IPs in local development only when `ALLOW_PRIVATE_IPS=true`.

## 3. Sensitive Data Redaction & Masking

- **Credential Error Redaction**: Connection errors pass through `SanitizeConnectionError` to scrub passwords, tokens, and credentials from standard URLs (`postgres://user:pass@host...`) and key-value formats (`Password=secret;...`) before logging or API serialization.
- **Sensitive Column Tokenization**: The database knowledge extractor uses word-boundary tokenization (`splitIntoWords`) to detect sensitive columns (`password`, `token`, `secret`, `api_key`, `salt`, `hash`, `ssn`, `credit_card`, `cvv`, `pwd`, `otp`, `jwt`, `bearer`, `passcode`) while eliminating false positives on benign columns (e.g. `author`, `author_id`, `authority`). Sensitive columns are completely excluded from row data sampling.

## 4. Inbound Rate Limiting

- **Implementation**: Redis-backed sliding window token bucket.
- **Rules**:
  - `/api/v1/auth/*`: 10 requests / min / IP.
  - `/api/v1/projects/{id}/chat`: 20 queries / min / user.
  - `/api/v1/projects/{id}/sources/{id}/sync`: 5 syncs / 10 min / project.
  - `/api/v1/projects/{id}/sources/database/*/sync`: 5 syncs / 10 min / project.
  - `/api/v1/projects/{id}/documents`: 20 uploads / min / project.
- **Headers**: Emits `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and returns HTTP 429 with `Retry-After: <seconds>`.

## 4. HTTP Security Headers

Every response includes:
- `Strict-Transport-Security: max-age=63072000; includeSubDomains; preload` (in production)
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Content-Security-Policy: default-src 'self'; script-src 'self'; object-src 'none';`
- `Referrer-Policy: strict-origin-when-cross-origin`
