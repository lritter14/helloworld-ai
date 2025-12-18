#!/usr/bin/env python3
"""
Retrieval metrics calculator for evaluation framework.

Computes retrieval metrics (Recall@K, MRR, Precision@K, Scope Miss Rate,
Attribution Hit Rate) using anchor-based matching that is resilient to
chunking strategy changes.

Usage:
    python eval/scripts/score_retrieval.py --run-id <run_id> --eval-set eval/eval_set.jsonl
"""

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Set, Tuple

# Import storage module
sys.path.insert(0, str(Path(__file__).parent))
from storage import RetrievalMetrics, TestResult, load_results, load_config


def normalize_heading_path(heading_path: str) -> str:
    """
    Normalize heading path for consistent matching.

    - Strip heading level markers (#, ##, ###, etc.) from each heading
    - Strip extra spaces
    - Use consistent delimiter: " > "
    - Handle edge cases

    This allows matching regardless of heading level (e.g., "# Title" matches "## Title")

    Args:
        heading_path: Raw heading path from chunk or gold support

    Returns:
        Normalized heading path with heading levels stripped
    """
    if not heading_path:
        return ""

    import re

    # Strip leading/trailing whitespace
    normalized = heading_path.strip()

    # Normalize delimiter - replace multiple spaces, tabs, or ">" with " > "
    # Replace any sequence of whitespace and ">" with " > "
    normalized = re.sub(r'\s*>\s*', ' > ', normalized)

    # Split by delimiter to process each heading individually
    parts = normalized.split(' > ')
    normalized_parts = []

    for part in parts:
        # Strip heading level markers (#, ##, ###, etc.) from the start of each heading
        # This allows matching regardless of heading level
        part = re.sub(r'^#+\s*', '', part.strip())
        # Collapse multiple spaces to single space
        part = re.sub(r'\s+', ' ', part)
        if part:  # Only add non-empty parts
            normalized_parts.append(part.strip())

    # Rejoin with normalized delimiter
    normalized = ' > '.join(normalized_parts)

    # Final strip
    normalized = normalized.strip()

    return normalized


def matches_gold_support(
    chunk_rel_path: str,
    chunk_heading_path: str,
    chunk_text: str,
    gold_rel_path: str,
    gold_heading_path: str,
    gold_snippets: Optional[List[str]] = None,
) -> bool:
    """
    Check if a retrieved chunk matches a gold support anchor.

    Match criteria:
    - Same rel_path (exact match)
    - Retrieved heading_path starts with gold heading_path (prefix match)
    - If snippets provided, chunk text must contain at least one snippet

    Args:
        chunk_rel_path: Relative path from retrieved chunk
        chunk_heading_path: Heading path from retrieved chunk
        chunk_text: Text content of retrieved chunk
        gold_rel_path: Relative path from gold support
        gold_heading_path: Heading path from gold support
        gold_snippets: Optional list of snippets that must appear in chunk text

    Returns:
        True if chunk matches gold support
    """
    # Normalize heading paths
    chunk_heading = normalize_heading_path(chunk_heading_path)
    gold_heading = normalize_heading_path(gold_heading_path)

    # Exact match on rel_path
    if chunk_rel_path != gold_rel_path:
        return False

    # Prefix match on heading_path (retrieved starts with gold)
    # This handles cases where chunking depth changes
    if not chunk_heading.startswith(gold_heading):
        return False

    # If snippets are provided, check that at least one appears in chunk text
    if gold_snippets:
        chunk_text_lower = chunk_text.lower()
        for snippet in gold_snippets:
            if snippet.lower() in chunk_text_lower:
                return True
        # If snippets provided but none match, it's not a match
        return False

    # If no snippets provided, match is valid if rel_path and heading_path match
    return True


