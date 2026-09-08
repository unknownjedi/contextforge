# ContextForge Security Controls & Invariants

This document outlines the concrete security controls and technical safeguards implemented in ContextForge.

## 1. Cryptography & Key Management

- **AES-256-GCM Token Encryption at Rest**:
  - All stored GitHub user OAuth tokens, refresh tokens, and GitHub PATs are encrypted before insertion into PostgreSQL using AES-256-GCM.
  - Encryption key is supplied via the `ENCRYPTION_KEY_SECRET` environment variable and never stored in the database.
  - A unique 12-byte random nonce is generated per encryption operation (`crypto/rand`) and prepended to the ciphertext.
- **Session Security**:
  - Raw session tokens are 256-bit random byte arrays formatted in hex (64 chars).
  - Database stores ONLY the SHA-256 hash of the session token. Compromise of the database does not yield valid session tokens.
  - Session cookies use `HttpOnly`, `SameSite=Lax`, and `Secure` (in HTTPS/production).

## 2. SSRF Protection Specifications

The URL ingestion connector enforces strict IP and DNS validation before making HTTP requests:
- Resolves DNS hostname to IP address.
- Blocks connection if resolved IP falls within:
  - `0.0.0.0/8` (Current network)
  - `10.0.0.0/8` (Private network RFC 1918)
  - `100.64.0.0/10` (Shared address space)
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
  - `fc00::/7` (IPv6 Unique Local)
  - `fe80::/10` (IPv6 Link-local)
- Follow-redirects logic re-verifies the destination IP on every redirect.

## 3. Inbound Rate Limiting

- **Implementation**: Redis-backed sliding window token bucket.
- **Rules**:
  - `/api/v1/auth/*`: 10 requests / min / IP.
  - `/api/v1/projects/{id}/chat`: 20 queries / min / user.
  - `/api/v1/projects/{id}/sources/{id}/sync`: 5 syncs / 10 min / project.
  - `/api/v1/projects/{id}/documents`: 20 uploads / min / project.
- **Headers**: Emits `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and returns HTTP 429 with `Retry-After: <seconds>`.

## 4. HTTP Security Headers

Every response includes:
- `Strict-Transport-Security: max-age=63072000; includeSubDomains; preload` (in production)
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Content-Security-Policy: default-src 'self'; script-src 'self'; object-src 'none';`
- `Referrer-Policy: strict-origin-when-cross-origin`
