---
name: Chatbot Evaluation Framework
overview: Create a comprehensive Python-based evaluation framework to measure chatbot effectiveness over time, tracking improvements as models, hardware, and strategies change. Includes test suite management (JSONL format), core metrics (Retrieval Recall@K, MRR, Scope Miss Rate, Groundedness, Correctness, Abstention), anchor-based labeling workflow, results storage with latency/cost tracking, and automated reporting. Requires Go API changes for stable chunk IDs (32-char) and debug mode. Supports answerable/unanswerable questions and test categories (factual, multi_hop, recency/conflict).
todos:
  - id: stable_chunk_ids
    content: Add stable deterministic chunk IDs to Go API (32-char hash). Use byte offsets or text hash + rolling window instead of chunk_index for stability. Include chunk anchor (rel_path + heading_path + line numbers) in debug response.
    status: completed
  - id: debug_api_mode
    content: Add debug=true flag to /api/v1/ask endpoint returning top K chunks with rel_path, heading_path, scores, and metadata. Add abstained/abstain_reason fields to response.
    status: completed
  - id: eval_metrics_doc
    content: Create EVAL.md documenting core metrics (Recall@K, MRR, Scope Miss Rate, Groundedness, Correctness, Abstention metrics)
    status: completed
  - id: eval_set_creation
    content: Create initial frozen eval_set.jsonl with 20-100 questions (include answerable/unanswerable, multi_hop, recency/conflict categories). Freeze dataset version for comparability.
    status: completed
  - id: results_storage
    content: Design results storage format (JSONL per run + metrics JSON) with latency breakdown, cost tracking, judge input storage. Default to truncated chunk text (200 chars), full text only with --store-full-text flag.
    status: completed
  - id: labeling_workflow
    content: "Build labeling workflow script (label_eval.py) for marking gold_supports (anchor-based: rel_path + heading_path)"
    status: completed
  - id: python_runner
    content: Write Python eval runner (run_eval.py) with folder_mode options, latency tracking, cost tracking, retrieval-only mode (--retrieval-only flag), operational metrics (error rate, coverage by doc type), and indexing coverage stats capture (docs processed, chunks skipped, token distribution)
    status: pending
  - id: retrieval_metrics
    content: Implement retrieval metrics calculator (score_retrieval.py) for Recall@K (any + all for multi-hop), MRR, Precision@K, Scope Miss Rate, and Attribution Hit Rate (anchor-based matching with prefix match rules, multi-hop support groups, recency/conflict rules)
    status: pending
  - id: answer_quality_judges
    content: Implement answer quality judges (judge_answers.py) - separate groundedness (with citation coverage requirement) and correctness judges with fixed model (immutable version), temperature=0, structured JSON output. Add optional judge reliability spot-check (re-judge random subset). Add judge caching (keyed by question+answer+context_hash+judge+prompt).
    status: pending
  - id: abstention_metrics
    content: Implement abstention metrics calculator (score_abstention.py) for answerable=false questions
    status: pending
  - id: git_strategy
    content: Set up gitignore for results.jsonl (commit only metrics.json) or implement text redaction
    status: pending
  - id: run_comparison
    content: Create run comparison tool (compare_runs.py) with eval configuration invariants checking
    status: pending
  - id: regression_gate
    content: Implement regression gate (fail if Recall@K/scope_miss_rate/groundedness drop below thresholds, configurable)
    status: pending
---

# Chatbot Evaluation Framework

## Overview

Build a Python-based performance evaluation system that tracks chatbot quality metrics over time, enabling data-driven decisions about model changes, hardware upgrades, and embedding strategy improvements. The framework runs as a separate Python harness that calls the Go API, keeping the evaluation logic separate from the core system.

## Core Metrics (EVAL.md)

Define and track these core metrics on every run:

**Retrieval Metrics**:

1. **Retrieval Recall@K**: Did we retrieve the supporting content? (Binary: any retrieved chunk matches gold_supports)
2. **MRR (Mean Reciprocal Rank)**: How high was the first correct chunk ranked? (1/rank of first matching chunk)
3. **Precision@K** (optional): Fraction of top K chunks that match any gold_support anchor (for multi-hop, allow multiple anchors)

   - Tells you if you dragged in junk that can hurt groundedness + cost/latency
   - Lightweight: "fraction of top K chunks that match any gold_support anchor"

4. **Scope Miss Rate**: Fraction of cases where folder selection excluded all gold supports (only when folder_mode=on)
5. **Attribution Hit Rate**: For answerable questions, did the final cited references include at least one matching gold_support? (Binary, only for answerable questions)

**Answer Quality Metrics**:

6. **Groundedness (0-5)**: Are all claims in the answer supported by the provided context? (LLM-as-judge)

   - Score of 5 requires citations for all major claims (citation coverage is part of groundedness)

7. **Correctness (0-5)**: Does the answer correctly address the question? (LLM-as-judge, considers context + question)

**Abstention Metrics** (for unanswerable questions):

8. **Abstention Accuracy**: When `answerable=false`, did the model refuse/say it can't find support? (Binary)
9. **Hallucination Rate on Unanswerable**: When `answerable=false`, did it confidently answer anyway? (Binary)

These metrics provide objective, repeatable measurements. Retrieval metrics (Recall@K, MRR) don't drift over time. LLM-judge metrics (Groundedness, Correctness) have controlled drift through fixed judge model, prompt version, and temperature=0, enabling meaningful comparisons across runs.

## Architecture

### 1. Prerequisites (Go API Changes)

#### A. Stable Chunk IDs

**Strongly Recommended**: Each chunk should have a deterministic, repeatable ID across re-indexes. Not strictly required for scoring if anchors (rel_path + heading_path + line numbers) are present in debug response, but valuable for debugging and run diffs.

**Implementation** (Go):

- Generate chunk ID as hash of: `vault_id + rel_path + heading_path + chunk_index + chunk_text_hash`
- Return these IDs in `/api/v1/ask` response references
- Ensure IDs remain stable when content doesn't change

**Rationale**: Foundation for labeling and scoring. Without stable IDs, you can't track which chunks are correct across runs.

#### B. Debug Retrieval Mode

**Requirement**: Add `debug=true` query parameter to `/api/v1/ask` endpoint.

**Response Enhancement** (when `debug=true`):

