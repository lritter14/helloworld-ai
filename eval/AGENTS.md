# Evaluation Framework - Agent Guide

This document outlines patterns, guidelines, and best practices for the evaluation framework. The evaluation framework is a Python-based harness that measures chatbot effectiveness over time, enabling data-driven decisions about model changes, hardware upgrades, and embedding strategy improvements.

## Overview

The evaluation framework runs as a separate Python harness that calls the Go API, keeping the evaluation logic separate from the core system. It tracks core metrics (Retrieval Recall@K, MRR, Scope Miss Rate, Groundedness, Correctness, Abstention) and stores results in a structured format for comparison across runs.

## Core Principles

### 1. Anchor-Based Labeling

**Principle**: Use anchor-based labeling (rel_path + heading_path) instead of chunk IDs for ground truth.

**Rationale**: Anchor-based labeling is resilient to chunking changes. If chunk boundaries change, gold_supports still work because they match by location (rel_path + heading_path), not by chunk ID.

**Implementation**:

- Gold supports use `rel_path` (relative path to note file) and `heading_path` (heading hierarchy path)
- Matching uses prefix matching on heading_path to handle chunking depth changes
- Snippets (optional) can be used for additional validation

**Example**:

```json
{
  "gold_supports": [
    {
      "rel_path": "Software/LeetCode Tips.md",
      "heading_path": "# Golang Tips & Oddities",
      "snippets": ["no built in string sort", "single element in a string is a byte"]
    }
  ]
}
```

### 2. Metric Stability

**Principle**: Retrieval metrics don't drift over time. LLM-judge metrics have controlled drift through fixed configuration.

**Retrieval Metrics** (Stable):

- Use deterministic anchor-based matching (rel_path + heading_path)
- Don't depend on LLM judges
- Computed from debug API response (retrieved chunks with ranks)

**LLM-Judge Metrics** (Controlled Drift):

- Fixed judge model (immutable version or local model build hash)
- Temperature = 0 (deterministic scoring)
- Fixed prompt version (stored in config for reproducibility)
- Judge input storage (full judge input payload saved for re-judging later)

**Rationale**: Even with temperature=0, model updates (cloud) can change behavior. Pinning judge model version ensures meaningful comparisons across runs.

### 3. Configuration Tracking

**Principle**: Track all configuration that affects results for reproducibility.

**Versioning Strategy** (treat every experiment as a tuple):

- **Dataset version**: Frozen eval_set.jsonl commit hash
- **Index build version**: chunker_version + embedding_model + chunking params
- **Retriever version**: vector params, filters, rerankers, K value
- **Answerer version**: prompt template version + LLM model

**Implementation**:

- Store full configuration in `config.json` for each run
- Include `eval_set_commit_hash` for dataset version tracking
- Include model names, prompt versions, and all RAG parameters
- Generate `config_hash` for quick comparison between runs

### 4. Results Storage Strategy

**Principle**: Store results in a structured format that balances detail with privacy/size concerns.

**Default Behavior**:

- Chunk text truncated to 200 characters by default
- Full text only with `--store-full-text` flag
- Store full judge input payload for reproducibility
- Store latency breakdown and cost tracking

**Git Strategy**:

- Commit only `metrics.json` + small summary (aggregated metrics, no sensitive content)
- Keep full `results.jsonl` locally or gitignored (contains chunk texts which may be sensitive)
- Or: Redact text fields before committing (keep structure, remove content)

**File Structure**:

```
eval/results/
  <run_id>/
    results.jsonl      # One line per test case (full detail)
    metrics.json       # Aggregated metrics (committed to git)
    config.json        # Run configuration snapshot
```

## Script Patterns

### 1. Labeling Workflow (`label_eval.py`)

**Purpose**: Create ground truth for retrieval metrics by marking which content supports the answer.

**Workflow**:

1. Load test cases from `eval_set.jsonl`
2. For each test case:
   - Call API with `K=20` and `debug=true` to retrieve chunks
   - Display chunks with their text, rel_path, heading_path, and scores
   - User selects which chunks contain the answer (using numbers to toggle)
   - User marks question as answerable/unanswerable
3. Create `gold_supports` with anchor-based format
4. Save updated test cases back to `eval_set.jsonl` (with backup)

**Key Features**:

- Interactive chunk selection (arrow keys, space to select)
- Anchor-based labeling (rel_path + heading_path)
- Automatic snippet extraction
- Backup creation before saving

**Usage**:

```bash
python eval/scripts/label_eval.py --eval-set eval/eval_set.jsonl --api-url http://localhost:9000
```

