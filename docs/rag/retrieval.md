# ContextForge Retrieval Specification

## 1. Vector Search via pgvector

All vector similarity operations are executed inside PostgreSQL using `pgvector` with cosine distance (`<=>`):

```sql
SELECT 
    id,
    document_id,
    content,
    metadata,
    1 - (embedding <=> $1) AS similarity
FROM document_chunks
WHERE project_id = $2
  AND 1 - (embedding <=> $1) >= $3
ORDER BY embedding <=> $1 ASC
LIMIT $4;
```

Parameters:
- `$1`: Query embedding vector (`[]float32`)
- `$2`: Target `project_id` UUID (enforces hard project isolation)
- `$3`: Minimum similarity score threshold (default: `0.5`)
- `$4`: Maximum chunk limit `K` (default: `5`, configurable up to `20`)

## 2. Index Strategy

To maintain sub-500ms retrieval latencies across growing knowledge bases:
- **Composite Index**: `CREATE INDEX ON document_chunks (project_id);`
- **HNSW Vector Index**:
  ```sql
  CREATE INDEX idx_document_chunks_embedding_hnsw 
  ON document_chunks 
  USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 64);
  ```

## 3. Grounding & Confidence Thresholds

- If retrieved chunks have similarity below `0.5`, the system emits a low-confidence notice:
  `"I cannot find sufficient evidence in the indexed project sources to answer this question accurately."`
- The model is strictly forbidden from fabricating facts or citations not grounded in the retrieved chunks.

## 4. Hybrid Retrieval & Reciprocal Rank Fusion (RRF)

To maximize precision across both natural language queries and exact identifiers (such as column names, foreign key references, or function signatures), ContextForge employs Reciprocal Rank Fusion ($k=60$):

$$\text{RRF Score}(d) = \sum_{m \in M} \frac{1}{k + r_m(d)}$$

where $M = \{\text{dense vector search}, \text{lexical keyword search}\}$ and $r_m(d)$ is the rank of document chunk $d$ in system $m$.

## 5. Line-Anchored Citation Format

Retrieved citations carry precise 1-indexed line anchors for both code files and normalized database DDL documents:
- **Code Citations**: `src/services/auth.ts#L42-L68`
- **Database Schema Citations**: `schema/public/orders.sql#L12-L28` (referencing exact column constraints, foreign keys, or indexes)
- **Database Sample Row Citations**: `data/public/products.sql#L1-L15` (referencing bounded table sample rows)