- Include top K retrieved chunks with:
  - `chunk_id` (stable ID)
  - `rel_path`
  - `heading_path`
  - `score_vector` (vector similarity score)
  - `score_lexical` (lexical/BM25 score if applicable)
  - `score_final` (combined score)
  - `text` (full or truncated chunk text)
- Include folder selection output (chosen folders + reasoning if available)

**Rationale**: Machine-readable retrieval details needed for metrics calculation, not just logs.

### 2. Test Suite Structure (Python)

**Test Case Format** (`eval_set.jsonl` - JSONL format, one test case per line):

```json
{
  "id": "test_001",
  "question": "What is the main topic of the project?",
  "answerable": true,
  "expected_key_facts": [
    "The project is about RAG systems",
    "Uses llama.cpp for local LLMs",
    "Indexes markdown notes from Obsidian vaults"
  ],
  "gold_supports": [
    {
      "rel_path": "projects/main.md",
      "heading_path": "# Overview",
      "snippets": ["RAG systems", "llama.cpp"]
    }
  ],
  "required_support_groups": null,  // For multi-hop: [[0, 1], [2]] - indices into gold_supports array, OR-of-groups, AND within group
  "recency_conflict_rule": null,  // For recency/conflict: "cite_newer" | "acknowledge_both" | "cite_both"
  "tags": ["work", "code"],
  "vaults": ["personal"],
  "folders": ["projects"],
  "category": "factual",
  "difficulty": "easy"
}
```

**Key Fields**:

- `id`: Unique test case identifier
- `question`: The query to test
- `answerable`: Boolean - does the corpus contain an answer? (false = should abstain)
- `expected_key_facts`: Bullet points of what the answer should contain (for reference, optional)
- `gold_supports`: Ground truth supporting content (anchor-based, resilient to chunking changes)
  - `rel_path`: Relative path to the note file
  - `heading_path`: Heading hierarchy path (e.g., "# Overview > ## Details")
  - `snippets`: Optional exact phrases/quotes that should appear (for validation)
- `tags`: Flexible tags for filtering (work/personal, code/health, etc.)
- `vaults`/`folders`: Scope for the question
- `category`: Test category (factual, reasoning, multi_hop, recency/conflict, etc.)
- `difficulty`: Difficulty level (easy, medium, hard)

**Rationale**:

- Anchor-based `gold_supports` (rel_path + heading_path) is resilient to chunking changes
- `answerable` field enables abstention metrics (critical for real RAG systems)
- JSONL format is simple, easy to version control, and allows incremental labeling

**Test Case Categories**:

- `factual`: Simple factual questions that should have direct answers in notes
- `reasoning`: Questions requiring reasoning or synthesis across content
- `multi_hop`: Questions requiring information from 2+ chunks/notes (tests retrieval of multiple pieces)
- `recency/conflict`: Questions where notes contradict or have temporal conflicts (tests handling of conflicts, expects "depends / latest says...")
- `general`: Questions that shouldn't rely on your notes (tests hallucination control, should abstain)
- `adversarial`: Ambiguity, edge cases, outdated notes (tests robustness)

**Rationale**: Different categories test different aspects of RAG behavior. Multi-hop and recency/conflict catch common RAG issues.

### 3. Evaluation Methods (Python)

#### A. Retrieval Metrics (`score_retrieval.py`)

**Metrics**:

- **Recall@K**: Binary - any retrieved chunk matches `gold_supports`? (1 if yes, 0 if no)
  - **Match Definition** (deterministic normalization):
    - Normalize `heading_path`: strip extra spaces, consistent delimiter (` > `), same depth rules
    - A retrieved chunk matches a gold support if:
      - Same `rel_path` (exact match), and
      - Retrieved `heading_path` **starts with** gold `heading_path` (prefix match) - handles cases where chunking depth changes
    - If `snippets` are provided in gold_supports, optionally require snippet hit (chunk text contains snippet)
  - This prevents accidental "misses" when chunking depth or heading formatting changes
  - **Multi-hop Recall** (for `category: "multi_hop"`):
    - `Recall_any@K`: At least one support hit (current definition)
    - `Recall_all@K`: Did we retrieve *all required* supports? (only for tests marked `multi_hop=true` with `required_support_groups`)
    - Prevents multi-hop questions from looking "green" when only half the needed evidence was retrieved
  - **Recency/Conflict Cases** (for `category: "recency/conflict"`):
    - Define what "correct" means in labeling: e.g., "must cite the newer note" or "must acknowledge conflict and cite both"
    - Store expected behavior in test case metadata
- **Precision@K** (optional): Fraction of top K chunks that match any gold_support anchor
  - For multi-hop, allow multiple anchors (count as match if matches any required support)
  - Lightweight metric: tells you if you dragged in junk
- **MRR (Mean Reciprocal Rank)**: 1/rank of first matching chunk (0 if no match found)
- **Scope Miss Rate**: Fraction of cases where folder selection excluded all gold supports
  - Only calculated when `folder_mode=on` or `folder_mode=on_with_fallback`
  - Miss if folder selection excludes all gold supports (strict definition)
- **Attribution Hit Rate**: For answerable questions, did the final cited references include at least one matching `gold_support`?
  - Checks if any reference in the answer's `references` field matches a gold_support (using same match criteria as Recall@K)
  - Catches cases where retrieval found the right content but generation ignored it or cited irrelevant chunks
  - Only computed for questions where `answerable=true`

**Implementation**:

- Fast, objective, no external dependencies
- Requires ground truth: `gold_supports` from labeling workflow (anchor-based, not chunk IDs)
- Computed from debug API response (retrieved chunks with rel_path, heading_path, and ranks)
- Folder selection info from debug response

**Rationale**:

- Anchor-based matching (rel_path + heading_path) is resilient to chunking changes
- Scope miss rate detects when folder selection silently kills recall
- These metrics measure retrieval quality independently of answer generation

#### B. Answer Quality Judges (`judge_answers.py`)

**Two Separate Scores**: Split faithfulness into groundedness and correctness to detect different failure modes.

**1. Groundedness Judge** (0-5):

- **Focus**: Are all claims in the answer supported by the provided context?
- **Input**: Answer text + retrieved context (top K chunk texts)
- **Output**: Score (0-5) + structured JSON with unsupported_claims and supported_claims lists

**2. Correctness Judge** (0-5):

- **Focus**: Does the answer correctly address the question? (considers context + question)
- **Input**: Question + answer text + retrieved context
- **Output**: Score (0-5) + reasoning

**Judge Configuration** (Critical for preventing drift):