def compute_recall_at_k(
    retrieved_chunks: List[Dict[str, Any]],
    gold_supports: List[Dict[str, Any]],
    k: int,
) -> Tuple[float, Optional[int]]:
    """
    Compute Recall@K (any) - did we retrieve at least one matching chunk?

    Args:
        retrieved_chunks: List of retrieved chunks (top K)
        gold_supports: List of gold support anchors
        k: K value (number of top chunks to consider)

    Returns:
        Tuple of (recall_score: 0.0 or 1.0, first_match_rank: None if no match)
    """
    # Consider only top K chunks
    top_k_chunks = retrieved_chunks[:k]

    # Check each chunk against each gold support
    for chunk in top_k_chunks:
        chunk_rel_path = chunk.get("rel_path", "")
        chunk_heading_path = chunk.get("heading_path", "")
        chunk_text = chunk.get("text", "")

        for gold_support in gold_supports:
            gold_rel_path = gold_support.get("rel_path", "")
            gold_heading_path = gold_support.get("heading_path", "")
            gold_snippets = gold_support.get("snippets")

            if matches_gold_support(
                chunk_rel_path,
                chunk_heading_path,
                chunk_text,
                gold_rel_path,
                gold_heading_path,
                gold_snippets,
            ):
                # Found a match - return recall=1.0 and rank
                rank = chunk.get("rank", 0)
                return (1.0, rank)

    # No match found
    return (0.0, None)


def compute_recall_all_at_k(
    retrieved_chunks: List[Dict[str, Any]],
    gold_supports: List[Dict[str, Any]],
    required_support_groups: List[List[int]],
    k: int,
) -> float:
    """
    Compute Recall_all@K for multi-hop questions.

    For multi-hop, we need to retrieve ALL required supports (per group logic).
    Groups are OR-of-groups, AND within group.

    Args:
        retrieved_chunks: List of retrieved chunks (top K)
        gold_supports: List of gold support anchors
        required_support_groups: List of groups, each group is list of indices into gold_supports
        k: K value

    Returns:
        Recall_all score: 1.0 if all required supports retrieved, 0.0 otherwise
    """
    if not required_support_groups:
        # If no groups specified, fall back to regular recall (any)
        recall_any, _ = compute_recall_at_k(retrieved_chunks, gold_supports, k)
        return recall_any

    # Consider only top K chunks
    top_k_chunks = retrieved_chunks[:k]

    # For each group, check if at least one support in the group is matched
    # Groups are OR (need at least one group to be fully satisfied)
    # Within group, supports are AND (all supports in group must be matched)
    for group in required_support_groups:
        # Check if all supports in this group are matched
        group_satisfied = True
        for support_idx in group:
            if support_idx >= len(gold_supports):
                # Invalid index
                group_satisfied = False
                break

            gold_support = gold_supports[support_idx]
            gold_rel_path = gold_support.get("rel_path", "")
            gold_heading_path = gold_support.get("heading_path", "")
            gold_snippets = gold_support.get("snippets")

            # Check if any chunk matches this support
            support_matched = False
            for chunk in top_k_chunks:
                chunk_rel_path = chunk.get("rel_path", "")
                chunk_heading_path = chunk.get("heading_path", "")
                chunk_text = chunk.get("text", "")

                if matches_gold_support(
                    chunk_rel_path,
                    chunk_heading_path,
                    chunk_text,
                    gold_rel_path,
                    gold_heading_path,
                    gold_snippets,
                ):
                    support_matched = True
                    break

            if not support_matched:
                group_satisfied = False
                break

        # If this group is fully satisfied, we're done (OR logic)
        if group_satisfied:
            return 1.0

    # No group was fully satisfied
    return 0.0


def compute_mrr(
    retrieved_chunks: List[Dict[str, Any]],
    gold_supports: List[Dict[str, Any]],
    k: int,
) -> float:
    """
    Compute MRR (Mean Reciprocal Rank) - 1/rank of first matching chunk.

    Args:
        retrieved_chunks: List of retrieved chunks (top K)
        gold_supports: List of gold support anchors
        k: K value

    Returns:
        MRR score (0.0 if no match found, otherwise 1/rank)
    """
    recall, first_match_rank = compute_recall_at_k(retrieved_chunks, gold_supports, k)

    if first_match_rank is None:
        return 0.0

    return 1.0 / first_match_rank


