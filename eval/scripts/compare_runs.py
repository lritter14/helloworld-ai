#!/usr/bin/env python3
"""
Run comparison tool for evaluation framework.

Compares two evaluation runs to identify improvements and regressions.
Checks eval configuration invariants to ensure meaningful comparisons.

Usage:
    python eval/scripts/compare_runs.py \
        --run-id-1 <baseline_run_id> \
        --run-id-2 <new_run_id> \
        [--ignore-invariants]

    # Example:
    python eval/scripts/compare_runs.py \
        --run-id-1 20251218_063617 \
        --run-id-2 20251219_120000
"""

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# Import storage module
sys.path.insert(0, str(Path(__file__).parent))
from storage import load_config, load_metrics, load_results


def check_invariants(
    config1: Dict[str, Any],
    config2: Dict[str, Any],
    metrics1: Dict[str, Any],
    metrics2: Dict[str, Any],
    ignore_invariants: bool = False,
) -> Tuple[bool, List[str]]:
    """
    Check eval configuration invariants between two runs.

    Invariants that must match for meaningful comparison:
    - Same eval set commit hash
    - Same judge model + judge prompt version
    - Same judge temperature
    - Same debug payload fields (implicitly checked via results structure)

    Args:
        config1: Configuration from run 1
        config2: Configuration from run 2
        metrics1: Metrics from run 1
        metrics2: Metrics from run 2
        ignore_invariants: If True, skip invariant checks

    Returns:
        Tuple of (all_match: bool, warnings: List[str])
    """
    if ignore_invariants:
        return True, []

    warnings = []
    all_match = True

    # Check eval set commit hash
    eval_hash1 = config1.get("eval_set_commit_hash") or metrics1.get("eval_set_commit_hash")
    eval_hash2 = config2.get("eval_set_commit_hash") or metrics2.get("eval_set_commit_hash")

    if eval_hash1 and eval_hash2:
        if eval_hash1 != eval_hash2:
            warnings.append(
                f"⚠️  Eval set commit hash differs: {eval_hash1[:8]} vs {eval_hash2[:8]}"
            )
            all_match = False
    elif eval_hash1 or eval_hash2:
        warnings.append("⚠️  One run missing eval_set_commit_hash (comparison may be invalid)")

    # Check judge model
    judge_model1 = config1.get("judge_model")
    judge_model2 = config2.get("judge_model")

    if judge_model1 and judge_model2:
        if judge_model1 != judge_model2:
            warnings.append(
                f"⚠️  Judge model differs: {judge_model1} vs {judge_model2}"
            )
            all_match = False
    elif judge_model1 or judge_model2:
        # Only warn if one is set and the other isn't (both None is OK for retrieval-only runs)
        if (judge_model1 is not None) != (judge_model2 is not None):
            warnings.append("⚠️  One run missing judge_model (comparison may be invalid)")

    # Check judge prompt version
    prompt_v1 = config1.get("judge_prompt_version")
    prompt_v2 = config2.get("judge_prompt_version")

    if prompt_v1 and prompt_v2:
        if prompt_v1 != prompt_v2:
            warnings.append(
                f"⚠️  Judge prompt version differs: {prompt_v1} vs {prompt_v2}"
            )
            all_match = False
    elif prompt_v1 or prompt_v2:
        if (prompt_v1 is not None) != (prompt_v2 is not None):
            warnings.append("⚠️  One run missing judge_prompt_version (comparison may be invalid)")

    # Check judge temperature
    temp1 = config1.get("judge_temperature")
    temp2 = config2.get("judge_temperature")

    if temp1 is not None and temp2 is not None:
        if abs(temp1 - temp2) > 0.001:  # Float comparison with tolerance
            warnings.append(
                f"⚠️  Judge temperature differs: {temp1} vs {temp2}"
            )
            all_match = False
    elif temp1 is not None or temp2 is not None:
        if (temp1 is not None) != (temp2 is not None):
            warnings.append("⚠️  One run missing judge_temperature (comparison may be invalid)")

    return all_match, warnings


def get_metric_value(metrics: Dict[str, Any], path: List[str], default: Any = None) -> Any:
    """
    Get nested metric value by path.

    Args:
        metrics: Metrics dictionary
        path: List of keys to traverse (e.g., ["aggregate_metrics", "recall_at_k_avg"])
        default: Default value if path doesn't exist

    Returns:
        Metric value or default
    """
    value = metrics
    for key in path:
        if isinstance(value, dict):
            value = value.get(key)
            if value is None:
                return default
        else:
            return default
    return value


