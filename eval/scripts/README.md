# Evaluation Scripts

This directory contains Python scripts for the evaluation framework.

## Full Evaluation Pipeline

The `run_full_eval.py` script provides a single entry point that runs all evaluation scripts in the correct order:

1. **run_eval.py** - Execute evaluation suite against Go API
2. **score_retrieval.py** - Compute retrieval metrics (Recall@K, MRR, etc.)
3. **judge_answers.py** - Judge answer quality (optional, if judge model provided)
4. **score_abstention.py** - Compute abstention metrics

### Usage

```bash
# Full evaluation with judges
python eval/scripts/run_full_eval.py \
    --eval-set eval/eval_set.jsonl \
    --judge-model qwen2.5-14b

# Retrieval-only (fast, no judge cost)
python eval/scripts/run_full_eval.py \
    --eval-set eval/eval_set.jsonl \
    --retrieval-only

# Custom configuration
python eval/scripts/run_full_eval.py \
    --eval-set eval/eval_set.jsonl \
    --k 10 \
    --judge-model qwen2.5-14b \
    --judge-base-url http://localhost:8081

# Skip specific steps
python eval/scripts/run_full_eval.py \
    --eval-set eval/eval_set.jsonl \
    --judge-model qwen2.5-14b \
    --skip-retrieval-metrics \
    --skip-abstention-metrics
```

### Pipeline Steps

The pipeline automatically:
- Runs the evaluation suite and captures results
- Computes retrieval metrics (unless `--skip-retrieval-metrics`)
- Judges answer quality if judge model is provided (unless `--retrieval-only` or `--skip-judges`)
- Computes abstention metrics (unless `--skip-abstention-metrics`)
- Outputs a summary with run ID and results location

### Arguments

The script accepts all arguments from the individual scripts:
- **run_eval.py arguments**: `--api-url`, `--k`, `--rerank-vector-weight`, `--folder-mode`, `--retrieval-only`, etc.
- **judge_answers.py arguments**: `--judge-model`, `--judge-base-url`, `--judge-temperature`, `--cache-dir`, etc.
- **Control flags**: `--skip-retrieval-metrics`, `--skip-abstention-metrics`, `--skip-judges`

### Output

Results are stored in `eval/results/<run_id>/`:
- `results.jsonl`: Individual test results with all metrics
- `metrics.json`: Aggregated metrics across all tests
- `config.json`: Run configuration snapshot

## Labeling Workflow

The `label_eval.py` script provides an interactive workflow for marking gold_supports in test cases. This creates ground truth for retrieval metrics using anchor-based labeling (rel_path + heading_path) that is resilient to chunking changes.

### Features

- **Interactive Chunk Selection**: Display retrieved chunks and allow selection of which ones contain the answer
- **Anchor-Based Labeling**: Uses rel_path + heading_path (not chunk IDs) for resilience to chunking changes
- **Automatic Snippet Extraction**: Extracts snippets from selected chunks
- **Answerable Flag**: Mark questions as answerable or unanswerable
- **Backup Creation**: Automatically creates backup before saving changes

### Usage

```bash
# Label all test cases (run from project root)
python eval/scripts/label_eval.py --eval-set eval/eval_set.jsonl --api-url http://localhost:9000

# Label a specific test case
python eval/scripts/label_eval.py --eval-set eval/eval_set.jsonl --test-id test_001

# Skip already labeled test cases
python eval/scripts/label_eval.py --eval-set eval/eval_set.jsonl --skip-labeled
```

### Workflow

1. Script loads test cases from `eval_set.jsonl`
2. For each test case:
   - Calls API with `K=20` and `debug=true` to retrieve chunks
   - Displays chunks with their text, rel_path, heading_path, and scores
   - User selects which chunks contain the answer (using numbers to toggle)
   - User marks question as answerable/unanswerable
3. Creates `gold_supports` with anchor-based format:
   - `rel_path`: Relative path to the note file
   - `heading_path`: Heading hierarchy path (e.g., "# Overview > ## Details")
   - `snippets`: Optional exact phrases/quotes extracted from chunk text
4. Saves updated test cases back to `eval_set.jsonl` (with backup)

### Interactive Commands

- `<number>`: Toggle selection of chunk (1-N)
- `all`: Select all chunks
- `none`: Deselect all chunks
- `done`: Finish selection and continue
- `quit`: Quit without saving (saves progress so far)