- **Fixed Judge Model**: Pick a single fixed judge model per "season" (e.g., Qwen2.5-14B, or specific cloud model)
  - **Immutable Version**: Judge model must be pinned to an immutable version or local model build hash
  - Even with temp=0, **model updates** (cloud) can change behavior
  - Codify: "judge model is pinned to an immutable version or local model build hash"
- **Judge Temperature**: Always use `temperature=0` for deterministic scoring
- **Prompt Version**: Store exact judge prompt version in config.json
- **Store Judge Input**: Save full judge input payload (question, answer, context) in results for re-judging later

**Judge Options**:

1. **Cloud LLM** (OpenAI GPT-4, Anthropic Claude) - Higher quality, costs money
2. **Local LLM** (via existing llama.cpp chat endpoint) - Free, fully local

**Groundedness Prompt Template**:

```
Evaluate whether all claims in the answer are supported by the retrieved context.

Answer: {answer}
Retrieved Context:
{context_chunks}

IMPORTANT:
- Treat anything not present in context as unsupported, even if it's "common knowledge"
- Penalize "confident tone" on unsupported claims

Rate groundedness (0-5):
- 5: All claims directly supported by context AND all major claims have citations
- 4: Most claims supported with citations, minor unsupported details
- 3: Some claims supported, some unsupported, or missing citations
- 2: Major claims unsupported or missing citations
- 1: Answer contradicts context
- 0: Answer has no relation to context

Note: Score of 5 requires citations for all major claims (citation coverage is part of groundedness).

Return JSON: {
  "score": 0-5,
  "reasoning": "...",
  "unsupported_claims": ["claim 1", "claim 2"],
  "supported_claims": ["claim 3", "claim 4"]
}
```

**Correctness Prompt Template**:

```
Evaluate whether the answer correctly addresses the question.

Question: {question}
Answer: {answer}
Retrieved Context:
{context_chunks}

Rate correctness (0-5):
- 5: Answer is fully correct and complete
- 4: Answer is mostly correct with minor issues
- 3: Answer is partially correct
- 2: Answer has significant errors
- 1: Answer is mostly incorrect
- 0: Answer is completely wrong

Return JSON: {"score": 0-5, "reasoning": "..."}
```

**Judge Reliability Spot-Check** (optional but recommended):

Even with temperature=0, judges can be inconsistent or drift over time. Add a reliability check:

- Re-judge a small random subset (e.g., n=10-20 questions, ~5-10% of eval set) using:
  - **Option A**: Second judge model (different model, same prompt)
  - **Option B**: Same judge with slightly different prompt version
- Compute disagreement rate (score difference > 1 point or different binary classification)
- Report disagreement rate in metrics.json as `judge_reliability: {"disagreement_rate": 0.15, "spot_check_n": 20}`

**Rationale**: Early warning that judge is becoming a bottleneck or drifting. High disagreement (>20%) suggests judge instability.

**Rationale** (for main judges):

- Groundedness detects hallucination (claims not in context)
- Correctness detects wrong interpretation (even if grounded)
- Structured JSON output (unsupported_claims) makes debugging dramatically easier
- Fixed judge model prevents score drift across runs

#### C. Abstention Metrics (`score_abstention.py`)

**Purpose**: Measure whether the system knows when not to answer (critical for real RAG systems).

**Metrics** (only for questions where `answerable=false`):

- **Abstention Accuracy**: Did the model refuse/say it can't find support? (Binary: 1 if abstained, 0 if answered)
- **Hallucination Rate on Unanswerable**: Did it confidently answer anyway? (Binary: 1 if answered confidently, 0 if abstained)
  - Inverse of abstention accuracy
  - High rate indicates the system is hallucinating on unanswerable questions

**Implementation** (robust detection):

**Option 1 (Preferred)**: Add abstention as first-class field in Go API response:

- `abstained: bool` - explicit abstention flag
- `abstain_reason: "no_relevant_context" | "ambiguous_question" | "insufficient_information" | ...` (optional)

**Option 2 (Fallback)**: If Go API changes aren't feasible, use a tiny judge prompt for abstention classification:

- Cheap LLM call (can use same judge model with temperature=0)
- Consistent classification (better than regex pattern matching)
- Prompt: "Does this answer indicate the system cannot answer the question? Return JSON: {\"abstained\": true/false, \"confidence\": 0-1}"

**Rationale**: Pattern matching is noisy (models phrase refusals differently). First-class API field is most reliable; judge prompt is good fallback.

**Rationale**: Prevents "Recall@K is always 0" from being misinterpreted as "retrieval is broken" when the correct behavior is to abstain. Critical for real-world RAG systems.

#### D. Future: Extended Evaluators

**Embedding Quality** (future):

- Semantic similarity analysis
- Clustering quality
- Can be added if needed for embedding model optimization

**Claim-Level Check** (optional, high leverage):

- Ask model to output answer as bullet claims (or structured JSON)
- Judge each claim individually against context
- Aggregate to a score
- More granular than answer-level judging

### 4. Labeling Workflow (`label_eval.py`)

**Purpose**: Create ground truth for retrieval metrics by marking which content supports the answer (anchor-based, not chunk IDs).

**Workflow**:

1. For each question in `eval_set.jsonl`, run ask with `K=20` and `debug=true`
2. Display retrieved chunks with their text, `rel_path`, and `heading_path`
3. User selects which chunks actually contain the answer
4. Store as `gold_supports` with anchor-based format:

   - `rel_path`: Relative path to the note file
   - `heading_path`: Heading hierarchy path (e.g., "# Overview > ## Details")
   - `snippets`: Optional exact phrases/quotes (can be extracted from chunk text)

5. Mark `answerable=true/false` based on whether corpus contains answer

**Implementation Options**:

- **CLI**: Simple terminal-based selection (arrow keys, space to select)
- **Web UI** (future): More user-friendly for bulk labeling

**Rationale**:

- Anchor-based labeling (rel_path + heading_path) is resilient to chunking changes
- If chunk boundaries change, gold_supports still work (matches by location, not ID)
- Ground truth is essential for retrieval metrics

### 5. Results Storage

**Format**: Simple file-based storage (JSONL + JSON)

**Structure**:

```
results/
  <run_id>/
    results.jsonl          # One line per test case with full results
    metrics.json           # Aggregated metrics
    config.json            # Run configuration snapshot
```

**Results JSONL Format** (one line per test case):