def compare_metrics(
    metrics1: Dict[str, Any],
    metrics2: Dict[str, Any],
) -> Dict[str, Any]:
    """
    Compare aggregate metrics between two runs.

    Args:
        metrics1: Metrics from run 1 (baseline)
        metrics2: Metrics from run 2 (new)

    Returns:
        Dictionary with metric comparisons
    """
    agg1 = metrics1.get("aggregate_metrics", {})
    agg2 = metrics2.get("aggregate_metrics", {})

    comparisons = {}

    # Retrieval metrics
    recall1 = get_metric_value(agg1, ["recall_at_k_avg"])
    recall2 = get_metric_value(agg2, ["recall_at_k_avg"])
    if recall1 is not None and recall2 is not None:
        comparisons["recall_at_k_avg"] = {
            "baseline": recall1,
            "new": recall2,
            "delta": recall2 - recall1,
            "delta_pct": ((recall2 - recall1) / recall1 * 100) if recall1 != 0 else None,
        }

    mrr1 = get_metric_value(agg1, ["mrr_avg"])
    mrr2 = get_metric_value(agg2, ["mrr_avg"])
    if mrr1 is not None and mrr2 is not None:
        comparisons["mrr_avg"] = {
            "baseline": mrr1,
            "new": mrr2,
            "delta": mrr2 - mrr1,
            "delta_pct": ((mrr2 - mrr1) / mrr1 * 100) if mrr1 != 0 else None,
        }

    precision1 = get_metric_value(agg1, ["precision_at_k_avg"])
    precision2 = get_metric_value(agg2, ["precision_at_k_avg"])
    if precision1 is not None and precision2 is not None:
        comparisons["precision_at_k_avg"] = {
            "baseline": precision1,
            "new": precision2,
            "delta": precision2 - precision1,
            "delta_pct": ((precision2 - precision1) / precision1 * 100) if precision1 != 0 else None,
        }

    scope_miss1 = get_metric_value(agg1, ["scope_miss_rate"])
    scope_miss2 = get_metric_value(agg2, ["scope_miss_rate"])
    if scope_miss1 is not None and scope_miss2 is not None:
        comparisons["scope_miss_rate"] = {
            "baseline": scope_miss1,
            "new": scope_miss2,
            "delta": scope_miss2 - scope_miss1,
            "delta_pct": ((scope_miss2 - scope_miss1) / scope_miss1 * 100) if scope_miss1 != 0 else None,
        }

    attribution1 = get_metric_value(agg1, ["attribution_hit_rate"])
    attribution2 = get_metric_value(agg2, ["attribution_hit_rate"])
    if attribution1 is not None and attribution2 is not None:
        comparisons["attribution_hit_rate"] = {
            "baseline": attribution1,
            "new": attribution2,
            "delta": attribution2 - attribution1,
            "delta_pct": ((attribution2 - attribution1) / attribution1 * 100) if attribution1 != 0 else None,
        }

    # Answer quality metrics
    groundedness1 = get_metric_value(agg1, ["groundedness_avg"])
    groundedness2 = get_metric_value(agg2, ["groundedness_avg"])
    if groundedness1 is not None and groundedness2 is not None:
        comparisons["groundedness_avg"] = {
            "baseline": groundedness1,
            "new": groundedness2,
            "delta": groundedness2 - groundedness1,
            "delta_pct": ((groundedness2 - groundedness1) / groundedness1 * 100) if groundedness1 != 0 else None,
        }

    correctness1 = get_metric_value(agg1, ["correctness_avg"])
    correctness2 = get_metric_value(agg2, ["correctness_avg"])
    if correctness1 is not None and correctness2 is not None:
        comparisons["correctness_avg"] = {
            "baseline": correctness1,
            "new": correctness2,
            "delta": correctness2 - correctness1,
            "delta_pct": ((correctness2 - correctness1) / correctness1 * 100) if correctness1 != 0 else None,
        }

    # Abstention metrics
    abstention1 = get_metric_value(agg1, ["abstention_accuracy"])
    abstention2 = get_metric_value(agg2, ["abstention_accuracy"])
    if abstention1 is not None and abstention2 is not None:
        comparisons["abstention_accuracy"] = {
            "baseline": abstention1,
            "new": abstention2,
            "delta": abstention2 - abstention1,
            "delta_pct": ((abstention2 - abstention1) / abstention1 * 100) if abstention1 != 0 else None,
        }

    hallucination1 = get_metric_value(agg1, ["hallucination_rate_unanswerable"])
    hallucination2 = get_metric_value(agg2, ["hallucination_rate_unanswerable"])
    if hallucination1 is not None and hallucination2 is not None:
        comparisons["hallucination_rate_unanswerable"] = {
            "baseline": hallucination1,
            "new": hallucination2,
            "delta": hallucination2 - hallucination1,
            "delta_pct": ((hallucination2 - hallucination1) / hallucination1 * 100) if hallucination1 != 0 else None,
        }

    # Latency metrics
    latency1 = agg1.get("latency", {})
    latency2 = agg2.get("latency", {})
    if latency1 and latency2:
        p50_1 = latency1.get("p50_ms")
        p50_2 = latency2.get("p50_ms")
        if p50_1 is not None and p50_2 is not None:
            comparisons["latency_p50_ms"] = {
                "baseline": p50_1,
                "new": p50_2,
                "delta": p50_2 - p50_1,
                "delta_pct": ((p50_2 - p50_1) / p50_1 * 100) if p50_1 != 0 else None,
            }

        p95_1 = latency1.get("p95_ms")
        p95_2 = latency2.get("p95_ms")
        if p95_1 is not None and p95_2 is not None:
            comparisons["latency_p95_ms"] = {
                "baseline": p95_1,
                "new": p95_2,
                "delta": p95_2 - p95_1,
                "delta_pct": ((p95_2 - p95_1) / p95_1 * 100) if p95_1 != 0 else None,
            }

    return comparisons


