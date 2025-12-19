# Evaluation Run Summary

**Run ID**: `20251219_223251`
**Timestamp**: 2025-12-19 22:35:54 UTC

## Run Description

temperature to 0.3, rebalance system prompt

## Configuration

### RAG Parameters

- **K**: 5
- **Rerank Weights**: Vector=0.7, Lexical=0.3
- **Folder Mode**: on_with_fallback

### Models

- **LLM Model**: Qwen2.5-3B-Instruct-Q4_K_M
- **Embedding Model**: ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0
- **Judge Model**: Qwen2.5-3B-Instruct-Q4_K_M
- **Judge Prompt Version**: v1.0
- **Judge Temperature**: 0.0

### Dataset

- **Eval Set**: eval/eval_set.jsonl
- **Eval Set Commit Hash**: `2e3265d7dad5f20c9835bed9c94b06576df0e2d6`

## Metrics Overview

The evaluation framework tracks the following metrics:

### Retrieval Metrics

These metrics measure whether the system successfully found the relevant content:

- **Recall@K**: Did we retrieve at least one chunk that matches the gold supports? (Binary: 0 or 1)
- **MRR (Mean Reciprocal Rank)**: How high was the first correct chunk ranked? (0-1, where 1.0 = first rank)
- **Precision@K**: Fraction of top K chunks that match any gold_support anchor (0-1)
- **Scope Miss Rate**: Fraction of cases where folder selection excluded all gold supports (0-1)
- **Attribution Hit Rate**: Did the final cited references include at least one matching gold_support? (Binary: 0 or 1)

### Answer Quality Metrics

These metrics measure whether the generated answer is correct and well-supported:

- **Groundedness (0-5)**: Are all claims in the answer supported by the provided context?
  - Score of 5 requires citations for all major claims
- **Correctness (0-5)**: Does the answer correctly address the question?

### Abstention Metrics

These metrics measure whether the system knows when not to answer:

- **Abstention Accuracy**: When answerable=false, did the model refuse? (Binary: 0 or 1)
- **Hallucination Rate on Unanswerable**: When answerable=false, did it confidently answer anyway? (Binary: 0 or 1)

## Results Summary


### Operational Metrics

- **Error Rate**: 0.00%
- **Timeout Rate**: 0.00%
- **Empty Response Rate**: 0.00%

### Performance

- **Latency p50**: 6653ms
- **Latency p95**: 10705ms

## Comparison to Previous Run

**Previous Run ID**: `20251219_050759`

### Configuration Changes


### Metric Changes


## Files

Detailed results are available in:

- `results.jsonl`: Individual test results with full detail
- `metrics.json`: Aggregated metrics in JSON format
- `config.json`: Run configuration snapshot