### 2. Results Storage (`storage.py`)

**Purpose**: Provide data structures and utilities for storing evaluation results.

**Key Data Structures**:

- `RetrievedChunk`: Represents a retrieved chunk with scores and metadata
- `RunConfig`: Configuration snapshot for a run
- `TestResult`: Complete test result for a single test case
- `ResultsWriter`: Writer for storing results to JSONL/JSON format

**Text Truncation**:

- Default truncation to 200 characters
- Use `store_full_text=True` to store full chunk text
- Truncation applied in `to_dict()` methods

**Usage**:

```python
from storage import ResultsWriter, TestResult, RunConfig

writer = ResultsWriter(output_dir="eval/results", run_id="run_20240115_001")
config = RunConfig(k=5, folder_mode="on_with_fallback", ...)
writer.write_config(config, eval_set_commit_hash="abc123")

result = TestResult(test_case_id="test_001", ...)
writer.write_result(result, store_full_text=False)
```

### 3. Evaluation Runner (`run_eval.py`) - To Be Implemented

**Purpose**: Execute test suite against Go API and store results.

**Key Features**:

- Reads `eval_set.jsonl` (frozen dataset version)
- Calls `/api/v1/ask` for each question (with `debug=true`)
- Captures full response (answer, references, retrieved chunks)
- Records configuration snapshot
- Tracks latency breakdown and cost
- **Retrieval-Only Mode**: `--retrieval-only` flag for fast iteration (no judge cost)
- **Judge Caching**: Cache judge calls keyed by `(question, answer, context_hash, judge, prompt)`

**Usage**:

```bash
# Full evaluation
python eval/scripts/run_eval.py --eval-set eval/eval_set.jsonl --k 5

# Retrieval-only (fast, no judge cost)
python eval/scripts/run_eval.py --eval-set eval/eval_set.jsonl --retrieval-only
```

### 4. Retrieval Metrics Calculator (`score_retrieval.py`) - To Be Implemented

**Purpose**: Calculate retrieval metrics from debug API response.

**Metrics**:

- **Recall@K**: Binary - any retrieved chunk matches gold_supports?
  - `Recall_any@K`: At least one support hit
  - `Recall_all@K`: All required supports retrieved (for multi-hop)
- **MRR**: 1/rank of first matching chunk
- **Precision@K**: Fraction of top K chunks that match any gold_support anchor
- **Scope Miss Rate**: Fraction of cases where folder selection excluded all gold supports
- **Attribution Hit Rate**: Did final cited references include at least one matching gold_support?

**Match Definition**:

- Normalize `heading_path`: strip extra spaces, consistent delimiter (` > `)
- Match if: same `rel_path` (exact) AND retrieved `heading_path` starts with gold `heading_path` (prefix match)
- Handles chunking depth changes gracefully

**Multi-hop Handling**:

- For `category: "multi_hop"` with `required_support_groups`, compute both `Recall_any@K` and `Recall_all@K`
- `Recall_all@K` prevents multi-hop questions from looking "green" when only half the needed evidence was retrieved

### 5. Answer Quality Judges (`judge_answers.py`) - To Be Implemented

**Purpose**: Judge answer quality using LLM-as-judge with controlled configuration.

**Two Separate Scores**:

1. **Groundedness (0-5)**: Are all claims in the answer supported by the provided context?
2. **Correctness (0-5)**: Does the answer correctly address the question?

**Judge Configuration** (Critical for preventing drift):

- **Fixed Judge Model**: Pick a single fixed judge model per "season" (e.g., Qwen2.5-14B)
- **Immutable Version**: Judge model must be pinned to an immutable version or local model build hash
- **Judge Temperature**: Always use `temperature=0` for deterministic scoring
- **Prompt Version**: Store exact judge prompt version in config.json

**Judge Caching**:

- Cache judge calls keyed by `(question, answer, topK_context_hash, judge_model, prompt_version)`
- Speeds up re-runs and reduces costs
- Cache stored in `cache/judge_cache.jsonl`

**Structured Output**:

- Groundedness judge returns structured JSON with `unsupported_claims` and `supported_claims` lists
- Makes debugging dramatically easier

### 6. Abstention Metrics Calculator (`score_abstention.py`) - To Be Implemented

**Purpose**: Measure whether the system knows when not to answer.

**Metrics** (only for questions where `answerable=false`):

- **Abstention Accuracy**: Did the model refuse/say it can't find support? (Binary: 1 if abstained, 0 if answered)
- **Hallucination Rate on Unanswerable**: Did it confidently answer anyway? (Binary: 1 if answered confidently, 0 if abstained)

