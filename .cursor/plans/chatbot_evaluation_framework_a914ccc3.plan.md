---
name: Chatbot Evaluation Framework
overview: Create a comprehensive evaluation framework to measure chatbot effectiveness over time, tracking improvements as models, hardware, and strategies change. Includes test suite management, multiple evaluation methods (LLM-as-judge, retrieval metrics, embedding quality), results storage, and automated reporting.
todos:
  - id: test_suite_structure
    content: Create test case and suite data structures in internal/evaluation/testcase.go and suite.go
    status: pending
  - id: llm_judge_evaluator
    content: Implement LLM-as-judge evaluator in internal/evaluation/evaluators/llm_judge.go with cloud LLM integration
    status: pending
  - id: retrieval_evaluator
    content: Implement retrieval metrics evaluator (Precision@K, Recall@K, MRR) in internal/evaluation/evaluators/retrieval.go
    status: pending
  - id: embedding_evaluator
    content: Implement embedding quality evaluator in internal/evaluation/evaluators/embedding.go
    status: pending
  - id: results_storage
    content: Create results database schema and storage layer in internal/evaluation/storage/
    status: pending
  - id: test_runner
    content: Implement test runner with parallel execution and progress tracking in internal/evaluation/runner.go
    status: pending
  - id: cli_tool
    content: Create CLI tool in cmd/eval/main.go for running evaluations and generating reports
    status: pending
  - id: test_generator
    content: Implement test case generator in internal/evaluation/generator.go for extracting questions from notes
    status: pending
  - id: report_generator
    content: Create report generator with HTML output and charts in internal/evaluation/reports/reporter.go
    status: pending
  - id: configuration
    content: Add evaluation configuration support in internal/evaluation/config.go with LLM-as-judge settings
    status: pending
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

- JSON/YAML file format for test cases
- CLI tool to add/edit test cases
- Validation of test case format
- Support for bulk import/export

### 6. Reporting and Visualization (`reports/`)

**Report Generation** (`reporter.go`):

- HTML reports with charts (using a simple charting library or static HTML)
- Comparison reports between runs
- Trend analysis over time
- Category breakdowns (by difficulty, category)
- Configuration impact analysis

**Metrics Dashboard**:

- Overall accuracy percentage
- Per-category scores
- Retrieval quality metrics
- Embedding quality metrics
- Cost tracking (for LLM-as-judge)
- Performance trends over time

### 7. Configuration (`config.go`)

**LLM-as-Judge Configuration**:

- Judge model selection (OpenAI GPT-4, Claude, etc.)
- API keys (from environment variables)
- Rate limiting settings
- Cost tracking enabled/disabled

**Evaluation Settings**:

- Which evaluators to run (LLM-as-judge, retrieval, embedding)
- Parallel execution settings
- Retry logic configuration
- Timeout settings

### 8. Integration Points

**RAG Engine Integration**:

- Use existing `rag.Engine` interface
- Capture RAG responses (answer + references)
- No modifications needed to existing RAG code

**Storage Integration**:

- New SQLite database for evaluation results (separate from main DB)
- Reuse existing storage patterns and interfaces

**Configuration Integration**:

- Read current system config from `internal/config`
- Capture model names, embedding model, vector size
- Track hardware info (optional, via system calls)

## Implementation Details

### File Structure

```
internal/evaluation/
├── testcase.go          # Test case data structures
├── suite.go             # Test suite management
├── runner.go            # Test execution engine
├── generator.go         # Test case generation
├── config.go            # Evaluation configuration
├── evaluators/
│   ├── llm_judge.go     # LLM-as-judge evaluator
│   ├── retrieval.go     # Retrieval metrics evaluator
│   └── embedding.go     # Embedding quality evaluator
├── storage/
│   ├── results.go       # Results database operations
│   └── models.go        # Database models
└── reports/
    └── reporter.go       # Report generation

cmd/eval/
└── main.go              # CLI tool entry point

testdata/
└── test_suite.json      # Default test suite file
```

### Database SchemaCREATE TABLE evaluation_runs (
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
    llm_scores TEXT,  -- JSON (correctness, completeness, relevance, overall)
    retrieval_metrics TEXT,  -- JSON (precision@k, recall@k, mrr)
    embedding_metrics TEXT,  -- JSON
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

### Test Case JSON Format

```json
{
  "version": "1.0",
  "description": "Initial test suite",
  "test_cases": [
    {
      "id": "test_001",
      "question": "What is the main topic of the project?",
      "expected_answer": "The project is about...",
      "expected_references": [
        {
          "vault": "personal",
          "rel_path": "projects/main.md",
          "heading_path": "# Overview"
        }
      ],
      "category": "factual",
      "difficulty": "easy",
      "vaults": ["personal"],
      "folders": ["projects"]
    }
  ]
}
```

### CLI Usage Examples

```bash
# Run full evaluation suite
./bin/eval run

# Run specific category
./bin/eval run --category factual

# Compare with previous run
./bin/eval compare --run-id <id>

# Generate test cases from notes
./bin/eval generate --vault personal --output testdata/new_cases.json

# View report
./bin/eval report --run-id <id>

# List all runs
./bin/eval list
```

## Evaluation Workflow

1. **Setup**: Configure LLM-as-judge API keys, select evaluators
2. **Test Execution**: Run test suite against current system
3. **Results Storage**: Save results with configuration snapshot
4. **Analysis**: Generate reports and compare with previous runs
5. **Iteration**: Make changes (model, hardware, strategy) and re-run

## Key Metrics Tracked

- **Answer Quality**: LLM-as-judge scores (correctness, completeness, relevance, overall)
- **Retrieval Quality**: Precision@K, Recall@K, MRR
- **Embedding Quality**: Semantic similarity scores, clustering metrics
- **Performance**: Execution time per test, total suite time
- **Cost**: LLM-as-judge API costs per run
- **Accuracy**: Percentage of tests passing thresholds

## Benefits

- **Data-Driven Decisions**: Quantify impact of model/hardware changes
- **Regression Detection**: Catch quality degradation early
- **Optimization Guidance**: Identify which components need improvement
- **Historical Tracking**: See improvement trends over time
- **Configuration Comparison**: Compare different setups objectively