def compute_precision_at_k(
    retrieved_chunks: List[Dict[str, Any]],
    gold_supports: List[Dict[str, Any]],
    k: int,
) -> float:
    """
    Compute Precision@K - fraction of top K chunks that match any gold_support.

    Args:
        retrieved_chunks: List of retrieved chunks (top K)
        gold_supports: List of gold support anchors
        k: K value

    Returns:
        Precision score (0.0 to 1.0)
    """
    if k == 0:
        return 0.0

    # Consider only top K chunks
    top_k_chunks = retrieved_chunks[:k]

    # Count how many chunks match any gold support
    matching_count = 0
    for chunk in top_k_chunks:
        chunk_rel_path = chunk.get("rel_path", "")
        chunk_heading_path = chunk.get("heading_path", "")
        chunk_text = chunk.get("text", "")

        # Check against all gold supports
        for gold_support in gold_supports:
            gold_rel_path = gold_support.get("rel_path", "")
            gold_heading_path = gold_support.get("heading_path", "")
            gold_snippets = gold_support.get("snippets")

            if matches_gold_support(
                chunk_rel_path,
                chunk_heading_path,
                chunk_text,
                gold_rel_path,
                gold_heading_path,
                gold_snippets,
            ):
                matching_count += 1
                break  # Count each chunk only once

    return matching_count / k


def compute_scope_miss(
    retrieved_chunks: List[Dict[str, Any]],
    gold_supports: List[Dict[str, Any]],
    folder_mode: str,
    folder_selection_info: Optional[Dict[str, Any]] = None,
) -> bool:
    """
    Compute scope miss - did folder selection exclude all gold supports?

    Only calculated when folder_mode is "on" or "on_with_fallback".

    Args:
        retrieved_chunks: List of retrieved chunks
        gold_supports: List of gold support anchors
        folder_mode: Folder selection mode ("off", "on", "on_with_fallback")
        folder_selection_info: Optional folder selection debug info from API

    Returns:
        True if scope miss occurred (all gold supports excluded), False otherwise
    """
    # Only compute for folder modes that use folder selection
    if folder_mode not in ["on", "on_with_fallback"]:
        return False

    # If no gold supports (e.g., unanswerable questions), not a scope miss
    if not gold_supports:
        return False

    # If we retrieved at least one matching chunk, no scope miss
    recall, _ = compute_recall_at_k(retrieved_chunks, gold_supports, len(retrieved_chunks))
    if recall > 0:
        return False

    # No matching chunks retrieved - this could be a scope miss
    # However, we can't definitively say it's a scope miss without knowing
    # which folders were selected. For now, we'll be conservative and only
    # mark as scope miss if we have folder selection info that shows
    # gold supports were in excluded folders.

    # TODO: If folder_selection_info is available, check if gold supports
    # were in excluded folders. For now, we'll return False (not a scope miss)
    # if we can't determine definitively.

    # For now, if no chunks match and folder mode is on, assume scope miss
    # This is a conservative approach - may have false positives
    return True


def compute_attribution_hit(
    references: List[Dict[str, Any]],
    gold_supports: List[Dict[str, Any]],
    answerable: bool,
) -> bool:
    """
    Compute attribution hit - did final cited references include at least one matching gold_support?

    Only computed for answerable questions.

    Args:
        references: List of references from answer (from API response)
        gold_supports: List of gold support anchors
        answerable: Whether question is answerable

    Returns:
        True if any reference matches a gold support, False otherwise
    """
    if not answerable:
        # Not computed for unanswerable questions
        return False

    if not references:
        # No references cited
        return False

    # Check each reference against gold supports
    for ref in references:
        # Extract rel_path and heading_path from reference
        # Reference format: vault, rel_path, heading_path, chunk_index
        ref_rel_path = ref.get("rel_path", "")
        ref_heading_path = ref.get("heading_path", "")
        ref_text = ""  # References don't include text, only metadata

        for gold_support in gold_supports:
            gold_rel_path = gold_support.get("rel_path", "")
            gold_heading_path = gold_support.get("heading_path", "")
            gold_snippets = gold_support.get("snippets")

            if matches_gold_support(
                ref_rel_path,
                ref_heading_path,
                ref_text,
                gold_rel_path,
                gold_heading_path,
                gold_snippets,
            ):
                return True

    return False


