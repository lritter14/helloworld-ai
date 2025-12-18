"""
Results storage module for evaluation framework.

Handles writing evaluation results to JSONL format (one line per test case)
and aggregated metrics to JSON format.
"""

import json
from dataclasses import dataclass, asdict, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional


# Default truncation length for chunk text
DEFAULT_TRUNCATE_LENGTH = 200


@dataclass
class RetrievedChunk:
    """Represents a retrieved chunk with scores and metadata."""

    chunk_id: str
    rel_path: str
    heading_path: str
    rank: int
    score_vector: float
    score_lexical: Optional[float] = None
    score_final: float = 0.0
    text: str = ""
    token_count: Optional[int] = None

    def to_dict(self, store_full_text: bool = False) -> Dict[str, Any]:
        """Convert to dictionary, optionally truncating text."""
        result = {
            "chunk_id": self.chunk_id,
            "rel_path": self.rel_path,
            "heading_path": self.heading_path,
            "rank": self.rank,
            "score_vector": self.score_vector,
            "score_final": self.score_final,
        }

        if self.score_lexical is not None:
            result["score_lexical"] = self.score_lexical

        # Truncate text unless store_full_text is True
        if store_full_text:
            result["text"] = self.text
        else:
            if len(self.text) > DEFAULT_TRUNCATE_LENGTH:
                result["text"] = self.text[:DEFAULT_TRUNCATE_LENGTH] + "..."
            else:
                result["text"] = self.text

        if self.token_count is not None:
            result["token_count"] = self.token_count

        return result


@dataclass
class RunConfig:
    """Configuration snapshot for a run."""

    k: int
    rerank_weights: Dict[str, float]
    folder_mode: str
    llm_model: Optional[str] = None
    embedding_model: Optional[str] = None
    judge_model: Optional[str] = None
    judge_prompt_version: Optional[str] = None
    judge_temperature: float = 0.0
    dataset_version: Optional[str] = None
    index_build_version: Optional[str] = None
    retriever_version: Optional[str] = None
    answerer_version: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return asdict(self)


@dataclass
class IndexingCoverage:
    """Indexing coverage statistics."""

    docs_processed: int
    docs_with_0_chunks: int
    chunks_attempted: int
    chunks_embedded: int
    chunks_skipped: int
    chunks_skipped_reasons: Dict[str, int] = field(default_factory=dict)
    chunk_token_stats: Optional[Dict[str, float]] = None
    chunker_version: Optional[str] = None
    index_version: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        result = asdict(self)
        # Remove None values
        return {k: v for k, v in result.items() if v is not None}


@dataclass
class LatencyBreakdown:
    """Latency breakdown for different phases."""

    total_ms: float
    folder_selection_ms: Optional[float] = None
    retrieval_ms: Optional[float] = None
    generation_ms: Optional[float] = None
    judge_ms: Optional[float] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        result = asdict(self)
        # Remove None values
        return {k: v for k, v in result.items() if v is not None}


@dataclass
class RetrievalMetrics:
    """Retrieval metrics for a test case."""

    recall_at_k: Optional[float] = None
    recall_all_at_k: Optional[float] = None
    mrr: Optional[float] = None
    precision_at_k: Optional[float] = None
    scope_miss: Optional[bool] = None
    attribution_hit: Optional[bool] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        result = asdict(self)
        # Remove None values
        return {k: v for k, v in result.items() if v is not None}


@dataclass
class GroundednessScore:
    """Groundedness score and details."""

    score: float
    reasoning: str
    unsupported_claims: List[str] = field(default_factory=list)
    supported_claims: List[str] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return asdict(self)


@dataclass
class CorrectnessScore:
    """Correctness score and reasoning."""

    score: float
    reasoning: str

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return asdict(self)


@dataclass
class AbstentionResult:
    """Abstention result for unanswerable questions."""

    abstained: bool
    hallucinated: bool

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return asdict(self)


@dataclass
class JudgeInput:
    """Judge input payload for reproducibility."""

    question: str
    answer: str
    context_chunk_ids: List[str] = field(default_factory=list)
    context_chunks_truncated: List[str] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return asdict(self)


@dataclass
class CostTracking:
    """Cost tracking for judge calls."""

    judge_tokens: int = 0
    judge_cost_usd: float = 0.0

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return asdict(self)