def find_test_changes(
    results1: List[Dict[str, Any]],
    results2: List[Dict[str, Any]],
) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    """
    Find test cases that changed between runs.

    Returns:
        Tuple of (regressions: List, improvements: List)
        Each item contains test_case_id, question, and metric changes
    """
    # Index results by test_case_id
    results1_dict = {r.get("test_case_id"): r for r in results1}
    results2_dict = {r.get("test_case_id"): r for r in results2}

    regressions = []
    improvements = []

    # Find common test cases
    common_ids = set(results1_dict.keys()) & set(results2_dict.keys())

    for test_id in common_ids:
        r1 = results1_dict[test_id]
        r2 = results2_dict[test_id]

        # Compare key metrics
        metrics1 = r1.get("retrieval_metrics", {})
        metrics2 = r2.get("retrieval_metrics", {})

        recall1 = metrics1.get("recall_at_k", 0.0)
        recall2 = metrics2.get("recall_at_k", 0.0)

        groundedness1 = r1.get("groundedness", {}).get("score")
        groundedness2 = r2.get("groundedness", {}).get("score")

        correctness1 = r1.get("correctness", {}).get("score")
        correctness2 = r2.get("correctness", {}).get("score")

        # Determine if this is a regression or improvement
        # Regression: success (1.0 or high score) → failure (0.0 or low score)
        # Improvement: failure (0.0 or low score) → success (1.0 or high score)

        is_regression = False
        is_improvement = False
        changes = []

        # Check recall changes
        if recall1 == 1.0 and recall2 == 0.0:
            is_regression = True
            changes.append("recall_at_k: 1.0 → 0.0")
        elif recall1 == 0.0 and recall2 == 1.0:
            is_improvement = True
            changes.append("recall_at_k: 0.0 → 1.0")

        # Check groundedness changes (threshold: 4.0)
        if groundedness1 is not None and groundedness2 is not None:
            if groundedness1 >= 4.0 and groundedness2 < 3.0:
                is_regression = True
                changes.append(f"groundedness: {groundedness1:.1f} → {groundedness2:.1f}")
            elif groundedness1 < 3.0 and groundedness2 >= 4.0:
                is_improvement = True
                changes.append(f"groundedness: {groundedness1:.1f} → {groundedness2:.1f}")

        # Check correctness changes (threshold: 4.0)
        if correctness1 is not None and correctness2 is not None:
            if correctness1 >= 4.0 and correctness2 < 3.0:
                is_regression = True
                changes.append(f"correctness: {correctness1:.1f} → {correctness2:.1f}")
            elif correctness1 < 3.0 and correctness2 >= 4.0:
                is_improvement = True
                changes.append(f"correctness: {correctness1:.1f} → {correctness2:.1f}")

        if is_regression and changes:
            regressions.append({
                "test_case_id": test_id,
                "question": r1.get("question", ""),
                "changes": changes,
            })

        if is_improvement and changes:
            improvements.append({
                "test_case_id": test_id,
                "question": r1.get("question", ""),
                "changes": changes,
            })

    return regressions, improvements