### Requirements

- `requests` library: `pip install requests`
- API must support `debug=true` query parameter on `/api/v1/ask` endpoint

## Storage Module

The `storage.py` module provides data structures and utilities for storing evaluation results.

### Storage Features

- **Results Storage**: Write test results to JSONL format (one line per test case)
- **Metrics Storage**: Write aggregated metrics to JSON format
- **Config Storage**: Store run configuration snapshots
- **Text Truncation**: Default truncation to 200 characters, with option to store full text
- **Latency Breakdown**: Track timing for folder selection, retrieval, generation, and judge phases
- **Cost Tracking**: Track judge tokens and estimated costs
- **Judge Input Storage**: Store full judge input payload for reproducibility

### Storage Usage

```python
from storage import (
    ResultsWriter,
    TestResult,
    RetrievedChunk,
    RunConfig,
    LatencyBreakdown,
    RetrievalMetrics,
    GroundednessScore,
    CorrectnessScore,
    AbstentionResult,
    JudgeInput,
    CostTracking,
)

# Initialize writer
writer = ResultsWriter(output_dir="eval/results", run_id="run_20240115_001")

# Create configuration
config = RunConfig(
    k=5,
    rerank_weights={"vector": 0.7, "lexical": 0.3},
    folder_mode="on_with_fallback",
    llm_model="llama3.2",
    embedding_model="granite-278m",
    judge_model="qwen2.5-14b",
    judge_prompt_version="v1.0",
    judge_temperature=0.0,
)

# Write configuration
writer.write_config(config, eval_set_commit_hash="abc123")

# Create test result
result = TestResult(
    test_case_id="test_001",
    question="What is the main topic?",
    answer="The project is about RAG systems.",
    references=[...],
    retrieved_chunks=[...],
    config=config,
    latency=LatencyBreakdown(total_ms=1234, retrieval_ms=200, generation_ms=900),
    retrieval_metrics=RetrievalMetrics(recall_at_k=1.0, mrr=0.5),
    groundedness=GroundednessScore(score=4.5, reasoning="..."),
    correctness=CorrectnessScore(score=4.0, reasoning="..."),
    judge_input=JudgeInput(question="...", answer="...", context_chunk_ids=[...]),
    cost=CostTracking(judge_tokens=500, judge_cost_usd=0.001),
)

# Write result (truncated text by default)
writer.write_result(result, store_full_text=False)

# Or write with full text
writer.write_result(result, store_full_text=True)

# Write aggregated metrics
metrics = {
    "recall_at_k_avg": 0.85,
    "mrr_avg": 0.72,
    "groundedness_avg": 4.2,
    "latency": {"p50_ms": 1200, "p95_ms": 2500},
    "cost": {"judge_total_usd": 0.05, "judge_total_tokens": 25000},
}

writer.write_metrics(metrics, config_hash="hash123", eval_set_commit_hash="abc123")
```

### Loading Results

```python
from storage import load_results, load_metrics, load_config

# Load all results from a run
results = load_results("eval/results/run_20240115_001")

# Load metrics
metrics = load_metrics("eval/results/run_20240115_001")

# Load configuration
config = load_config("eval/results/run_20240115_001")
```

### Data Structures

All data structures are dataclasses that can be easily converted to/from dictionaries:

- `RetrievedChunk`: Represents a retrieved chunk with scores and metadata
- `RunConfig`: Configuration snapshot for a run
- `IndexingCoverage`: Indexing coverage statistics
- `LatencyBreakdown`: Latency breakdown for different phases
- `RetrievalMetrics`: Retrieval metrics (Recall@K, MRR, etc.)
- `GroundednessScore`: Groundedness score and details
- `CorrectnessScore`: Correctness score and reasoning
- `AbstentionResult`: Abstention result for unanswerable questions
- `JudgeInput`: Judge input payload for reproducibility
- `CostTracking`: Cost tracking for judge calls
- `TestResult`: Complete test result for a single test case

### Text Truncation

By default, chunk text is truncated to 200 characters to keep results lightweight. To store full text, pass `store_full_text=True` to `write_result()`.

### File Structure

Results are stored in the following structure:

```text
eval/results/
  <run_id>/
    results.jsonl      # One line per test case
    metrics.json       # Aggregated metrics
    config.json        # Run configuration
```
