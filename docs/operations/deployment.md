# Production Deployment Guide

This document outlines deployment architecture patterns, infrastructure requirements, security hardening, and operational steps for deploying ContextForge in production environments.

---

## 1. Production Architecture Overview

ContextForge comprises three application tiers and two stateful data backends:

```
                          [ Internet Traffic / Users ]
                                       │
                                       ▼ (HTTPS 443)
                         [ Ingress / Reverse Proxy ]
                       (Cloudflare, Nginx, or Traefik)
                         │                         │
                         │ /api/*                  │ /*
                         ▼                         ▼
            [ Go API Service Cluster ]      [ Next.js Frontend Cluster ]
            (Stateless, autoscaling)         (Stateless, SSR/Static)
                   │        │                      │
                   │        └──────────────────────┘
                   │
         ┌─────────┴─────────────────────┐
         ▼                               ▼
 [ Redis Broker & Cache ]    [ Managed PostgreSQL 16 + pgvector ]
 (Asynq queues, rate limits)  (Relational schema, vectors, HNSW index)
         ▲
         │
 [ Asynq Worker Cluster ]
 (Ingestion, chunking, embeddings)
```

---

## 2. Infrastructure Sizing & Specifications

### Minimal Production Sizing (10-50 Repositories, < 500k chunks)
| Service | CPU | Memory | Storage | Recommended Managed Service |
| :--- | :--- | :--- | :--- | :--- |
| **PostgreSQL 16** | 2 vCPU | 8 GB RAM | 50 GB SSD (NVMe) | AWS RDS (db.t4g.large) / Supabase Pro / GCP Cloud SQL |
| **Redis 7** | 1 vCPU | 2 GB RAM | Persistent SSD | AWS ElastiCache / Redis Cloud / Upstash |
| **Go API Server** | 1 vCPU | 2 GB RAM | Ephemeral | AWS ECS / GCP Cloud Run / Fly.io / Kubernetes Pod |
| **Asynq Worker** | 2 vCPU | 4 GB RAM | Ephemeral | AWS ECS / GCP Cloud Run / Kubernetes Deployment |
| **Next.js Web** | 1 vCPU | 1 GB RAM | Ephemeral | Vercel / Cloudflare Pages / Containerized Pod |

### Standard Production Sizing (100-500 Repositories, 5M+ chunks)
| Service | CPU | Memory | Storage | Recommended Managed Service |
| :--- | :--- | :--- | :--- | :--- |
| **PostgreSQL 16** | 4-8 vCPU | 32-64 GB RAM | 250 GB Provisioned IOPS | AWS RDS (db.r6g.xlarge/2xlarge) with `shared_buffers = 16GB` |
| **Redis 7** | 2 vCPU | 8 GB RAM | Multi-AZ Failover | AWS ElastiCache / Managed Redis Cluster |
| **Go API (3 replicas)**| 2 vCPU ea | 4 GB RAM ea | Ephemeral | Kubernetes Deployment with HPA (CPU > 70%) |
| **Worker (4 replicas)**| 4 vCPU ea | 8 GB RAM ea | Ephemeral | Kubernetes Deployment with KEDA (queue depth scaling) |
| **Next.js Web** | 2 vCPU ea | 2 GB RAM ea | Ephemeral | Standalone Node.js Docker or Vercel Enterprise |

---

## 3. PostgreSQL & pgvector Tuning

For optimal HNSW vector search latency and high-throughput ingestion:

```ini
# postgresql.conf optimization guidelines
shared_buffers = 16GB             # 25% of total RAM
effective_cache_size = 48GB       # 75% of total RAM
maintenance_work_mem = 4GB        # Speeds up HNSW index building
work_mem = 64MB
max_worker_processes = 8
max_parallel_workers = 8
max_parallel_maintenance_workers = 4

# pgvector HNSW search query configuration
# Tune hnsw.ef_search per query or session:
# SET hnsw.ef_search = 100;
```

---

## 4. Secret Management & Cryptography

Never hardcode or bake secrets into Docker images. Use an external secret manager (AWS Secrets Manager, HashiCorp Vault, GCP Secret Manager, or Kubernetes Secrets).

| Variable Name | Description | Minimum Requirement |
| :--- | :--- | :--- |
| `CF_AUTH_JWT_SECRET` | Signing key for user sessions & JWTs | 64-byte random hex string (`openssl rand -hex 64`) |
| `CF_AUTH_TOKEN_ENCRYPTION_KEY` | Key for AES-256-GCM token encryption | 32-byte random hex string (`openssl rand -hex 32`) |
| `CF_DATABASE_URL` | PostgreSQL connection string with SSL | `sslmode=verify-full` or `sslmode=require` |
| `CF_REDIS_URL` | Redis connection URL with TLS | `rediss://user:password@host:port/0` |
| `CF_GITHUB_APP_PRIVATE_KEY` | PEM private key for GitHub App | Base64 encoded RSA private key (2048-bit+) |

---

## 5. Reverse Proxy & SSE Streaming Configuration

ContextForge uses Server-Sent Events (SSE) for real-time RAG query streaming (`/api/v1/chat/completions/stream`). Proxies must not buffer responses.

### Nginx Configuration Snippet
```nginx
server {
    listen 443 ssl http2;
    server_name api.contextforge.example.com;

    ssl_certificate /etc/letsencrypt/live/api.contextforge.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.contextforge.example.com/privkey.pem;

    # Standard API endpoints
    location / {
        proxy_pass http://api_upstream;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # SSE Streaming endpoint: Disable proxy buffering and keep connections alive
    location /api/v1/chat/ {
        proxy_pass http://api_upstream;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Crucial for SSE:
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
        proxy_http_version 1.1;
        proxy_set_header Connection '';
        chunked_transfer_encoding on;
    }
}
```

---

## 6. Zero-Downtime Deployment & Migrations

1. **Pre-Deployment Migrations**: Run additive database migrations (new tables, nullable columns) *before* rolling out new container versions:
   ```bash
   migrate -path migrations -database "${CF_DATABASE_URL}" up
   ```
2. **Rolling Update**: Deploy updated Go API and worker images with Kubernetes rolling update strategy (`maxSurge: 25%`, `maxUnavailable: 0`).
3. **Health Check Probes**:
   - Liveness Probe: `GET /healthz` (Status 200 if HTTP listener responds)
   - Readiness Probe: `GET /readyz` (Status 200 if PostgreSQL connection pool and Redis connections are live)
4. **Post-Deployment Cleanup**: Destructive schema changes (dropping old columns) are only executed in subsequent releases after old code versions are decommissioned.

---

## 7. Metrics & Observability

- Prometheus metrics are exposed at `GET /metrics`.
- Scrape intervals should be set to 15 seconds.
- Monitor critical metrics:
  - `contextforge_rag_query_latency_seconds`
  - `contextforge_hnsw_search_duration_seconds`
  - `contextforge_ingestion_queue_depth`
  - `contextforge_db_connection_pool_wait_count`
