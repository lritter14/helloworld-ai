#!/usr/bin/env python3
"""
Abstention metrics calculator for evaluation framework.

Computes abstention metrics (Abstention Accuracy, Hallucination Rate on Unanswerable)
for questions where answerable=false. These metrics measure whether the system knows
when not to answer, which is critical for real RAG systems.

Usage:
    python eval/scripts/score_abstention.py --run-id <run_id> --eval-set eval/eval_set.jsonl
"""

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional

# Import storage module
sys.path.insert(0, str(Path(__file__).parent))
from storage import AbstentionResult, load_results


def compute_abstention_metrics_for_test(
    test_result: Dict[str, Any],
    test_case: Dict[str, Any],
) -> Optional[AbstentionResult]:
    """
    Compute abstention metrics for a single test case.

    Only computed for questions where answerable=false.

    Args:
        test_result: Test result from results.jsonl
        test_case: Test case from eval_set.jsonl

    Returns:
        AbstentionResult if test case is unanswerable, None otherwise
    """
    answerable = test_case.get("answerable", True)

    # Only compute abstention metrics for unanswerable questions
    if answerable:
        return None

    # Get abstained flag from result
    # The API response should have included abstained field
    # Check if abstention is already in result
    abstention_data = test_result.get("abstention")
    answer = test_result.get("answer", "")
    
    if abstention_data:
        # Abstention already computed - use it but ensure hallucinated is correct
        abstained = abstention_data.get("abstained", False)
        # For unanswerable questions: if not abstained, then hallucinated
        hallucinated = not abstained
    else:
        # Abstention not yet computed - infer from answer
        # For unanswerable questions:
        # - Empty answer → likely abstained (correct behavior)
        # - Non-empty answer → not abstained (hallucinated)
        if not answer or answer.strip() == "":
            # Empty answer indicates abstention
            abstained = True
            hallucinated = False
        else:
            # Non-empty answer means system answered (should have abstained)
            abstained = False
            hallucinated = True

    return AbstentionResult(abstained=abstained, hallucinated=hallucinated)


def update_results_with_abstention(
    run_dir: Path,
    eval_set_path: Path,
    output_path: Optional[Path] = None,
) -> None:
    """
    Load results, compute abstention metrics, and update results file.

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

        # Compute abstention metrics
        abstention = compute_abstention_metrics_for_test(result, test_case)

        # Update result with metrics
        if abstention:
            result["abstention"] = abstention.to_dict()
        updated_results.append(result)

    # Write updated results
    output_file = output_path or (run_dir / "results.jsonl")
    with open(output_file, "w", encoding="utf-8") as f:
        for result in updated_results:
            f.write(json.dumps(result, ensure_ascii=False) + "\n")

    print(f"Updated {len(updated_results)} results with abstention metrics")
    print(f"Results written to: {output_file}")


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


def aggregate_abstention_metrics(
    run_dir: Path,
    eval_set_path: Path,
) -> Dict[str, Any]:
    """
    Aggregate abstention metrics across all unanswerable test cases.

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

    # Aggregate metrics for unanswerable questions only
    abstention_accuracies = []
    hallucination_flags = []
    unanswerable_count = 0

    for result in results:
        test_case_id = result.get("test_case_id", "")
        test_case = test_cases.get(test_case_id)

        if not test_case:
            continue

        answerable = test_case.get("answerable", True)

        # Only compute metrics for unanswerable questions
        if not answerable:
            unanswerable_count += 1

            # Get abstention metrics (compute if not present)
            abstention = result.get("abstention")
            if not abstention:
                abstention = compute_abstention_metrics_for_test(result, test_case)
                if abstention:
                    abstention = abstention.to_dict()

            if abstention:
                abstained = abstention.get("abstained", False)
                hallucinated = abstention.get("hallucinated", False)

                # Abstention accuracy: 1 if abstained, 0 if not
                abstention_accuracy = 1.0 if abstained else 0.0
                abstention_accuracies.append(abstention_accuracy)

                # Hallucination flag: 1 if hallucinated, 0 if not
                hallucination_flag = 1.0 if hallucinated else 0.0
                hallucination_flags.append(hallucination_flag)

    aggregate = {}

    if abstention_accuracies:
        aggregate["abstention_accuracy"] = sum(abstention_accuracies) / len(abstention_accuracies)
    else:
        aggregate["abstention_accuracy"] = None

    if hallucination_flags:
        aggregate["hallucination_rate_unanswerable"] = sum(hallucination_flags) / len(hallucination_flags)
    else:
        aggregate["hallucination_rate_unanswerable"] = None

    aggregate["unanswerable_tests"] = unanswerable_count
    aggregate["answerable_tests"] = len(results) - unanswerable_count

    return aggregate


def main():
    parser = argparse.ArgumentParser(
        description="Compute abstention metrics for evaluation run",
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
        update_results_with_abstention(run_dir, args.eval_set)

    # Compute aggregate metrics
    aggregate = aggregate_abstention_metrics(run_dir, args.eval_set)

    # Output aggregate metrics
    if args.output_metrics:
        with open(args.output_metrics, "w", encoding="utf-8") as f:
            json.dump(aggregate, f, indent=2, ensure_ascii=False)
        print(f"Aggregate metrics written to: {args.output_metrics}")
    else:
        print("\nAggregate Abstention Metrics:")
        print(json.dumps(aggregate, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()