def compute_retrieval_metrics_for_test(
    test_result: Dict[str, Any],
    test_case: Dict[str, Any],
) -> RetrievalMetrics:
    """
    Compute all retrieval metrics for a single test case.

    Args:
        test_result: Test result from results.jsonl
        test_case: Test case from eval_set.jsonl

    Returns:
        RetrievalMetrics object with all computed metrics
    """
    retrieved_chunks = test_result.get("retrieved_chunks", [])
    references = test_result.get("references", [])
    config = test_result.get("config", {})
    k = config.get("k", 5)
    folder_mode = config.get("folder_mode", "off")

    gold_supports = test_case.get("gold_supports", [])
    answerable = test_case.get("answerable", True)
    required_support_groups = test_case.get("required_support_groups")
    category = test_case.get("category", "")

    # Compute Recall@K (any)
    recall_at_k, _ = compute_recall_at_k(retrieved_chunks, gold_supports, k)

    # Compute Recall_all@K for multi-hop
    recall_all_at_k = None
    if category == "multi_hop" and required_support_groups:
        recall_all_at_k = compute_recall_all_at_k(
            retrieved_chunks, gold_supports, required_support_groups, k
        )

    # Compute MRR
    mrr = compute_mrr(retrieved_chunks, gold_supports, k)

    # Compute Precision@K
    precision_at_k = compute_precision_at_k(retrieved_chunks, gold_supports, k)

    # Compute Scope Miss
    folder_selection_info = test_result.get("debug", {}).get("folder_selection")
    scope_miss = compute_scope_miss(
        retrieved_chunks, gold_supports, folder_mode, folder_selection_info
    )

    # Compute Attribution Hit
    attribution_hit = compute_attribution_hit(references, gold_supports, answerable)

    return RetrievalMetrics(
        recall_at_k=recall_at_k,
        recall_all_at_k=recall_all_at_k,
        mrr=mrr,
        precision_at_k=precision_at_k,
        scope_miss=scope_miss,
        attribution_hit=attribution_hit,
    )


