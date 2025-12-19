#!/usr/bin/env python3
"""
Evaluation runner script for chatbot evaluation framework.

Executes test suite against Go API and stores results with full configuration
snapshot, latency tracking, cost tracking, and operational metrics.

Usage:
    python eval/scripts/run_eval.py \
        --eval-set eval/eval_set.jsonl \
        --k 5 \
        --folder-mode on_with_fallback \
        --judge-model qwen2.5-14b \
        --output-dir eval/results

    # Retrieval-only mode (fast, no judge cost)
    python eval/scripts/run_eval.py \
        --eval-set eval/eval_set.jsonl \
        --retrieval-only
"""

import argparse
import hashlib
import json
import subprocess
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

try:
    import requests
except ImportError:
    print("Error: 'requests' library is required. Install with: pip install requests")
    sys.exit(1)

# Import storage module
sys.path.insert(0, str(Path(__file__).parent))
from storage import (
    AbstentionResult,
    CostTracking,
    IndexingCoverage,
    LatencyBreakdown,
    RetrievedChunk,
    ResultsWriter,
    RunConfig,
    TestResult,
)


def get_git_commit_hash(file_path: Path) -> Optional[str]:
    """Get git commit hash for a file."""
    try:
        result = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=file_path.parent,
            capture_output=True,
            text=True,
            check=True,
        )
        return result.stdout.strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        return None