def compare_configs(
    config1: Dict[str, Any],
    config2: Dict[str, Any],
) -> Dict[str, Any]:
    """
    Compare configurations between two runs.

    Args:
        config1: Configuration from run 1
        config2: Configuration from run 2

    Returns:
        Dictionary with configuration differences
    """
    differences = {}

    # Key config fields to compare
    fields_to_compare = [
        "k",
        "rerank_weights",
        "folder_mode",
        "llm_model",
        "embedding_model",
        "judge_model",
        "judge_prompt_version",
        "judge_temperature",
        "index_build_version",
        "retriever_version",
        "answerer_version",
    ]

    for field in fields_to_compare:
        val1 = config1.get(field)
        val2 = config2.get(field)

        if val1 != val2:
            differences[field] = {
                "baseline": val1,
                "new": val2,
            }

    return differences


def format_delta(value: float, delta: float, delta_pct: Optional[float] = None) -> str:
    """Format metric delta for display."""
    sign = "+" if delta >= 0 else ""
    if delta_pct is not None:
        return f"{value:.3f} ({sign}{delta:.3f}, {sign}{delta_pct:.1f}%)"
    return f"{value:.3f} ({sign}{delta:.3f})"


def print_comparison_report(
    run_id1: str,
    run_id2: str,
    metrics1: Dict[str, Any],
    metrics2: Dict[str, Any],
    config1: Dict[str, Any],
    config2: Dict[str, Any],
    results1: List[Dict[str, Any]],
    results2: List[Dict[str, Any]],
    comparisons: Dict[str, Any],
    regressions: List[Dict[str, Any]],
    improvements: List[Dict[str, Any]],
    config_diffs: Dict[str, Any],
    invariant_warnings: List[str],
) -> None:
    """Print formatted comparison report to terminal."""
    print("=" * 80)
    print("EVALUATION RUN COMPARISON")
    print("=" * 80)
    print(f"\nBaseline Run: {run_id1}")
    print(f"New Run:     {run_id2}")
    print(f"\nBaseline Timestamp: {metrics1.get('timestamp', 'N/A')}")
    print(f"New Timestamp:      {metrics2.get('timestamp', 'N/A')}")

    # Invariant warnings
    if invariant_warnings:
        print("\n" + "=" * 80)
        print("⚠️  INVARIANT WARNINGS")
        print("=" * 80)
        for warning in invariant_warnings:
            print(f"  {warning}")
        print("\n⚠️  Comparison may not be meaningful due to invariant differences!")
        print("    Use --ignore-invariants to proceed anyway.")

    # Configuration differences
    if config_diffs:
        print("\n" + "=" * 80)
        print("CONFIGURATION DIFFERENCES")
        print("=" * 80)
        for field, diff in config_diffs.items():
            print(f"\n  {field}:")
            print(f"    Baseline: {diff['baseline']}")
            print(f"    New:      {diff['new']}")

    # Metric comparisons
    if comparisons:
        print("\n" + "=" * 80)
        print("METRIC COMPARISONS")
        print("=" * 80)

        # Retrieval metrics
        retrieval_metrics = [
            "recall_at_k_avg",
            "mrr_avg",
            "precision_at_k_avg",
            "scope_miss_rate",
            "attribution_hit_rate",
        ]
        if any(k in comparisons for k in retrieval_metrics):
            print("\n  Retrieval Metrics:")
            for metric in retrieval_metrics:
                if metric in comparisons:
                    comp = comparisons[metric]
                    delta_str = format_delta(
                        comp["new"],
                        comp["delta"],
                        comp.get("delta_pct"),
                    )
                    arrow = "↑" if comp["delta"] > 0 else "↓" if comp["delta"] < 0 else "→"
                    print(f"    {metric:25s} {arrow} {delta_str}")

        # Answer quality metrics
        quality_metrics = ["groundedness_avg", "correctness_avg"]
        if any(k in comparisons for k in quality_metrics):
            print("\n  Answer Quality Metrics:")
            for metric in quality_metrics:
                if metric in comparisons:
                    comp = comparisons[metric]
                    delta_str = format_delta(
                        comp["new"],
                        comp["delta"],
                        comp.get("delta_pct"),
                    )
                    arrow = "↑" if comp["delta"] > 0 else "↓" if comp["delta"] < 0 else "→"
                    print(f"    {metric:25s} {arrow} {delta_str}")

        # Abstention metrics
        abstention_metrics = ["abstention_accuracy", "hallucination_rate_unanswerable"]
        if any(k in comparisons for k in abstention_metrics):
            print("\n  Abstention Metrics:")
            for metric in abstention_metrics:
                if metric in comparisons:
                    comp = comparisons[metric]
                    delta_str = format_delta(
                        comp["new"],
                        comp["delta"],
                        comp.get("delta_pct"),
                    )
                    arrow = "↑" if comp["delta"] > 0 else "↓" if comp["delta"] < 0 else "→"
                    print(f"    {metric:25s} {arrow} {delta_str}")

        # Latency metrics
        latency_metrics = ["latency_p50_ms", "latency_p95_ms"]
        if any(k in comparisons for k in latency_metrics):
            print("\n  Latency Metrics:")
            for metric in latency_metrics:
                if metric in comparisons:
                    comp = comparisons[metric]
                    delta_str = format_delta(
                        comp["new"],
                        comp["delta"],
                        comp.get("delta_pct"),
                    )
                    arrow = "↓" if comp["delta"] < 0 else "↑" if comp["delta"] > 0 else "→"
                    print(f"    {metric:25s} {arrow} {delta_str}")

    # Regressions
    if regressions:
        print("\n" + "=" * 80)
        print(f"TOP REGRESSIONS ({len(regressions)} total)")
        print("=" * 80)
        for i, reg in enumerate(regressions[:10], 1):  # Show top 10
            print(f"\n  {i}. {reg['test_case_id']}")
            print(f"     Question: {reg['question'][:70]}...")
            print(f"     Changes: {', '.join(reg['changes'])}")

    # Improvements
    if improvements:
        print("\n" + "=" * 80)
        print(f"TOP IMPROVEMENTS ({len(improvements)} total)")
        print("=" * 80)
        for i, imp in enumerate(improvements[:10], 1):  # Show top 10
            print(f"\n  {i}. {imp['test_case_id']}")
            print(f"     Question: {imp['question'][:70]}...")
            print(f"     Changes: {', '.join(imp['changes'])}")

    print("\n" + "=" * 80)