**Detection**:

- **Option 1 (Preferred)**: Use explicit `abstained: bool` field in Go API response
- **Option 2 (Fallback)**: Use a tiny judge prompt for abstention classification

**Rationale**: Prevents "Recall@K is always 0" from being misinterpreted as "retrieval is broken" when the correct behavior is to abstain.

### 7. Run Comparison Tool (`compare_runs.py`) - To Be Implemented

**Purpose**: Compare two evaluation runs to identify improvements and regressions.

**Output**:

- Metric deltas (Recall@K, MRR, groundedness avg)
- Top regressions (questions that flipped from success → fail)
- Top improvements (questions that flipped from fail → success)
- Configuration differences

**Invariants Checking**:

- Fail fast if these don't match (unless `--ignore-invariants` flag):
  - Same eval set commit hash
  - Same judge model + judge prompt version
  - Same judge temperature
  - Same debug payload fields

**Rationale**: Prevents meaningless comparisons (e.g., comparing runs with different test cases or different judges).

## Test Case Format

**Test Case Structure** (`eval_set.jsonl` - JSONL format, one test case per line):

```json
{
  "id": "test_001",
  "question": "What are the key tips for LeetCode interviews in Golang?",
  "answerable": true,
  "expected_key_facts": ["no built in string sort", "single element in string is a byte", "custom sorting"],
  "gold_supports": [
    {
      "rel_path": "Software/LeetCode Tips.md",
      "heading_path": "# Golang Tips & Oddities",
      "snippets": ["no built in string sort", "single element in a string is a byte"]
    }
  ],
  "required_support_groups": null,
  "recency_conflict_rule": null,
  "tags": ["personal", "code"],
  "vaults": ["personal"],
  "folders": ["Software"],
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
- `required_support_groups`: For multi-hop: `[[0, 1], [2]]` - indices into gold_supports array, OR-of-groups, AND within group
- `recency_conflict_rule`: For recency/conflict: `"cite_newer" | "acknowledge_both" | "cite_both"`
- `tags`: Flexible tags for filtering (work/personal, code/health, etc.)
- `vaults`/`folders`: Scope for the question
- `category`: Test category (factual, reasoning, multi_hop, recency/conflict, etc.)
- `difficulty`: Difficulty level (easy, medium, hard)

**Test Case Categories**:

- `factual`: Simple factual questions that should have direct answers in notes
- `reasoning`: Questions requiring reasoning or synthesis across content
- `multi_hop`: Questions requiring information from 2+ chunks/notes (tests retrieval of multiple pieces)
- `recency/conflict`: Questions where notes contradict or have temporal conflicts (tests handling of conflicts)
- `general`: Questions that shouldn't rely on your notes (tests hallucination control, should abstain)
- `adversarial`: Ambiguity, edge cases, outdated notes (tests robustness)

## API Integration

### Debug Mode

**Requirement**: The Go API must support `debug=true` query parameter on `/api/v1/ask` endpoint.

**Response Enhancement** (when `debug=true`):

- Include top K retrieved chunks with:
  - `chunk_id` (stable ID)
  - `rel_path`
  - `heading_path`
  - `score_vector` (vector similarity score)
  - `score_lexical` (lexical/BM25 score if applicable)
  - `score_final` (combined score)
  - `text` (full or truncated chunk text)
  - `rank` (rank in retrieval results)
- Include folder selection output (chosen folders + reasoning if available)

**Status**: ✅ Implemented in `internal/handlers/ask.go`

### Stable Chunk IDs

**Requirement**: Each chunk should have a deterministic, repeatable ID across re-indexes.

**Implementation**: ✅ Implemented in `internal/indexer/pipeline.go`

- Generate chunk ID as hash of: `vault_id + rel_path + heading_path + chunk_text`
- Return these IDs in `/api/v1/ask` response references
- Ensure IDs remain stable when content doesn't change

**Rationale**: Foundation for labeling and scoring. Without stable IDs, you can't track which chunks are correct across runs.

### Abstention Fields

**Requirement**: Add explicit abstention fields to API response.

**Status**: ✅ Implemented in `internal/handlers/ask.go`

- `abstained: bool` - explicit abstention flag
- `abstain_reason: string` - optional reason (e.g., "no_relevant_context", "ambiguous_question")

**Rationale**: First-class API field is most reliable for abstention detection (better than pattern matching).

## Evaluation Workflow

### 1. Setup

- Create `EVAL.md` documenting the core metrics and definitions ✅
- Create initial `eval_set.jsonl` with 30-50 questions ✅
- Label test cases (mark gold supports using `label_eval.py`) ✅

### 2. Test Execution

- Run evaluation suite (`run_eval.py`) - 🔄 To Be Implemented
- Captures results with full configuration snapshot
- Tracks latency breakdown and cost

### 3. Scoring

- Calculate retrieval metrics (`score_retrieval.py`) - 🔄 To Be Implemented
- Judge answer quality (`judge_answers.py`) - 🔄 To Be Implemented
- Calculate abstention metrics (`score_abstention.py`) - 🔄 To Be Implemented

### 4. Analysis

- Compare runs (`compare_runs.py`) - 🔄 To Be Implemented
- Identify regressions and improvements

### 5. Iteration

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

## Best Practices

### 1. Freeze Eval Set

- Once labeled, freeze `eval_set.jsonl` for comparability
- Track eval set version via commit hash
- Create new eval set version only when adding significant new test cases

### 2. Judge Model Selection

- Pick a single fixed judge model per "season"
- Pin to immutable version or local model build hash
- Use `temperature=0` for deterministic scoring
- Store prompt version in config

### 3. Storage Strategy

- Default to truncated text (200 chars) to keep runs lightweight
- Use `--store-full-text` only when needed for detailed debugging
- Commit only `metrics.json` to git (not full `results.jsonl`)

### 4. Regression Detection

- Set thresholds for key metrics (Recall@K, scope_miss_rate, groundedness)
- Fail fast on regressions (configurable via CLI flags)
- Force explicit acknowledgment of trade-offs

### 5. Iteration Speed

- Use `--retrieval-only` mode for fast iteration on chunking/indexing/rerank
- Run judges later on selected runs: `judge_answers.py --run-id <id>`
- Speeds iteration without paying judge cost each time

## File Structure

```
eval/
├── EVAL.md                    # Core metrics definition ✅
├── AGENTS.md                  # This file ✅
├── eval_set.jsonl             # Test cases (JSONL format, frozen) ✅
├── results/                   # Evaluation run results (gitignored or redacted)
│   └── <run_id>/
│       ├── results.jsonl      # Per-test results (full detail)
│       ├── metrics.json       # Aggregated metrics (committed to git)
│       └── config.json        # Run configuration
├── cache/                     # Judge cache (gitignored)
│   └── judge_cache.jsonl      # Cached judge calls
└── scripts/
    ├── label_eval.py          # Labeling workflow tool ✅
    ├── storage.py             # Results storage module ✅
    ├── test_storage.py        # Storage tests ✅
    ├── README.md              # Scripts documentation ✅
    ├── SETUP.md               # Setup guide ✅
    ├── run_eval.py            # Main evaluation runner 🔄
    ├── score_retrieval.py     # Retrieval metrics calculator 🔄
    ├── judge_answers.py       # Answer quality judges 🔄
    ├── score_abstention.py    # Abstention metrics calculator 🔄
    └── compare_runs.py        # Run comparison tool 🔄