@dataclass
class TestResult:
    """Complete test result for a single test case."""

    test_case_id: str
    question: str
    answer: str
    references: List[Dict[str, Any]]
    retrieved_chunks: List[RetrievedChunk]
    config: RunConfig
    indexing_coverage: Optional[IndexingCoverage] = None
    latency: Optional[LatencyBreakdown] = None
    retrieval_metrics: Optional[RetrievalMetrics] = None
    groundedness: Optional[GroundednessScore] = None
    correctness: Optional[CorrectnessScore] = None
    abstention: Optional[AbstentionResult] = None
    judge_input: Optional[JudgeInput] = None
    cost: Optional[CostTracking] = None

    def to_dict(self, store_full_text: bool = False) -> Dict[str, Any]:
        """Convert to dictionary for JSONL output."""
        result = {
            "test_case_id": self.test_case_id,
            "question": self.question,
            "answer": self.answer,
            "references": self.references,
            "retrieved_chunks": [
                chunk.to_dict(store_full_text=store_full_text)
                for chunk in self.retrieved_chunks
            ],
            "config": self.config.to_dict(),
        }

        if self.indexing_coverage is not None:
            result["indexing_coverage"] = self.indexing_coverage.to_dict()

        if self.latency is not None:
            result["latency"] = self.latency.to_dict()

        if self.retrieval_metrics is not None:
            result["retrieval_metrics"] = self.retrieval_metrics.to_dict()

        if self.groundedness is not None:
            result["groundedness"] = self.groundedness.to_dict()

        if self.correctness is not None:
            result["correctness"] = self.correctness.to_dict()

        if self.abstention is not None:
            result["abstention"] = self.abstention.to_dict()

        if self.judge_input is not None:
            result["judge_input"] = self.judge_input.to_dict()

        if self.cost is not None:
            result["cost"] = self.cost.to_dict()

        return result


class ResultsWriter:
    """Writes evaluation results to JSONL and metrics to JSON."""

    def __init__(self, output_dir: str, run_id: str):
        """
        Initialize results writer.

        Args:
            output_dir: Base output directory (e.g., 'eval/results')
            run_id: Unique run identifier
        """
        self.output_dir = Path(output_dir)
        self.run_id = run_id
        self.run_dir = self.output_dir / run_id
        self.results_file = self.run_dir / "results.jsonl"
        self.metrics_file = self.run_dir / "metrics.json"
        self.config_file = self.run_dir / "config.json"

        # Create run directory
        self.run_dir.mkdir(parents=True, exist_ok=True)

    def write_result(self, result: TestResult, store_full_text: bool = False):
        """
        Write a single test result to results.jsonl.

        Args:
            result: TestResult to write
            store_full_text: If True, store full chunk text; otherwise truncate to 200 chars
        """
        result_dict = result.to_dict(store_full_text=store_full_text)
        with open(self.results_file, "a", encoding="utf-8") as f:
            f.write(json.dumps(result_dict, ensure_ascii=False) + "\n")

    def write_config(self, config: RunConfig, eval_set_commit_hash: Optional[str] = None):
        """
        Write run configuration to config.json.

        Args:
            config: RunConfig to write
            eval_set_commit_hash: Optional git commit hash of eval_set.jsonl
        """
        config_dict = config.to_dict()
        if eval_set_commit_hash:
            config_dict["eval_set_commit_hash"] = eval_set_commit_hash

        with open(self.config_file, "w", encoding="utf-8") as f:
            json.dump(config_dict, f, indent=2, ensure_ascii=False)

    def write_metrics(
        self,
        metrics: Dict[str, Any],
        timestamp: Optional[str] = None,
        config_hash: Optional[str] = None,
        eval_set_commit_hash: Optional[str] = None,
    ):
        """
        Write aggregated metrics to metrics.json.

        Args:
            metrics: Dictionary containing aggregated metrics
            timestamp: ISO format timestamp (defaults to now)
            config_hash: Hash of configuration for quick comparison
            eval_set_commit_hash: Git commit hash of eval_set.jsonl
        """
        if timestamp is None:
            timestamp = datetime.now(timezone.utc).isoformat()

        metrics_dict = {
            "run_id": self.run_id,
            "timestamp": timestamp,
            "aggregate_metrics": metrics,
        }

        if config_hash:
            metrics_dict["config_hash"] = config_hash

        if eval_set_commit_hash:
            metrics_dict["eval_set_commit_hash"] = eval_set_commit_hash

        with open(self.metrics_file, "w", encoding="utf-8") as f:
            json.dump(metrics_dict, f, indent=2, ensure_ascii=False)

    def get_results_path(self) -> Path:
        """Get path to results.jsonl file."""
        return self.results_file

    def get_metrics_path(self) -> Path:
        """Get path to metrics.json file."""
        return self.metrics_file

    def get_config_path(self) -> Path:
        """Get path to config.json file."""
        return self.config_file


def load_results(run_dir: str) -> List[Dict[str, Any]]:
    """
    Load all results from a run directory.

    Args:
        run_dir: Path to run directory

    Returns:
        List of test result dictionaries
    """
    results_file = Path(run_dir) / "results.jsonl"
    if not results_file.exists():
        return []

    results = []
    with open(results_file, "r", encoding="utf-8") as f:
        for line in f:
            if line.strip():
                results.append(json.loads(line))

    return results


def load_metrics(run_dir: str) -> Dict[str, Any]:
    """
    Load metrics from a run directory.

    Args:
        run_dir: Path to run directory

    Returns:
        Metrics dictionary
    """
    metrics_file = Path(run_dir) / "metrics.json"
    if not metrics_file.exists():
        return {}

    with open(metrics_file, "r", encoding="utf-8") as f:
        return json.load(f)


def load_config(run_dir: str) -> Dict[str, Any]:
    """
    Load configuration from a run directory.

    Args:
        run_dir: Path to run directory

    Returns:
        Configuration dictionary
    """
    config_file = Path(run_dir) / "config.json"
    if not config_file.exists():
        return {}

    with open(config_file, "r", encoding="utf-8") as f:
        return json.load(f)