```json
{
  "test_case_id": "test_001",
  "question": "...",
  "answer": "...",
  "references": [...],
  "retrieved_chunks": [
    {
      "chunk_id": "...",
      "rel_path": "...",
      "heading_path": "...",
      "rank": 1,
      "score_vector": 0.95,
      "score_lexical": 0.80,
      "score_final": 0.90,
      "text": "First 200 chars...",  // Truncated by default, full text only with --store-full-text
      "token_count": 245  // Token count for this chunk
    },
    ...
  ],
  "config": {
    "k": 5,
    "rerank_weights": {"vector": 0.7, "lexical": 0.3},
    "folder_mode": "on_with_fallback",
    "llm_model": "...",
    "embedding_model": "...",
    "judge_model": "qwen2.5-14b",
    "judge_prompt_version": "v1.0",
    "judge_temperature": 0,
    "dataset_version": "abc123...",
    "index_build_version": "chunker_v1.2+embedding_granite-278m",
    "retriever_version": "k5+rerank_70_30",
    "answerer_version": "prompt_v2.0+llm_llama3.2"
  },
  "indexing_coverage": {
    "docs_processed": 1500,
    "docs_with_0_chunks": 5,
    "chunks_attempted": 8500,
    "chunks_embedded": 8450,
    "chunks_skipped": 50,
    "chunks_skipped_reasons": {
      "context_limit_exceeded": 45,
      "too_small": 5
    },
    "chunk_token_stats": {
      "min": 10,
      "max": 512,
      "mean": 245,
      "p95": 480
    },
    "chunker_version": "v1.2",
    "index_version": "2024-01-15_abc123"
  },
  "latency": {
    "total_ms": 1234,
    "folder_selection_ms": 50,
    "retrieval_ms": 200,
    "generation_ms": 900,
    "judge_ms": 84
  },
  "retrieval_metrics": {
    "recall_at_k": 1.0,
    "mrr": 0.5,
    "scope_miss": false,
    "attribution_hit": true
  },
  "groundedness": {
    "score": 4.5,
    "reasoning": "...",
    "unsupported_claims": [],
    "supported_claims": ["claim 1", "claim 2"]
  },
  "correctness": {
    "score": 4.0,
    "reasoning": "..."
  },
  "abstention": {
    "abstained": false,
    "hallucinated": false
  },
  "judge_input": {
    "question": "...",
    "answer": "...",
    "context_chunk_ids": ["chunk_abc123", "chunk_def456"],
    "context_chunks_truncated": ["First 200 chars of chunk 1...", "First 200 chars of chunk 2..."]
  },
  "cost": {
    "judge_tokens": 500,
    "judge_cost_usd": 0.001
  }
}
```

**Metrics JSON Format**:

```json
{
  "run_id": "...",
  "timestamp": "...",
  "config_hash": "...",
  "eval_set_commit_hash": "abc123...",
  "aggregate_metrics": {
    "recall_at_k_avg": 0.85,
    "mrr_avg": 0.72,
    "scope_miss_rate": 0.05,
    "attribution_hit_rate": 0.88,
    "groundedness_avg": 4.2,
    "correctness_avg": 4.0,
    "abstention_accuracy": 0.90,
    "hallucination_rate_unanswerable": 0.10,
    "judge_reliability": {
      "disagreement_rate": 0.15,
      "spot_check_n": 20
    },
    "latency": {
      "p50_ms": 1200,
      "p95_ms": 2500,
      "total_ms": 60000
    },
    "cost": {
      "judge_total_usd": 0.05,
      "judge_total_tokens": 25000
    },
    "indexing_coverage": {
      "no_chunks_generated_rate": 0.003,
      "chunks_skipped_due_to_context_rate": 0.006,
      "docs_processed": 1500,
      "docs_with_0_chunks": 5,
      "chunks_attempted": 8500,
      "chunks_embedded": 8450,
      "chunks_skipped": 50,
      "chunk_token_stats": {
        "min": 10,
        "max": 512,
        "mean": 245,
        "p95": 480
      }
    },
    "operational_metrics": {
      "error_rate": 0.02,
      "timeout_rate": 0.01,
      "empty_response_rate": 0.005,
      "coverage_by_doc_type": {
        "markdown": {"processed": 1200, "with_0_chunks": 2, "chunks_skipped": 30},
        "pdf": {"processed": 200, "with_0_chunks": 2, "chunks_skipped": 15},
        "code": {"processed": 100, "with_0_chunks": 1, "chunks_skipped": 5}
      }
    }
  },
  "by_tag": {
    "work": {
      "recall_at_k_avg": 0.90,
      "groundedness_avg": 4.3,
      "correctness_avg": 4.1
    },
    "personal": {
      "recall_at_k_avg": 0.80,
      "groundedness_avg": 4.0,
      "correctness_avg": 3.9
    }
  },
  "by_category": {
    "factual": {...},
    "multi_hop": {...},
    "recency/conflict": {...}
  },
  "total_tests": 50,
  "answerable_tests": 45,
  "unanswerable_tests": 5
}
```

**Configuration Tracking**:

- **Explicit config capture**: K, rerank weights, folder selection on/off, model names, prompt version
- **Versioning Strategy** (treat every experiment as a tuple):
  - **Dataset version**: Frozen eval_set.jsonl commit hash
  - **Index build version**: chunker_version + embedding_model + chunking params (min/max size, overlap, etc.)
  - **Retriever version**: vector params, filters, rerankers, K value
  - **Answerer version**: prompt template version + LLM model
- **Config hash**: Hash of config for quick comparison
- **Hardware info** (optional): CPU, GPU, memory (via system calls)
- **RAG parameters**: Vector/lexical weights, thresholds, folder selection strategy
- **Indexing Coverage Stats** (make eval sensitive to indexing changes):
  - `chunk_token_count` per chunk (for token budget analysis)
  - `index_version` / `chunker_version` (track chunking strategy changes)
  - `embedding_model` (track embedding model changes)
  - Coverage stats per run:
    - Docs processed, docs with 0 chunks
    - Chunks attempted / embedded / skipped (and why - e.g., "skipped_due_to_context_limit")
    - Distribution of tokens per chunk (min/max/mean/p95)

**Rationale**: File-based storage is simple, version-controllable, and easy to inspect. Can migrate to database later if needed.

### 6. Test Runner (`run_eval.py`)

**Purpose**: Execute test suite against Go API and store results.

**Features**:

- Reads `eval_set.jsonl` (frozen dataset version)
- Calls `/api/v1/ask` for each question (with `debug=true`) - runs pipeline end-to-end exactly like UI would
- Captures full response (answer, references, retrieved chunks with doc path, score, rank)
- Captures indexing coverage stats (if available from API or index metadata):
  - Docs processed, docs with 0 chunks
  - Chunks attempted/embedded/skipped (and why)
  - Token count per chunk, distribution stats
- Records configuration snapshot (including eval_set commit hash, index/retriever/answerer versions)
- Tracks latency breakdown (folder selection, retrieval, generation, judge)
- Tracks cost (judge tokens, estimated cost)
- **Storage Strategy**: By default, stores `chunk_id` + truncated text (first 200 chars) to keep runs lightweight and reduce privacy risk
  - Use `--store-full-text` flag to store full chunk text (for detailed debugging)
- Writes results to `results/<run_id>/results.jsonl`
- Progress tracking and error handling
- **Regression Gate**: Optionally fail if key metrics drop below thresholds (configurable)
- **Judge Caching** (make eval runs cheaper/faster):
  - Cache judge calls keyed by `(question, answer, topK_context_hash, judge_model, prompt_version)`
  - If you rerun retrieval-only experiments, you don't want to repay judging cost every time
  - Cache stored in `cache/judge_cache.jsonl` or similar
- **Retrieval-Only Mode** (speed iteration on chunking/indexing/rerank):
  - `--retrieval-only` flag: run retrieval metrics only (fast, no judge cost)
  - Then optionally run judges later on selected runs: `judge_answers.py --run-id <id>`
  - Speeds iteration on chunking/indexing/rerank without paying judge cost each time

**Configuration**:

- Explicit config parameters (K, rerank weights, folder_mode, judge settings, etc.)
- Config stored with results for reproducibility
- Can override defaults via CLI flags

**Folder Selection Modes**:

- `folder_mode=off`: No folder selection (search all folders)
- `folder_mode=on`: Use folder selection (may exclude relevant content)
- `folder_mode=on_with_fallback`: Use folder selection, but broaden search if confidence low

**Execution**:

- Sequential execution (simple, debuggable)
- Can add parallel execution later if needed
- Rate limiting for LLM-as-judge calls (if using cloud judge)
- Separate timing for each phase (folder selection, retrieval, generation, judge)

**Usage**:

```bash
python scripts/run_eval.py \
  --eval-set eval_set.jsonl \
  --k 5 \
  --rerank-vector-weight 0.7 \
  --rerank-lexical-weight 0.3 \
  --folder-mode on_with_fallback \
  --judge-model qwen2.5-14b \
  --judge-temperature 0 \
  --output-dir results
  # Optional: --store-full-text (stores full chunk text, not just truncated)
```

### 7. Run Comparison (`compare_runs.py`)

**Purpose**: Compare two evaluation runs to identify improvements and regressions.

**Output**:

- Metric deltas (Recall@K, MRR, faithfulness avg)
- Top regressions (questions that flipped from success → fail)
- Top improvements (questions that flipped from fail → success)
- Configuration differences

**Format**: Terminal report (simple, fast to generate)

**Usage**:

```bash
python scripts/compare_runs.py \
  --run-id-1 <baseline_run_id> \
  --run-id-2 <new_run_id>
```

**Rationale**: Quick feedback on whether changes improved or degraded performance.

**Regression Gate** (fail fast on regressions):

Add a regression gate to prevent "fixing chunking" but quietly breaking retrieval:

- Fail the run if key metrics drop by more than threshold:
  - Recall@K drops by >X% (e.g., >5% absolute)
  - Scope miss rate rises by >X% (e.g., >10% absolute)
  - Groundedness average drops by >X points (e.g., >0.5)
- Configurable thresholds via CLI flags (e.g., `--regression-threshold-recall=0.05`)
- Exit with non-zero code if regression detected
- Can be disabled with `--allow-regressions` flag (for exploratory runs)

**Rationale**: Prevents silent regressions from being committed. Forces explicit acknowledgment of trade-offs.

### 8. Test Case Generation (Future)

**Automatic Generation** (future enhancement):

- Extract questions from notes (Q&A patterns, headings that are questions)
- Generate expected key facts using cloud LLM (one-time generation)
- Extract expected references from note structure
- Validate generated test cases manually before adding to suite

**Manual Management**:

- Edit `eval_set.jsonl` directly (simple text format)
- Add questions as they fail in real usage
- Keep eval set aligned with actual usage patterns

### 9. Reporting and Visualization (Future)

**Report Generation** (future enhancement):

- HTML reports with charts (using matplotlib or plotly)
- Comparison reports between runs
- Trend analysis over time
- Category breakdowns (by tags, difficulty)
- Configuration impact analysis

**Initial Approach**: Terminal reports are sufficient for MVP. Add HTML reports later if needed.

### 10. Configuration

**LLM-as-Judge Configuration** (Python):

- Judge model selection (OpenAI GPT-4, Claude, or local llama.cpp)
- **Fixed judge model per "season"** (prevents score drift)
- **Judge temperature = 0** (deterministic scoring)
- **Judge prompt version** (stored in config for reproducibility)
- API keys (from environment variables for cloud models)
- Rate limiting settings
- Cost tracking enabled/disabled

**Evaluation Settings** (Python):

