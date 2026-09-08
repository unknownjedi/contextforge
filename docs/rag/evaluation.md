# ContextForge RAG Evaluation Suite

ContextForge includes an automated evaluation suite to assess retrieval precision, recall, and answer faithfulness without regressions.

## Metrics Tracked

1. **Recall@K**: Percentage of target chunks retrieved within the top K results.
2. **Mean Reciprocal Rank (MRR)**: Average reciprocal rank of the first relevant retrieved document.
3. **Citation Precision**: Proportion of generated citations that accurately link to the exact line range supporting the claim.
4. **Faithfulness**: Absence of ungrounded statements (hallucinations) in generated answers.

## Local Evaluation Command

```bash
cd apps/api && go test -v -tags=evaluation ./tests/evaluation/...
```