```

## Dependencies

**Python Requirements**:

- `requests` - For API calls
- `dataclasses` - For structured data (Python 3.7+)
- `pathlib` - For file operations (Python 3.4+)

**Installation**:

```bash
pip install requests
```

## Testing

**Storage Module Tests**:

- `test_storage.py` includes comprehensive tests for storage module
- Tests cover text truncation, config storage, metrics aggregation

**Running Tests**:

```bash
cd eval/scripts
python -m pytest test_storage.py -v
```

## Future Enhancements

**Nice-to-Have** (can be added incrementally):

- **Claim-Level Check**: Judge each claim individually against context (more granular than answer-level)
- **Enhanced Judge Prompt**: Reduce false positives with additional instructions
- **Web UI for Labeling**: Replace CLI with web UI for bulk labeling
- **HTML Reports**: Upgrade from terminal reports to HTML reports with charts
- **Automatic Test Case Generation**: Extract questions from notes automatically
- **External Sanity Suite**: Add second eval file for general/adversarial questions

## Summary

The evaluation framework provides objective, repeatable measurements that enable:

- **Data-Driven Decisions**: Quantify impact of model/hardware changes
- **Regression Detection**: Catch quality degradation early
- **Optimization Guidance**: Identify which components need improvement
- **Historical Tracking**: See improvement trends over time
- **Configuration Comparison**: Compare different setups objectively

Retrieval metrics are stable and don't drift. LLM-judge metrics have controlled drift through fixed judge model, prompt version, and temperature=0, enabling meaningful comparisons across runs.