- Which evaluators to run (retrieval, groundedness, correctness, abstention)
- API endpoint URL (default: http://localhost:8080)
- Timeout settings
- Retry logic configuration

**Run Configuration** (captured per run):

- K value
- Rerank weights (vector/lexical)
- Folder mode (off, on, on_with_fallback)
- Model names (LLM, embedding, judge)
- Prompt version
- Judge model + prompt version + temperature
- Any other RAG parameters

### 10.1. Eval Configuration Invariants

**Purpose**: To compare runs meaningfully, enforce invariants unless explicitly overridden.

**Invariants** (fail fast if these don't match):

- **Same eval set commit hash**: Ensures test cases haven't changed
- **Same judge model + judge prompt version**: Prevents judge drift
- **Same judge temperature**: Ensures deterministic scoring
- **Same debug payload fields**: Ensures retrieval metrics are comparable

**Implementation**:

- Store `eval_set_commit_hash` in config.json (git hash of eval_set.jsonl)
- Compare invariants when comparing runs
- Warn or fail if invariants differ (unless `--ignore-invariants` flag used)

**Rationale**: Prevents meaningless comparisons (e.g., comparing runs with different test cases or different judges).

### 11. Integration Points

**Go API Integration**:

- **HTTP API**: Python harness calls `/api/v1/ask` endpoint
- **Debug Mode**: Requires `debug=true` parameter support (Go change needed)
- **Stable IDs**: Strongly recommended chunk IDs in response (Go change needed, but not strictly required if anchors present)
- **No RAG Changes**: Evaluation harness is external, doesn't modify RAG code

**Storage Integration**:

- **File-based**: Results stored as JSONL/JSON files (no database needed initially)
- **Git Strategy**: 
  - Commit only `metrics.json` + small summary (aggregated metrics, no sensitive content)
  - Keep full `results.jsonl` locally or gitignored (contains chunk texts which may be sensitive)
  - Or: Redact text fields before committing (keep structure, remove content)
- **Future**: Can migrate to SQLite if file-based becomes unwieldy

**Rationale**: Full results can explode repo size and may include sensitive note content. Aggregated metrics are sufficient for tracking trends.

**Configuration Integration**:

- **Explicit Config**: Python script captures config explicitly (no auto-detection needed)
- **Config Hash**: Hash of config for quick comparison between runs
- **Hardware Info**: Optional, via Python system calls if needed

## Implementation Details

### File Structure

```
eval/
├── EVAL.md                    # Core metrics definition
├── eval_set.jsonl             # Test cases (JSONL format, frozen)
├── results/                   # Evaluation run results (gitignored or redacted)
│   └── <run_id>/
│       ├── results.jsonl      # Per-test results (full detail)
│       ├── metrics.json       # Aggregated metrics (committed to git)
│       └── config.json        # Run configuration
├── cache/                     # Judge cache (gitignored)
│   └── judge_cache.jsonl      # Cached judge calls keyed by (question, answer, context_hash, judge, prompt)
├── metrics/                   # Aggregated metrics (optional, for git)
└── scripts/
    ├── run_eval.py            # Main evaluation runner (with --retrieval-only mode)
    ├── label_eval.py          # Labeling workflow tool (anchor-based)
    ├── score_retrieval.py     # Retrieval metrics calculator (Recall@K any/all, MRR, Precision@K, Scope Miss)
    ├── judge_answers.py       # Answer quality judges (Groundedness + Correctness, with caching)
    ├── score_abstention.py    # Abstention metrics calculator
    └── compare_runs.py        # Run comparison tool (with invariants checking)
```

### Go API Changes Required

**1. Stable Chunk IDs** (in `internal/indexer/` or `internal/storage/`):

**Problem**: Using `chunk_index` in ID makes IDs unstable when headings are added/removed or splitting rules change.

**Solution Options** (choose one):

**Option A (Best)**: Use byte offsets in preprocessed text:

```go
func generateChunkID(vaultID int, relPath, headingPath string, startByte, endByte int, chunkText string) string {
    hash := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%d|%d|%s",
        vaultID, relPath, headingPath, startByte, endByte, chunkText)))
    return hex.EncodeToString(hash[:])[:32]
}
```

**Option B**: Use text hash + rolling window:

```go
func generateChunkID(vaultID int, relPath, headingPath string, chunkText string, windowHash string) string {
    hash := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%s",
        vaultID, relPath, headingPath, chunkText, windowHash)))
    return hex.EncodeToString(hash[:])[:32]
}
```

**Option C (Fallback)**: At minimum, include chunk "anchor" in debug response:

```go
type RetrievedChunk struct {
    ChunkID      string  `json:"chunk_id"`  // May change, but useful for debugging
    RelPath      string  `json:"rel_path"`
    HeadingPath  string  `json:"heading_path"`
    StartLine    int     `json:"start_line"`  // Anchor for eval
    EndLine      int     `json:"end_line"`    // Anchor for eval
    // ... other fields
}
```

**Rationale**:

- Chunk IDs are *nice* but not strictly required for scoring (we use anchor-based gold_supports)
- They're most valuable for debugging and run diffs
- Including anchor (rel_path + heading_path + line numbers) in debug is often enough for eval + labeling even if chunk_id changes
- 32 hex chars = 128 bits, minimizing collision risk over large corpus + time

**2. Debug Mode in Ask Handler** (in `internal/handlers/ask.go`):

Add `debug` query parameter support:

```go
-[643wq1	
type 	
 AskResponse struct {
    Answer      string          `json:"answer"`
    References  []Reference     `json:"references"`
    Abstained   bool            `json:"abstained,omitempty"` // Explicit abstention flag (preferred for eval)
    AbstainReason string        `json:"abstain_reason,omitempty"` // Optional: "no_relevant_context", "ambiguous_question", etc.
    Debug       *DebugInfo      `json:"debug,omitempty"` // Only if debug=true
}

type DebugInfo struct {
    RetrievedChunks []RetrievedChunk `json:"retrieved_chunks"`
    FolderSelection *FolderSelection  `json:"folder_selection,omitempty"`
}

type RetrievedChunk struct {
    ChunkID      string  `json:"chunk_id"`
    RelPath      string  `json:"rel_path"`
    HeadingPath  string  `json:"heading_path"`
    ScoreVector  float64 `json:"score_vector"`
    ScoreLexical float64 `json:"score_lexical,omitempty"`
    ScoreFinal   float64 `json:"score_final"`
    Text         string  `json:"text"` // Full or truncated
    Rank         int     `json:"rank"`
}
```

### Results Storage Format

**File-based storage** (no database schema needed initially):

Results stored as JSONL files, one line per test case. Metrics aggregated into JSON files. See "Results Storage" section above for exact formats.

### Future: Database Schema (Optional)

If file-based storage becomes unwieldy, migrate to SQLite with this schema:

````sql

CREATE TABLE evaluation_runs (

id TEXT PRIMARY KEY,

timestamp DATETIME NOT NULL,

config_hash TEXT NOT NULL,

llm_model TEXT,

embedding_model TEXT,

hardware_info TEXT,

rag_params TEXT,  -- JSON

total_tests INTEGER,

llm_judge_cost REAL

);

CREATE TABLE test_results (

id TEXT PRIMARY KEY,

run_id TEXT NOT NULL,

test_case_id TEXT NOT NULL,

question TEXT NOT NULL,

answer TEXT NOT NULL,

references TEXT,  -- JSON array

faithfulness_score REAL,

faithfulness_reasoning TEXT,

retrieval_metrics TEXT,  -- JSON (recall@k, mrr)

execution_time_ms INTEGER,

error TEXT,

FOREIGN KEY (run_id) REFERENCES evaluation_runs(id)

);

CREATE TABLE metrics (

id TEXT PRIMARY KEY,

run_id TEXT NOT NULL,

metric_name TEXT NOT NULL,

metric_value REAL NOT NULL,

category TEXT,

FOREIGN KEY (run_id) REFERENCES evaluation_runs(id)

);

### Test Case JSONL Format

**File**: `eval_set.jsonl` (one JSON object per line)

**Example**:

```json
{"id": "test_001", "question": "What is the main topic of the project?", "answerable": true, "expected_key_facts": ["RAG systems", "llama.cpp", "Obsidian vaults"], "gold_supports": [{"rel_path": "projects/main.md", "heading_path": "# Overview", "snippets": ["RAG systems"]}], "tags": ["work", "code"], "vaults": ["personal"], "folders": ["projects"], "category": "factual", "difficulty": "easy"}
{"id": "test_002", "question": "How do I configure the embedding model?", "answerable": true, "expected_key_facts": ["Set EMBEDDING_MODEL env var", "Model must support 512 tokens"], "gold_supports": [{"rel_path": "docs/config.md", "heading_path": "# Setup > ## Embeddings", "snippets": ["EMBEDDING_MODEL", "512 tokens"]}], "tags": ["work", "config"], "vaults": ["personal"], "folders": ["docs"], "category": "factual", "difficulty": "medium"}
{"id": "test_003", "question": "What did I decide about the API design last week?", "answerable": true, "expected_key_facts": [], "gold_supports": [{"rel_path": "notes/api-design.md", "heading_path": "# Decisions", "snippets": []}, {"rel_path": "notes/meeting-notes.md", "heading_path": "# 2024-01-15", "snippets": []}], "required_support_groups": [[0, 1]], "tags": ["work"], "vaults": ["personal"], "folders": ["notes"], "category": "multi_hop", "difficulty": "hard"}
{"id": "test_004", "question": "What is the capital of Mars?", "answerable": false, "expected_key_facts": [], "gold_supports": [], "tags": ["general"], "vaults": ["personal"], "folders": [], "category": "factual", "difficulty": "easy"}
{"id": "test_005", "question": "Which note is more recent about the deployment process?", "answerable": true, "expected_key_facts": [], "gold_supports": [{"rel_path": "docs/deployment-v2.md", "heading_path": "# Overview", "snippets": []}, {"rel_path": "docs/deployment-v1.md", "heading_path": "# Overview", "snippets": []}], "recency_conflict_rule": "cite_newer", "tags": ["work"], "vaults": ["personal"], "folders": ["docs"], "category": "recency/conflict", "difficulty": "medium"}
```

**Rationale**: JSONL is simple, version-controllable, and allows incremental updates. One test case per line makes it easy to add/remove cases. Anchor-based `gold_supports` is resilient to chunking changes.

### CLI Usage Examples

```bash
# Run full evaluation suite
python scripts/run_eval.py --eval-set eval_set.jsonl

# Run retrieval-only (fast, no judge cost) - speeds iteration on chunking/indexing
python scripts/run_eval.py \
  --eval-set eval_set.jsonl \
  --retrieval-only

# Run with specific configuration
python scripts/run_eval.py \
  --eval-set eval_set.jsonl \
  --k 10 \
  --rerank-vector-weight 0.7 \
  --rerank-lexical-weight 0.3

# Run judges later on selected run (uses cache if available)
python scripts/judge_answers.py --run-id <run_id> --judge-model local

# Label test cases (mark gold chunks)
python scripts/label_eval.py --eval-set eval_set.jsonl

# Score retrieval metrics
python scripts/score_retrieval.py --run-id <run_id>

# Judge answer faithfulness
python scripts/judge_answers.py --run-id <run_id> --judge-model local

# Compare two runs
python scripts/compare_runs.py \
  --run-id-1 <baseline> \
  --run-id-2 <new_run>
```

## Evaluation Workflow

1. **Setup**: 

   - Create `EVAL.md` documenting the core metrics and definitions
   - Create initial `eval_set.jsonl` with 30-50 questions
   - Label test cases (mark gold chunk IDs using `label_eval.py`)

2. **Test Execution**: 

   - Run evaluation suite (`run_eval.py`)
   - Captures results with full configuration snapshot

3. **Scoring**:

   - Calculate retrieval metrics (`score_retrieval.py`)
   - Judge answer faithfulness (`judge_answers.py`)

4. **Analysis**: 

   - Compare runs (`compare_runs.py`)
   - Identify regressions and improvements

5. **Iteration**: 

   - Make controlled changes (one thing at a time)
   - Re-run evaluation
   - Track improvements over time

## Controlled Experiments Approach

**Key Principle**: Only change **one thing** per run to isolate impact.

**Versioning Strategy** (so results are comparable):

When you change indexing, keep retriever+answerer constant for that run. Then flip. Treat every experiment as a tuple:

- **Dataset version** (frozen eval_set.jsonl)
- **Index build version** (chunker/normalizer + embedding model + params)
- **Retriever version** (vector params, filters, rerankers)
- **Answerer version** (prompt/model)

**Things to Test**:

- Chunk sizes / heading rules
- K value / dynamic K
- Rerank weights (vector/lexical balance)
- Folder selection strategy + fallback
- Embedding model
- Prompt templates (citation enforcement, structured answers)
- "Reasoning steps" (query rewrite, multi-query, etc.)

**Record Every Run Config**: Store full configuration with results for reproducibility.

## Suggested Workflow (Fast Iteration)

**Step 0**: Baseline run on current system → save `run.jsonl` + summary.

**Step 1**: Add instrumentation only (no behavior change) → confirm run-to-run stability.

**Step 2**: Fix the big warnings in a branch (token-budget chunking, fallback chunking) → rebuild index → rerun eval.

**Step 3**: Only then start tuning chunk size/overlap, markdown handling, tables, etc.

**Rationale**: Establish baseline first, then make controlled changes. Prevents chasing noise or breaking things while "fixing" them.

## Key Metrics Tracked

**Core Metrics** (tracked every run):

- **Retrieval Recall@K**: Did we retrieve the supporting content? (Binary: 0 or 1, with Recall_all@K for multi-hop)
- **MRR (Mean Reciprocal Rank)**: How high was the first correct chunk ranked? (0-1)
- **Precision@K** (optional): Fraction of top K chunks that match any gold_support anchor (tells you if you dragged in junk)
- **Scope Miss Rate**: Fraction of cases where folder selection excluded all gold supports (only when folder_mode=on)
- **Attribution Hit Rate**: Did the final cited references include at least one matching gold_support? (Binary, only for answerable questions)
- **Groundedness (0-5)**: Are all claims supported by provided context? (LLM-as-judge)
- **Correctness (0-5)**: Does the answer correctly address the question? (LLM-as-judge)
- **Abstention Accuracy**: When `answerable=false`, did the model refuse? (Binary, only for unanswerable questions)
- **Hallucination Rate on Unanswerable**: When `answerable=false`, did it confidently answer anyway? (Binary, only for unanswerable questions)

**Performance Metrics** (tracked from day 1):

- **Latency**: p50/p95 per test, total suite time
- **Latency Breakdown**: Separate timings for folder selection, retrieval, generation, judge
- **Cost**: Judge tokens and estimated cost per run (even if approximate)

**Indexing Coverage Metrics** (make eval sensitive to indexing changes):

- **No Chunks Generated Rate**: Fraction of docs that produced 0 chunks (should trend toward ~0)
- **Chunks Skipped Due to Context**: Fraction of chunks skipped because they exceeded context limit (should trend toward 0)
- **Coverage Stats**:
  - Docs processed, docs with 0 chunks
  - Chunks attempted / embedded / skipped (and why)
  - Distribution of tokens per chunk (min/max/mean/p95)
- **Index Version Info**: chunker_version, embedding_model, index_version

**Success Metrics for Specific Warnings**:

Track improvements on indexing issues:

- `no_chunks_generated_rate` trending toward ~0 (or explainable exceptions only)
- `chunks_skipped_due_to_context` trending toward 0
- Recall@K and MRR improving on queries that hit problematic docs (workout docs, note docs)
- Ideally groundedness improves (fewer "missing context" answers)

**Operational Metrics** (production reality):

- **Error Rate**: % of cases where API failed / timed out / returned empty
- **Timeout Rate**: % of cases that timed out
- **Empty Response Rate**: % of cases that returned empty answer
- **Coverage by Doc Type** (optional): markdown vs pdf vs code, since chunking failures often cluster by type
  - Docs processed, docs with 0 chunks, chunks skipped per doc type

**Breakdown Metrics**:

- **By Tags**: Metrics segmented by work/personal, category, difficulty
- **By Category**: Separate metrics for factual, multi_hop, recency/conflict, etc.
- **Answerable vs Unanswerable**: Separate metrics for answerable and unanswerable questions

## Expanding the Eval Set Over Time

**Key Principle**: Keep eval set aligned with actual usage.

**When to Add Questions**:

- System fails on a real user question → add to eval set
- New use case discovered → add representative questions
- Edge cases found → add to test robustness

**Labeling Workflow**:

- Run ask with `K=20` and `debug=true`
- Mark which chunks contain the answer
- Store as `gold_supports` (anchor-based: rel_path + heading_path) in JSONL

**Maintenance**:

- Review eval set periodically
- Remove outdated questions
- Ensure coverage of important scenarios

## Future: External Sanity Suite (Optional)

Once core harness works, add a second eval file:

- **General questions**: Questions that shouldn't rely on your notes (tests hallucination control)
- **Adversarial questions**: Ambiguity, conflicting notes, outdated notes (tests robustness)

This tests system behavior beyond your specific knowledge base.

## Benefits

- **Data-Driven Decisions**: Quantify impact of model/hardware changes with objective metrics
- **Regression Detection**: Catch quality degradation early before it affects users
- **Optimization Guidance**: Identify which components (retrieval, reranking, prompting) need improvement
- **Historical Tracking**: See improvement trends over time as you iterate
- **Configuration Comparison**: Compare different setups objectively (K values, weights, models)
- **Simple & Maintainable**: Python harness is separate from Go code, easy to modify and extend
- **Incremental**: Start with 30-50 questions, expand as needed
- **Abstention Handling**: Properly measures when system should not answer (critical for real RAG)
- **Resilient to Changes**: Anchor-based labeling survives chunking strategy changes

## Nice-to-Have Enhancements (Non-Breaking, Add Later)

These improvements can be added incrementally without breaking existing functionality:

### A. Claim-Level Check (Optional, High Leverage)

Instead of scoring "answer faithfulness" only at the answer level, add a lightweight **claim-level check**:

1. Ask model to output answer as bullet claims (or structured JSON claims)
2. Judge each claim individually against context (supported/unsupported)
3. Aggregate to a score

**Benefits**: More granular than answer-level judging, easier to debug specific failures.

**Implementation**: Add optional `--claim-level` flag to `judge_answers.py`.

### B. Enhanced Judge Prompt (Reduce False Positives)

Improve the groundedness prompt with additional instructions:

- Explicitly state: "Treat anything not present in context as unsupported even if it's 'common knowledge'"
- Add: "Penalize 'confident tone' on unsupported claims"
- Return structured JSON with `unsupported_claims` and `supported_claims` lists (already in plan)

**Status**: Partially implemented in MVP (structured JSON output). Can refine prompt wording later.

### C. Web UI for Labeling (Future)

Replace CLI labeling tool with a web UI for bulk labeling:

- More user-friendly for large eval sets
- Better visualization of chunks and context
- Batch operations

**Status**: CLI is sufficient for MVP. Web UI can be added later.

### D. HTML Reports with Charts (Future)

Upgrade from terminal reports to HTML reports with charts:

- Trend analysis over time
- Interactive charts (using matplotlib or plotly)
- Category breakdowns with visualizations

**Status**: Terminal reports are sufficient for MVP. HTML reports can be added when needed.

### E. Automatic Test Case Generation (Future)

Extract questions from notes automatically:

- Look for Q&A patterns in notes
- Extract headings that are questions
- Generate expected key facts using LLM (one-time generation)

**Status**: Manual test case creation is fine for MVP. Automatic generation can be added later.

### F. External Sanity Suite (Future)

Add a second eval file for general/adversarial questions:

- Questions that shouldn't rely on your notes (tests hallucination control)
- Adversarial questions (ambiguity, conflicting notes, outdated notes)

**Status**: Can be added as a separate eval_set_sanity.jsonl file later.