def load_env_file(project_root: Path) -> Dict[str, str]:
    """
    Load environment variables from .env file in project root.
    
    Returns a dictionary of key-value pairs from the .env file.
    Handles basic .env file format (KEY=VALUE, ignores comments and empty lines).
    """
    env_vars = {}
    env_path = project_root / ".env"
    
    if not env_path.exists():
        return env_vars
    
    try:
        with open(env_path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                # Skip empty lines and comments
                if not line or line.startswith("#"):
                    continue
                # Parse KEY=VALUE format
                if "=" in line:
                    key, value = line.split("=", 1)
                    key = key.strip()
                    value = value.strip()
                    # Remove quotes if present
                    if value.startswith('"') and value.endswith('"'):
                        value = value[1:-1]
                    elif value.startswith("'") and value.endswith("'"):
                        value = value[1:-1]
                    env_vars[key] = value
    except Exception as e:
        print(f"Warning: Failed to load .env file: {e}", file=sys.stderr)
    
    return env_vars


def find_project_root(start_path: Path) -> Optional[Path]:
    """Find project root by looking for go.mod file."""
    current = start_path.resolve()
    for _ in range(10):  # Limit search depth
        if (current / "go.mod").exists():
            return current
        parent = current.parent
        if parent == current:
            break
        current = parent
    return None


def compute_config_hash(config: RunConfig) -> str:
    """Compute hash of configuration for quick comparison."""
    config_dict = config.to_dict()
    config_str = json.dumps(config_dict, sort_keys=True)
    return hashlib.sha256(config_str.encode()).hexdigest()[:16]


def load_eval_set(eval_set_path: Path) -> List[Dict[str, Any]]:
    """Load test cases from JSONL file."""
    test_cases = []
    with open(eval_set_path, "r", encoding="utf-8") as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                test_case = json.loads(line)
                test_cases.append(test_case)
            except json.JSONDecodeError as e:
                print(f"Warning: Failed to parse line {line_num} in {eval_set_path}: {e}")
    return test_cases


def call_api(
    api_url: str,
    question: str,
    vaults: List[str],
    folders: List[str],
    k: int,
    folder_mode: Optional[str] = None,
    timeout: int = 120,
) -> Dict[str, Any]:
    """
    Call the ask API with debug mode enabled.

    Args:
        api_url: Base URL of the API
        question: Question to ask
        vaults: List of vault names
        folders: List of folder paths
        k: Number of chunks to retrieve
        folder_mode: Folder selection mode (off, on, on_with_fallback) - not yet supported by API
        timeout: Request timeout in seconds

    Returns:
        API response dictionary
    """
    url = f"{api_url.rstrip('/')}/api/v1/ask?debug=true"
    payload = {
        "question": question,
        "k": k,
    }
    if vaults:
        payload["vaults"] = vaults
    if folders:
        payload["folders"] = folders

    # Note: folder_mode is not yet supported by the API, but we include it
    # in the config for future use
    if folder_mode:
        # For now, folder_mode is a hint for future API support
        # The current API always does folder selection
        pass

    try:
        response = requests.post(url, json=payload, timeout=timeout)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.Timeout:
        raise Exception(f"API request timed out after {timeout} seconds")
    except requests.exceptions.RequestException as e:
        raise Exception(f"API request failed: {e}")


def parse_retrieved_chunks(debug_info: Optional[Dict[str, Any]]) -> List[RetrievedChunk]:
    """Parse retrieved chunks from debug info."""
    if not debug_info:
        return []

    chunks_data = debug_info.get("retrieved_chunks", [])
    chunks = []
    for chunk_data in chunks_data:
        chunk = RetrievedChunk(
            chunk_id=chunk_data.get("chunk_id", ""),
            rel_path=chunk_data.get("rel_path", ""),
            heading_path=chunk_data.get("heading_path", ""),
            rank=chunk_data.get("rank", 0),
            score_vector=chunk_data.get("score_vector", 0.0),
            score_lexical=chunk_data.get("score_lexical"),
            score_final=chunk_data.get("score_final", 0.0),
            text=chunk_data.get("text", ""),
            token_count=chunk_data.get("token_count"),
        )
        chunks.append(chunk)

    return chunks


def parse_indexing_coverage(api_response: Dict[str, Any]) -> Optional[IndexingCoverage]:
    """
    Parse indexing coverage stats from API response.

    Note: This may not be available in the current API implementation.
    Returns None if not available.
    """
    # Check if indexing coverage is in debug info or top-level
    coverage_data = api_response.get("indexing_coverage")
    if not coverage_data:
        return None

    return IndexingCoverage(
        docs_processed=coverage_data.get("docs_processed", 0),
        docs_with_0_chunks=coverage_data.get("docs_with_0_chunks", 0),
        chunks_attempted=coverage_data.get("chunks_attempted", 0),
        chunks_embedded=coverage_data.get("chunks_embedded", 0),
        chunks_skipped=coverage_data.get("chunks_skipped", 0),
        chunks_skipped_reasons=coverage_data.get("chunks_skipped_reasons", {}),
        chunk_token_stats=coverage_data.get("chunk_token_stats"),
        chunker_version=coverage_data.get("chunker_version"),
        index_version=coverage_data.get("index_version"),
    )


def parse_latency_breakdown(api_response: Dict[str, Any]) -> Optional[LatencyBreakdown]:
    """
    Parse latency breakdown from API response.

    Note: This may not be available in the current API implementation.
    Returns None if not available.
    """
    latency_data = api_response.get("latency")
    if not latency_data:
        return None

    return LatencyBreakdown(
        total_ms=latency_data.get("total_ms", 0.0),
        folder_selection_ms=latency_data.get("folder_selection_ms"),
        retrieval_ms=latency_data.get("retrieval_ms"),
        generation_ms=latency_data.get("generation_ms"),
        judge_ms=latency_data.get("judge_ms"),
    )


@dataclass
class OperationalMetrics:
    """Operational metrics for a run."""

    total_tests: int = 0
    successful_tests: int = 0
    error_tests: int = 0
    timeout_tests: int = 0
    empty_response_tests: int = 0
    error_rate: float = 0.0
    timeout_rate: float = 0.0
    empty_response_rate: float = 0.0
    coverage_by_doc_type: Dict[str, Dict[str, int]] = field(default_factory=dict)

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return {
            "error_rate": self.error_rate,
            "timeout_rate": self.timeout_rate,
            "empty_response_rate": self.empty_response_rate,
            "coverage_by_doc_type": self.coverage_by_doc_type,
        }


def run_test_case(
    test_case: Dict[str, Any],
    api_url: str,
    k: int,
    folder_mode: Optional[str],
    timeout: int,
) -> tuple[Optional[TestResult], Optional[Exception], float]:
    """
    Run a single test case and return result.

    Returns:
        Tuple of (TestResult, Exception if error occurred, latency_ms)
    """
    test_id = test_case.get("id", "unknown")
    question = test_case.get("question", "")
    vaults = test_case.get("vaults", [])
    folders = test_case.get("folders", [])

    start_time = time.time()
    error = None
    api_response = None

    try:
        api_response = call_api(api_url, question, vaults, folders, k, folder_mode, timeout)
    except Exception as e:
        error = e
        api_response = None

    elapsed_ms = (time.time() - start_time) * 1000

    if error:
        return None, error, elapsed_ms

    # Parse response
    answer = api_response.get("answer", "")
    references = api_response.get("references", [])
    abstained = api_response.get("abstained", False)
    abstain_reason = api_response.get("abstain_reason", "")

    # Check for empty response
    if not answer and not abstained:
        error = Exception("Empty response (no answer and not abstained)")

    # Parse debug info
    debug_info = api_response.get("debug")
    retrieved_chunks = parse_retrieved_chunks(debug_info)

    # Parse optional fields
    indexing_coverage = parse_indexing_coverage(api_response)
    latency_breakdown = parse_latency_breakdown(api_response)

    # Create abstention result if applicable
    abstention = None
    if abstained:
        # For unanswerable questions, abstention is correct behavior
        # For answerable questions, abstention may indicate a problem
        # We'll determine hallucinated based on test case answerable flag later
        abstention = AbstentionResult(abstained=True, hallucinated=False)

    # Create test result (without metrics - those are computed later)
    result = TestResult(
        test_case_id=test_id,
        question=question,
        answer=answer,
        references=references,
        retrieved_chunks=retrieved_chunks,
        config=None,  # Will be set by caller
        indexing_coverage=indexing_coverage,
        latency=latency_breakdown or LatencyBreakdown(total_ms=elapsed_ms),
        abstention=abstention,
    )

    return result, None, elapsed_ms


def aggregate_operational_metrics(results: List[TestResult], errors: List[Exception]) -> OperationalMetrics:
    """Aggregate operational metrics from results."""
    total = len(results) + len(errors)
    successful = len([r for r in results if r and r.answer])
    error_count = len([e for e in errors if e is not None])
    timeout_count = len([e for e in errors if e and "timeout" in str(e).lower()])
    # Empty response: no answer and didn't abstain (or abstention is None/False)
    empty_count = len([
        r for r in results
        if r and not r.answer and (not r.abstention or not r.abstention.abstained)
    ])

    metrics = OperationalMetrics(
        total_tests=total,
        successful_tests=successful,
        error_tests=error_count,
        timeout_tests=timeout_count,
        empty_response_tests=empty_count,
        error_rate=error_count / total if total > 0 else 0.0,
        timeout_rate=timeout_count / total if total > 0 else 0.0,
        empty_response_rate=empty_count / total if total > 0 else 0.0,
    )

    # Aggregate coverage by doc type (if available)
    # Note: This requires doc type information in indexing_coverage
    # For now, we'll aggregate by file extension from rel_path
    coverage_by_type: Dict[str, Dict[str, int]] = {}
    for result in results:
        if not result or not result.indexing_coverage:
            continue

        # Extract doc type from rel_path (markdown, etc.)
        # This is a simplified approach - actual implementation may vary
        for chunk in result.retrieved_chunks:
            rel_path = chunk.rel_path
            if not rel_path:
                continue

            # Determine doc type from extension
            ext = Path(rel_path).suffix.lower()
            doc_type = ext[1:] if ext else "unknown"  # Remove leading dot

            if doc_type not in coverage_by_type:
                coverage_by_type[doc_type] = {
                    "processed": 0,
                    "with_0_chunks": 0,
                    "chunks_skipped": 0,
                }

            # Aggregate stats (simplified - actual aggregation would need more data)
            # For now, we just track that we saw this doc type
            coverage_by_type[doc_type]["processed"] += 1

    metrics.coverage_by_doc_type = coverage_by_type

    return metrics


def aggregate_indexing_coverage(results: List[TestResult]) -> Optional[IndexingCoverage]:
    """Aggregate indexing coverage stats across all results."""
    coverage_list = [r.indexing_coverage for r in results if r and r.indexing_coverage]
    if not coverage_list:
        return None

    # Aggregate stats
    total_docs = sum(c.docs_processed for c in coverage_list)
    total_docs_0_chunks = sum(c.docs_with_0_chunks for c in coverage_list)
    total_chunks_attempted = sum(c.chunks_attempted for c in coverage_list)
    total_chunks_embedded = sum(c.chunks_embedded for c in coverage_list)
    total_chunks_skipped = sum(c.chunks_skipped for c in coverage_list)

    # Aggregate skipped reasons
    skipped_reasons: Dict[str, int] = {}
    for c in coverage_list:
        for reason, count in c.chunks_skipped_reasons.items():
            skipped_reasons[reason] = skipped_reasons.get(reason, 0) + count

    # Aggregate token stats (take mean of means, min of mins, max of maxes)
    token_stats = None
    if all(c.chunk_token_stats for c in coverage_list):
        token_stats = {
            "min": min(c.chunk_token_stats["min"] for c in coverage_list if c.chunk_token_stats),
            "max": max(c.chunk_token_stats["max"] for c in coverage_list if c.chunk_token_stats),
            "mean": sum(c.chunk_token_stats["mean"] for c in coverage_list if c.chunk_token_stats) / len(coverage_list),
            "p95": sum(c.chunk_token_stats["p95"] for c in coverage_list if c.chunk_token_stats) / len(coverage_list),
        }

    # Use version from first coverage (should be same across all)
    chunker_version = coverage_list[0].chunker_version if coverage_list else None
    index_version = coverage_list[0].index_version if coverage_list else None

    return IndexingCoverage(
        docs_processed=total_docs,
        docs_with_0_chunks=total_docs_0_chunks,
        chunks_attempted=total_chunks_attempted,
        chunks_embedded=total_chunks_embedded,
        chunks_skipped=total_chunks_skipped,
        chunks_skipped_reasons=skipped_reasons,
        chunk_token_stats=token_stats,
        chunker_version=chunker_version,
        index_version=index_version,
    )


def main():
    parser = argparse.ArgumentParser(
        description="Run evaluation suite against Go API",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--eval-set",
        type=Path,
        default=Path("eval/eval_set.jsonl"),
        help="Path to eval_set.jsonl file (default: eval/eval_set.jsonl)",
    )
    parser.add_argument(
        "--api-url",
        type=str,
        default="http://localhost:9000",
        help="Base URL of the API (default: http://localhost:9000)",
    )
    parser.add_argument(
        "--k",
        type=int,
        default=5,
        help="Number of chunks to retrieve (default: 5)",
    )
    parser.add_argument(
        "--rerank-vector-weight",
        type=float,
        default=0.7,
        help="Vector weight for reranking (default: 0.7)",
    )
    parser.add_argument(
        "--rerank-lexical-weight",
        type=float,
        default=0.3,
        help="Lexical weight for reranking (default: 0.3)",
    )
    parser.add_argument(
        "--folder-mode",
        type=str,
        choices=["off", "on", "on_with_fallback"],
        default="on_with_fallback",
        help="Folder selection mode (default: on_with_fallback). Note: API may not support this yet.",
    )
    parser.add_argument(
        "--judge-model",
        type=str,
        help="Judge model name (e.g., qwen2.5-14b). Only used if not in retrieval-only mode.",
    )
    parser.add_argument(
        "--judge-temperature",
        type=float,
        default=0.0,
        help="Judge temperature (default: 0.0)",
    )
    parser.add_argument(
        "--judge-prompt-version",
        type=str,
        default="v1.0",
        help="Judge prompt version (default: v1.0)",
    )
    parser.add_argument(
        "--output-dir",
        type=str,
        default="eval/results",
        help="Output directory for results (default: eval/results)",
    )
    parser.add_argument(
        "--retrieval-only",
        action="store_true",
        help="Run retrieval metrics only (skip judge calls, faster iteration)",
    )
    parser.add_argument(
        "--store-full-text",
        action="store_true",
        help="Store full chunk text (default: truncate to 200 chars)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=120,
        help="Request timeout in seconds (default: 120)",
    )
    parser.add_argument(
        "--llm-model",
        type=str,
        help="LLM model name (for config tracking)",
    )
    parser.add_argument(
        "--embedding-model",
        type=str,
        help="Embedding model name (for config tracking)",
    )
    parser.add_argument(
        "--limit",
        type=int,
        help="Limit number of test cases to process (for faster iteration)",
    )

    args = parser.parse_args()

    # Load test cases
    if not args.eval_set.exists():
        print(f"Error: Eval set file not found: {args.eval_set}")
        sys.exit(1)

    test_cases = load_eval_set(args.eval_set)
    total_test_cases = len(test_cases)
    
    # Apply limit if specified
    if args.limit and args.limit > 0:
        test_cases = test_cases[:args.limit]
        print(f"Loaded {total_test_cases} test cases from {args.eval_set} (limiting to {len(test_cases)} for faster iteration)")
    else:
        print(f"Loaded {len(test_cases)} test cases from {args.eval_set}")

    # Get eval set commit hash
    eval_set_commit_hash = get_git_commit_hash(args.eval_set)

    # Load model names from .env if not provided via CLI
    llm_model = args.llm_model
    embedding_model = args.embedding_model
    
    # Load from .env if either is missing
    if not llm_model or not embedding_model:
        # Find project root (where go.mod is)
        script_dir = Path(__file__).parent
        project_root = find_project_root(script_dir)
        
        # If project root not found, try looking for .env relative to script directory
        # (go up a few levels from eval/scripts/ to find project root)
        if not project_root:
            # Try going up from eval/scripts/ to find .env
            current = script_dir
            for _ in range(5):  # Limit search depth
                parent = current.parent
                if (parent / ".env").exists():
                    project_root = parent
                    break
                if parent == current:
                    break
                current = parent
        
        if project_root:
            env_vars = load_env_file(project_root)
            if not llm_model and "LLM_MODEL" in env_vars:
                llm_model = env_vars["LLM_MODEL"]
                print(f"Using LLM model from .env: {llm_model}")
            if not embedding_model and "EMBEDDING_MODEL_NAME" in env_vars:
                embedding_model = env_vars["EMBEDDING_MODEL_NAME"]
                print(f"Using embedding model from .env: {embedding_model}")

    # Create run ID
    run_id = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%S")

    # Create run config
    config = RunConfig(
        k=args.k,
        rerank_weights={"vector": args.rerank_vector_weight, "lexical": args.rerank_lexical_weight},
        folder_mode=args.folder_mode,
        llm_model=llm_model,
        embedding_model=embedding_model,
        judge_model=args.judge_model if not args.retrieval_only else None,
        judge_prompt_version=args.judge_prompt_version if not args.retrieval_only else None,
        judge_temperature=args.judge_temperature if not args.retrieval_only else 0.0,
        dataset_version=eval_set_commit_hash,
    )

    # Create results writer
    writer = ResultsWriter(args.output_dir, run_id)
    writer.write_config(config, eval_set_commit_hash)

    print(f"\nStarting evaluation run: {run_id}")
    print(f"Configuration:")
    print(f"  K: {args.k}")
    print(f"  Folder mode: {args.folder_mode}")
    print(f"  Retrieval-only: {args.retrieval_only}")
    print(f"  API URL: {args.api_url}")
    print()

    # Run test cases
    results: List[TestResult] = []
    errors: List[Optional[Exception]] = []
    latencies: List[float] = []

    for i, test_case in enumerate(test_cases, 1):
        test_id = test_case.get("id", f"test_{i}")
        print(f"[{i}/{len(test_cases)}] Running {test_id}...", end=" ", flush=True)

        result, error, latency_ms = run_test_case(
            test_case,
            args.api_url,
            args.k,
            args.folder_mode,
            args.timeout,
        )

        if error:
            print(f"ERROR: {error}")
            errors.append(error)
            results.append(None)  # Placeholder for indexing
        else:
            result.config = config  # Set config on result
            results.append(result)
            errors.append(None)
            print(f"OK ({latency_ms:.0f}ms)")

        latencies.append(latency_ms)

        # Write result immediately (streaming)
        if result:
            writer.write_result(result, store_full_text=args.store_full_text)

    # Filter out None results for aggregation
    valid_results = [r for r in results if r is not None]

    # Aggregate operational metrics
    operational_metrics = aggregate_operational_metrics(valid_results, errors)

    # Aggregate indexing coverage
    aggregated_coverage = aggregate_indexing_coverage(valid_results)

    # Compute latency stats
    latency_p50 = sorted(latencies)[len(latencies) // 2] if latencies else 0.0
    latency_p95 = sorted(latencies)[int(len(latencies) * 0.95)] if latencies else 0.0
    latency_total = sum(latencies)

    # Build metrics dictionary
    metrics = {
        "operational_metrics": operational_metrics.to_dict(),
        "latency": {
            "p50_ms": latency_p50,
            "p95_ms": latency_p95,
            "total_ms": latency_total,
        },
    }

    if aggregated_coverage:
        metrics["indexing_coverage"] = aggregated_coverage.to_dict()

    # Write metrics
    config_hash = compute_config_hash(config)
    writer.write_metrics(metrics, config_hash=config_hash, eval_set_commit_hash=eval_set_commit_hash)

    print(f"\nEvaluation complete!")
    print(f"Results written to: {writer.get_results_path()}")
    print(f"Metrics written to: {writer.get_metrics_path()}")
    print(f"Config written to: {writer.get_config_path()}")
    print(f"\nSummary:")
    print(f"  Total tests: {len(test_cases)}")
    print(f"  Successful: {operational_metrics.successful_tests}")
    print(f"  Errors: {operational_metrics.error_tests}")
    print(f"  Timeouts: {operational_metrics.timeout_tests}")
    print(f"  Empty responses: {operational_metrics.empty_response_tests}")
    print(f"  Error rate: {operational_metrics.error_rate:.2%}")
    print(f"  Latency p50: {latency_p50:.0f}ms")
    print(f"  Latency p95: {latency_p95:.0f}ms")


if __name__ == "__main__":
    main()

