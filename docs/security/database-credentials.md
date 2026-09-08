# Database Credential Security & Threat Model

This document outlines the security architecture, threat model, and cryptographic controls implemented in ContextForge for handling external database connection strings and credentials.

---

## 1. Threat Model & Risk Assessment

| Threat Vector | Potential Impact | Mitigating Controls |
|---|---|---|
| **Database Compromise (Storage at Rest)** | Leakage of third-party database passwords from database dumps or backups. | **AES-256-GCM** encryption with dynamic 96-bit nonces. Zero plaintext stored. |
| **Server-Side Request Forgery (SSRF)** | Attacker connects to cloud metadata endpoints (`169.254.169.254`) or internal VPC services. | **Network Target Validation**: Private IP ranges and loopback blocked by default; DNS resolution verified prior to dialing. |
| **Credential Leakage in Logs / APM** | Passwords logged in error stack traces or query failure metrics. | **Error Redaction Filter**: Regex-based credential scrubber removes `user:password@` patterns from all errors. |
| **Cross-Tenant Credential Theft** | Tenant A accesses or decrypts Tenant B's database connection. | **Project-Scoped Authorization**: All queries enforce `WHERE project_id = $1` at database and repository levels. |
| **Data Exfiltration of Sensitive Customer Data** | PII, password hashes, or financial records embedded in vector embeddings. | **Sensitive Column Exclusion**: Automatic pattern matching drops columns with secrets, salts, card numbers, and SSNs. |
| **Unauthorized Write / Mutation** | Malicious or buggy queries altering external database state. | **Read-Only Catalog Queries**: Connectors only execute catalog discovery queries and bounded `SELECT` statements. |

---

## 2. Cryptographic Architecture

### Algorithm
- **Cipher**: Advanced Encryption Standard in Galois/Counter Mode (**AES-256-GCM**).
- **Key Size**: 256 bits (32 bytes), configured via the `ENCRYPTION_KEY` environment variable.
- **Nonce / IV**: 96-bit (12-byte) cryptographically secure random nonce generated via Go's `crypto/rand` for each encryption operation.
- **Authentication Tag**: 128-bit (16-byte) GCM tag appended to ciphertext to ensure message authenticity and prevent tampering.

### Storage Format
Ciphertext is encoded as a base64 string:
```
Base64( Nonce [12 bytes] || Ciphertext [...] || AuthTag [16 bytes] )
```

### Key Management
- The master encryption key must be supplied as a 32-byte string or hex-encoded string via the environment variable `ENCRYPTION_KEY`.
- In production, inject this variable using a secrets manager such as HashiCorp Vault, AWS Secrets Manager, or Kubernetes Secrets.
- Encryption keys are never written to disk, database, or logs.

---

## 3. Network & SSRF Mitigations

Before attempting any connection, the target address is evaluated by the SSRF validator:

1. **URL Parsing**: Parses host and port from standard database connection schemes (`postgres://`, `mysql://`, `sqlserver://`, etc.).
2. **DNS Resolution**: Resolves hostnames to IP addresses using Go's `net.LookupIP`.
3. **CIDR Evaluation**: Validates resolved IPs against prohibited subnets:
   - `10.0.0.0/8` (RFC 1918 Private)
   - `172.16.0.0/12` (RFC 1918 Private)
   - `192.168.0.0/16` (RFC 1918 Private)
   - `127.0.0.0/8` (RFC 1122 Loopback)
   - `169.254.0.0/16` (RFC 3927 Link-Local / AWS/GCP Metadata Service)
   - `::1` (IPv6 Loopback)
   - `fc00::/7` (IPv6 Unique Local Address)
   - `fe80::/10` (IPv6 Link-Local)
4. **Bypass Flag**: `ALLOW_PRIVATE_IPS=true` can be enabled strictly in development environments to allow connecting to Docker Compose containers or `localhost`.

---

## 4. Sensitive Data Filtering

During table extraction in `schema_and_data` mode, column names are checked against sensitive regular expressions:

```go
var SensitiveColumnPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)(password|passwd|pwd)`),
    regexp.MustCompile(`(?i)(secret|token|apikey|api_key|access_token|refresh_token)`),
    regexp.MustCompile(`(?i)(private_key|privkey|certificate|cert)`),
    regexp.MustCompile(`(?i)(auth|salt|hash|signature)`),
    regexp.MustCompile(`(?i)(ssn|social_security|national_id)`),
    regexp.MustCompile(`(?i)(credit_card|card_num|cc_num|cvv|cvc)`),
    regexp.MustCompile(`(?i)(pin|passcode|otp)`),
}
```

If a column matches any sensitive pattern:
- It is flagged as `is_sensitive: true` in metadata.
- It is automatically excluded from `SELECT` projection during sample row batching.
- It is excluded from generated row knowledge documents.