def load_test_cases(eval_set_path: Path) -> Dict[str, Dict[str, Any]]:
    """
    Load test cases from eval_set.jsonl and index by test case ID.

    Args:
        eval_set_path: Path to eval_set.jsonl

    Returns:
        Dictionary mapping test_case_id to test case
    """
    test_cases = {}
    with open(eval_set_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                test_case = json.loads(line)
                test_id = test_case.get("id", "")
                if test_id:
                    test_cases[test_id] = test_case
            except json.JSONDecodeError as e:
                print(f"Warning: Failed to parse line in {eval_set_path}: {e}", file=sys.stderr)
    return test_cases


def update_results_with_metrics(
    run_dir: Path,
    eval_set_path: Path,
    output_path: Optional[Path] = None,
) -> None:
    """
    Load results, compute retrieval metrics, and update results file.

    Args:
        run_dir: Path to run directory containing results.jsonl
        eval_set_path: Path to eval_set.jsonl
        output_path: Optional path to write updated results (default: overwrite results.jsonl)
    """
    # Load test cases
    test_cases = load_test_cases(eval_set_path)

    # Load results
    results = load_results(str(run_dir))

    if not results:
        print(f"Warning: No results found in {run_dir}", file=sys.stderr)
        return

    # Compute metrics for each result
    updated_results = []
    for result in results:
        test_case_id = result.get("test_case_id", "")
        test_case = test_cases.get(test_case_id)

        if not test_case:
            print(
                f"Warning: Test case {test_case_id} not found in eval_set.jsonl",
                file=sys.stderr,
            )
            # Keep result without metrics
            updated_results.append(result)
            continue

        # Compute retrieval metrics
        retrieval_metrics = compute_retrieval_metrics_for_test(result, test_case)

        # Update result with metrics
        result["retrieval_metrics"] = retrieval_metrics.to_dict()
        updated_results.append(result)

    # Write updated results
    output_file = output_path or (run_dir / "results.jsonl")
    with open(output_file, "w", encoding="utf-8") as f:
        for result in updated_results:
            f.write(json.dumps(result, ensure_ascii=False) + "\n")

    print(f"Updated {len(updated_results)} results with retrieval metrics")
    print(f"Results written to: {output_file}")


def aggregate_retrieval_metrics(
    run_dir: Path,
    eval_set_path: Path,
) -> Dict[str, Any]:
    """
    Aggregate retrieval metrics across all test cases.

    Args:
        run_dir: Path to run directory containing results.jsonl
        eval_set_path: Path to eval_set.jsonl

    Returns:
        Dictionary with aggregated metrics
    """
    # Load test cases
    test_cases = load_test_cases(eval_set_path)

    # Load results
    results = load_results(str(run_dir))

    if not results:
        return {}

    # Aggregate metrics
    recall_scores = []
    recall_all_scores = []
    mrr_scores = []
    precision_scores = []
    scope_misses = []
    attribution_hits = []
    answerable_count = 0

    for result in results:
        test_case_id = result.get("test_case_id", "")
        test_case = test_cases.get(test_case_id)

        if not test_case:
            continue

        answerable = test_case.get("answerable", True)

        # Get metrics (compute if not present)
        retrieval_metrics = result.get("retrieval_metrics")
        if not retrieval_metrics:
            retrieval_metrics = compute_retrieval_metrics_for_test(result, test_case)
            retrieval_metrics = retrieval_metrics.to_dict()

        recall = retrieval_metrics.get("recall_at_k")
        recall_all = retrieval_metrics.get("recall_all_at_k")
        mrr = retrieval_metrics.get("mrr")
        precision = retrieval_metrics.get("precision_at_k")
        scope_miss = retrieval_metrics.get("scope_miss")
        attribution_hit = retrieval_metrics.get("attribution_hit")

        if recall is not None:
            recall_scores.append(recall)
        if recall_all is not None:
            recall_all_scores.append(recall_all)
        if mrr is not None:
            mrr_scores.append(mrr)
        if precision is not None:
            precision_scores.append(precision)
        if scope_miss is not None:
            scope_misses.append(scope_miss)
        if attribution_hit is not None and answerable:
            attribution_hits.append(attribution_hit)
            answerable_count += 1

    # Compute averages
    aggregate = {}

    if recall_scores:
        aggregate["recall_at_k_avg"] = sum(recall_scores) / len(recall_scores)

    if recall_all_scores:
        aggregate["recall_all_at_k_avg"] = sum(recall_all_scores) / len(recall_all_scores)

    if mrr_scores:
        aggregate["mrr_avg"] = sum(mrr_scores) / len(mrr_scores)

    if precision_scores:
        aggregate["precision_at_k_avg"] = sum(precision_scores) / len(precision_scores)

    if scope_misses:
        aggregate["scope_miss_rate"] = sum(scope_misses) / len(scope_misses)

    if attribution_hits:
        aggregate["attribution_hit_rate"] = sum(attribution_hits) / len(attribution_hits)

    return aggregate


def main():
    parser = argparse.ArgumentParser(
        description="Compute retrieval metrics for evaluation run",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--run-id",
        type=str,
        required=True,
        help="Run ID (directory name in results/)",
    )
    parser.add_argument(
        "--results-dir",
        type=Path,
        default=Path("eval/results"),
        help="Results directory (default: eval/results)",
    )
    parser.add_argument(
        "--eval-set",
        type=Path,
        default=Path("eval/eval_set.jsonl"),
        help="Path to eval_set.jsonl (default: eval/eval_set.jsonl)",
    )
    parser.add_argument(
        "--aggregate-only",
        action="store_true",
        help="Only compute aggregate metrics, don't update results.jsonl",
    )
    parser.add_argument(
        "--output-metrics",
        type=Path,
        help="Path to write aggregated metrics JSON (default: print to stdout)",
    )

    args = parser.parse_args()

    # Validate paths
    run_dir = args.results_dir / args.run_id
    if not run_dir.exists():
        print(f"Error: Run directory not found: {run_dir}", file=sys.stderr)
        sys.exit(1)

    if not args.eval_set.exists():
        print(f"Error: Eval set file not found: {args.eval_set}", file=sys.stderr)
        sys.exit(1)

    # Update results with metrics (unless aggregate-only)
    if not args.aggregate_only:
        update_results_with_metrics(run_dir, args.eval_set)

    # Compute aggregate metrics
    aggregate = aggregate_retrieval_metrics(run_dir, args.eval_set)

    # Output aggregate metrics
    if args.output_metrics:
        with open(args.output_metrics, "w", encoding="utf-8") as f:
            json.dump(aggregate, f, indent=2, ensure_ascii=False)
        print(f"Aggregate metrics written to: {args.output_metrics}")
    else:
        print("\nAggregate Retrieval Metrics:")
        print(json.dumps(aggregate, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()