def main():
    parser = argparse.ArgumentParser(
        description="Compare two evaluation runs to identify improvements and regressions",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--run-id-1",
        required=True,
        help="Baseline run ID (directory name in results/)",
    )
    parser.add_argument(
        "--run-id-2",
        required=True,
        help="New run ID (directory name in results/)",
    )
    parser.add_argument(
        "--results-dir",
        default="eval/results",
        help="Base results directory (default: eval/results)",
    )
    parser.add_argument(
        "--ignore-invariants",
        action="store_true",
        help="Ignore invariant checks and proceed with comparison",
    )

    args = parser.parse_args()

    # Resolve run directories
    results_dir = Path(args.results_dir)
    run_dir1 = results_dir / args.run_id_1
    run_dir2 = results_dir / args.run_id_2

    if not run_dir1.exists():
        print(f"Error: Baseline run directory not found: {run_dir1}", file=sys.stderr)
        sys.exit(1)

    if not run_dir2.exists():
        print(f"Error: New run directory not found: {run_dir2}", file=sys.stderr)
        sys.exit(1)

    # Load data
    print(f"Loading baseline run: {run_dir1}")
    metrics1 = load_metrics(str(run_dir1))
    config1 = load_config(str(run_dir1))
    results1 = load_results(str(run_dir1))

    print(f"Loading new run: {run_dir2}")
    metrics2 = load_metrics(str(run_dir2))
    config2 = load_config(str(run_dir2))
    results2 = load_results(str(run_dir2))

    if not metrics1 or not metrics2:
        print("Error: Could not load metrics from one or both runs", file=sys.stderr)
        sys.exit(1)

    # Check invariants
    invariant_match, invariant_warnings = check_invariants(
        config1,
        config2,
        metrics1,
        metrics2,
        ignore_invariants=args.ignore_invariants,
    )

    if not invariant_match and not args.ignore_invariants:
        print("\n" + "=" * 80)
        print("⚠️  INVARIANT CHECK FAILED")
        print("=" * 80)
        for warning in invariant_warnings:
            print(f"  {warning}")
        print("\n⚠️  Comparison may not be meaningful due to invariant differences!")
        print("    Use --ignore-invariants to proceed anyway.")
        sys.exit(1)

    # Compare metrics
    comparisons = compare_metrics(metrics1, metrics2)

    # Find test changes
    regressions, improvements = find_test_changes(results1, results2)

    # Compare configs
    config_diffs = compare_configs(config1, config2)

    # Print report
    print_comparison_report(
        args.run_id_1,
        args.run_id_2,
        metrics1,
        metrics2,
        config1,
        config2,
        results1,
        results2,
        comparisons,
        regressions,
        improvements,
        config_diffs,
        invariant_warnings,
    )


if __name__ == "__main__":
    main()

