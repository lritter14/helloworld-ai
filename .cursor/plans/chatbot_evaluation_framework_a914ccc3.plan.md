---
name: Chatbot Evaluation Framework
overview: Create a comprehensive evaluation framework to measure chatbot effectiveness over time, tracking improvements as models, hardware, and strategies change. Includes test suite management, multiple evaluation methods (LLM-as-judge, retrieval metrics, embedding quality), results storage, and automated reporting.
todos: []
---

# Chatbot Evaluation Framework

## Overview

Build a performance evaluation system that tracks chatbot quality metrics over time, enabling data-driven decisions about model changes, hardware upgrades, and embedding strategy improvements.

## Architecture

### 1. Test Suite Structure (`internal/evaluation/`)

**Test Case Format** (`testcase.go`):

- Question: The query to test
- ExpectedAnswer: Reference answer (optional, for LLM-as-judge)
- ExpectedReferences: Expected source chunks (for retrieval metrics)
- Category: Test category (e.g., "factual", "reasoning", "multi-hop")
- Difficulty: Difficulty level (easy, medium, hard)
- Vaults: Which vaults contain the answer
- Folders: Specific folders if applicable

**Test Suite** (`suite.go`):

- Collection of test cases
- Metadata (version, created date, description)
- Support for loading from JSON/YAML files

### 2. Evaluation Methods

#### A. LLM-as-Judge Evaluator (`evaluators/llm_judge.go`)

- Uses cloud LLM (OpenAI GPT-4 or Anthropic Claude) to evaluate answer quality
- Prompts judge model with question, expected answer (if available), and actual answer
- Returns scores: correctness (0-1), completeness (0-1), relevance (0-1), overall (0-1)
- Handles cases where expected answer is not provided (evaluates standalone quality)
- Cost tracking per evaluation

**Evaluation Prompt Template**:

```
Evaluate this answer to the question:

Question: {question}
Expected Answer (if available): {expected}
Actual Answer: {actual}
Context Used: {references}

Rate on:
1. Correctness: Is the answer factually correct?
2. Completeness: Does it fully answer the question?
3. Relevance: Is it relevant to the question?
4. Overall: Overall quality score

Return JSON: {"correctness": 0.0-1.0, "completeness": 0.0-1.0, "relevance": 0.0-1.0, "overall": 0.0-1.0, "reasoning": "..."}
```

#### B. Retrieval Metrics Evaluator (`evaluators/retrieval.go`)

- Precision@K: Fraction of retrieved chunks that are relevant
- Recall@K: Fraction of relevant chunks that were retrieved
- MRR (Mean Reciprocal Rank): Average of 1/rank of first relevant result
- Requires ground truth: expected chunk IDs or reference paths
- Fast, objective, no external dependencies

#### C. Embedding Quality Evaluator (`evaluators/embedding.go`)

- Semantic similarity: Compare query embedding to retrieved chunk embeddings
- Clustering quality: Measure how well related chunks cluster
- Cross-lingual consistency: For multilingual embeddings
- Embedding space analysis: Visualize embedding distributions

### 3. Results Storage (`storage/`)

**Results Database** (`results.go`):

- SQLite database for storing evaluation results
- Schema:
  - `evaluation_runs`: Run metadata (timestamp, config_hash, model, hardware, etc.)
  - `test_results`: Individual test case results
  - `metrics`: Aggregated metrics per run

**Configuration Tracking**:

- Hash of configuration (model names, embedding model, K values, thresholds)
- Hardware info (CPU, GPU, memory)
- Embedding strategy (model, chunking params)
- RAG parameters (vector/lexical weights, thresholds)

### 4. Test Runner (`runner.go`)

**CLI Tool** (`cmd/eval/main.go`):

- Run full test suite against current system
- Run specific test categories
- Compare against previous runs
- Generate reports

**Features**:

- Parallel test execution (with rate limiting for LLM-as-judge)
- Progress tracking
- Error handling and retry logic
- Configuration capture (auto-detect current system config)

### 5. Test Case Generation (`generator.go`)

**Automatic Test Case Generation**:

- Extract questions from notes (look for Q&A patterns, headings that are questions)
- Generate expected answers using cloud LLM (one-time generation)
- Extract expected references from note structure
- Validate generated test cases manually before adding to suite

**Manual Test Case Management**:

- JSO